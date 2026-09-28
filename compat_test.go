package main

import (
	"bufio"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
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
