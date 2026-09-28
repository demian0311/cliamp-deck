package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/bjarneo/cliamp/ipc"
)

// fakeCliamp answers every request line on a Unix socket with reply(request).
func fakeCliamp(t *testing.T, reply func(req map[string]any) string) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "s.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				sc := bufio.NewScanner(conn)
				for sc.Scan() {
					var req map[string]any
					json.Unmarshal(sc.Bytes(), &req)
					conn.Write([]byte(reply(req) + "\n"))
				}
			}()
		}
	}()
	return sock
}

func capsReply(names ...string) func(map[string]any) string {
	return func(req map[string]any) string {
		caps := make([]map[string]string, len(names))
		for i, n := range names {
			caps[i] = map[string]string{"name": n}
		}
		b, _ := json.Marshal(map[string]any{"version": 2, "id": req["id"], "ok": true, "result": caps})
		return string(b)
	}
}

func TestCompatAcceptsACliampWithEveryOperation(t *testing.T) {
	sock := fakeCliamp(t, capsReply(append([]string{"eq", "lyrics"}, requiredOps...)...))
	if err := checkCompat(client{sock: sock}); err != nil {
		t.Fatal(err)
	}
}

func TestCompatNamesMissingOperations(t *testing.T) {
	sock := fakeCliamp(t, capsReply("toggle", "next", "prev", "stop", "seek", "volume.adjust"))
	err := checkCompat(client{sock: sock})
	if err == nil || !strings.Contains(err.Error(), "provider.list, provider.playlists, provider.load") {
		t.Fatalf("got %v", err)
	}
}

// A pre-V2 cliamp answers in its old unversioned shape.
func TestCompatRejectsAnOldProtocol(t *testing.T) {
	sock := fakeCliamp(t, func(map[string]any) string { return `{"ok":false,"error":"unknown command"}` })
	err := checkCompat(client{sock: sock})
	if err == nil || !strings.Contains(err.Error(), "doesn't speak the V2 remote API") {
		t.Fatalf("got %v", err)
	}
}

// Quitting stops a cliamp that is not a headless daemon (here: no pid file)
// through the stop operation, leaving the process alone.
func TestQuitStopsPlayback(t *testing.T) {
	var ops []string
	var mu sync.Mutex
	sock := fakeCliamp(t, func(req map[string]any) string {
		mu.Lock()
		if op, _ := req["operation"].(string); op != "" {
			ops = append(ops, op)
		}
		mu.Unlock()
		b, _ := json.Marshal(map[string]any{"version": 2, "id": req["id"], "ok": true})
		return string(b)
	})
	if err := stopCliamp(client{sock: sock}); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(ops) != 1 || ops[0] != "stop" {
		t.Fatalf("operations sent: %v", ops)
	}
}

func TestZTogglesShuffle(t *testing.T) {
	var got []map[string]any
	var mu sync.Mutex
	sock := fakeCliamp(t, func(req map[string]any) string {
		mu.Lock()
		got = append(got, req)
		mu.Unlock()
		b, _ := json.Marshal(map[string]any{"version": 2, "id": req["id"], "ok": true})
		return string(b)
	})
	m := testModel(t)
	m.c = client{sock: sock}
	_, cmd := m.key("z")
	if cmd == nil {
		t.Fatal("z sent nothing")
	}
	if msg, ok := cmd().(opMsg); !ok || msg.err != nil || msg.label != "shuffle on" {
		t.Fatalf("reply: %#v", msg)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0]["operation"] != "shuffle" {
		t.Fatalf("requests: %v", got)
	}
	if params, _ := got[0]["params"].(map[string]any); params["name"] != "toggle" {
		t.Fatalf("params: %v", got[0]["params"])
	}
}

// A deck restarted after playing from a provider playlist reloads that
// playlist and moves to the track it was on, rather than playing it alone.
func TestResumeReloadsThePlaylist(t *testing.T) {
	var ops []string
	var played, seeked any
	var mu sync.Mutex
	sock := fakeCliamp(t, func(req map[string]any) string {
		mu.Lock()
		defer mu.Unlock()
		op, _ := req["operation"].(string)
		reply := map[string]any{"version": 2, "id": req["id"], "ok": true}
		if req["method"] == "state.get" {
			reply["snapshot"] = map[string]any{"state": "playing", "seekable": true, "duration": 200,
				"track": map[string]any{"path": "spotify:track:5"}}
		} else {
			ops = append(ops, op)
		}
		switch op {
		case "queue.list":
			var tracks []map[string]any
			for i := range 8 {
				tracks = append(tracks, map[string]any{"path": fmt.Sprintf("spotify:track:%d", i), "index": i})
			}
			reply["result"] = map[string]any{"tracks": tracks}
		case "queue.play":
			played = req["params"].(map[string]any)["index"]
		case "seek.absolute":
			seeked = req["params"].(map[string]any)["value"]
		}
		b, _ := json.Marshal(reply)
		return string(b)
	})
	path := filepath.Join(t.TempDir(), "state.toml")
	m := newModel(client{sock: sock}, "/nonexistent/colors.toml", path)
	m.rememberPlaylist("spotify", "pl1", "Road Trip")
	m.saved.LastTrack = &ipc.TrackInfo{Path: "spotify:track:5", Title: "Five"}
	m.saved.LastPosition = 42
	m.persist()

	m = newModel(client{sock: sock}, "/nonexistent/colors.toml", path)
	m.snap = &ipc.RuntimeSnapshot{State: "stopped"}
	cmd := m.resume()
	if cmd == nil {
		t.Fatal("nothing resumed")
	}
	if m.provider != "spotify" || m.playlist != "pl1" || m.playlistName != "Road Trip" {
		t.Errorf("sources not inside the resumed playlist: %q › %q", m.provider, m.playlist)
	}
	if msg := cmd().(opMsg); msg.err != nil || msg.label != "playing Five" {
		t.Fatalf("reply: %#v", msg)
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(ops, " ") != "provider.load queue.list queue.play seek.absolute" || played != float64(5) || seeked != float64(42) {
		t.Fatalf("ops %v, played index %v, seeked to %v", ops, played, seeked)
	}

	m.rememberPlaylist("", "", "") // a lone track was started since
	if s := loadState(path); s.LastPlaylist != "" || s.LastProvider != "" {
		t.Fatalf("playlist still remembered: %+v", s)
	}
}

// Enter on a track of a playlist cliamp has not loaded loads the playlist,
// then plays that track in it.
func TestEnterOnATrackLoadsItsPlaylist(t *testing.T) {
	var ops []string
	var loaded, played any
	var mu sync.Mutex
	sock := fakeCliamp(t, func(req map[string]any) string {
		mu.Lock()
		defer mu.Unlock()
		op, _ := req["operation"].(string)
		ops = append(ops, op)
		reply := map[string]any{"version": 2, "id": req["id"], "ok": true}
		switch op {
		case "provider.load":
			loaded = req["params"]
		case "queue.list":
			var tracks []map[string]any
			for i := range 3 {
				tracks = append(tracks, map[string]any{"path": fmt.Sprintf("t%d", i), "index": i})
			}
			reply["result"] = map[string]any{"tracks": tracks, "total": 3}
		case "queue.play":
			played = req["params"].(map[string]any)["index"]
		}
		b, _ := json.Marshal(reply)
		return string(b)
	})
	m := spotifyModel(t)
	m.c = client{sock: sock}
	m = key(m, "right") // Road Trip, not the loaded playlist
	m = upd(m, tracksMsg{key: m.rowsKey, tracks: []ipc.TrackInfo{{Path: "t0"}, {Path: "t1"}, {Path: "t2", Title: "Two"}}})
	m = key(key(m, "down"), "down")
	next, cmd := m.key("enter")
	m = next.(model)
	if cmd == nil || m.saved.LastPlaylist != "pl1" {
		t.Fatalf("nothing sent, remembered %q", m.saved.LastPlaylist)
	}
	if msg := cmd().(opMsg); msg.err != nil || msg.label != "playing Two" {
		t.Fatalf("reply: %#v", msg)
	}
	mu.Lock()
	defer mu.Unlock()
	params, _ := loaded.(map[string]any)
	if strings.Join(ops, " ") != "provider.load queue.list queue.play" || params["playlist"] != "pl1" || played != float64(2) {
		t.Fatalf("ops %v, loaded %v, played index %v", ops, loaded, played)
	}
}
