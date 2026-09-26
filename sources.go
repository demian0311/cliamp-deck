package main

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bjarneo/cliamp/ipc"
)

// The list area has three tabs. "sources" drills providers → playlists and
// also holds search results; queue and history are cliamp's own lists.
const (
	tabSources = iota
	tabQueue
	tabHistory
	tabCount
)

var tabNames = [tabCount]string{"sources", "queue", "history"}

type rowKind int

const (
	rowProvider rowKind = iota // a configured source; Enter opens it
	rowPlaylist                // a playlist or station; Enter loads it
	rowTrack                   // a single track; Enter plays, a appends, A plays next
	rowHeader                  // a group label, not selectable for actions
	rowSetup                   // a source cliamp can set up; s or Enter runs cliamp setup
)

type row struct {
	kind          rowKind
	label, right  string
	key, provider string
	track         *ipc.TrackInfo
	index         int  // position in cliamp's live playlist (queue tab)
	current       bool // what is playing now
}

type listState struct {
	rows []row
	sel  int
}

func (l *listState) move(d int) {
	if len(l.rows) == 0 {
		l.sel = 0
		return
	}
	l.sel = max(0, min(len(l.rows)-1, l.sel+d))
}

// listStart is the first visible row so the selection stays in view.
func listStart(n, sel, rows int) int {
	if n <= rows || rows <= 0 {
		return 0
	}
	return max(0, min(n-rows, sel-rows/2))
}

type (
	queueMsg struct {
		tracks []ipc.TrackInfo
		err    error
	}
	historyMsg struct {
		items []ipc.HistoryInfo
		err   error
	}
	searchGroup struct {
		name   string
		tracks []ipc.TrackInfo
		err    error
	}
	searchMsg struct {
		query  string
		groups []searchGroup
	}
)

func (c client) response(op string, params any) (ipc.Response, error) {
	var r ipc.Response
	raw, err := c.op(op, params)
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(raw, &r)
	return r, err
}

func fetchQueue(c client) tea.Cmd {
	return func() tea.Msg {
		r, err := c.response("queue.list", map[string]int{"limit": 200})
		return queueMsg{r.Tracks, err}
	}
}

func fetchHistory(c client) tea.Cmd {
	return func() tea.Msg {
		r, err := c.response("history", map[string]int{"limit": 100})
		return historyMsg{r.History, err}
	}
}

// search asks every provider in parallel, so slow ones (YouTube, Spotify) only
// delay their own group.
func search(c client, query string, providers []ipc.ProviderInfo) tea.Cmd {
	return func() tea.Msg {
		groups := make([]searchGroup, len(providers))
		var wg sync.WaitGroup
		for i, p := range providers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, err := c.response("provider.search", map[string]any{"provider": p.Key, "query": query, "limit": 25})
				groups[i] = searchGroup{p.Name, r.Tracks, err}
			}()
		}
		wg.Wait()
		return searchMsg{query, groups}
	}
}

func (m *model) topRows() []row {
	var rows []row
	configured := map[string]bool{}
	for _, p := range m.providers {
		configured[p.Key] = true
		rows = append(rows, row{kind: rowProvider, label: p.Name, key: p.Key,
			current: m.snap != nil && strings.HasPrefix(m.snap.Playlist, p.Key+":")})
	}
	var setup []row
	for _, sp := range setupProviders {
		done := false
		for _, k := range sp.keys {
			done = done || configured[k]
		}
		if !done {
			setup = append(setup, row{kind: rowSetup, label: sp.name, right: "s to connect"})
		}
	}
	if len(setup) > 0 && len(m.providers) > 0 {
		rows = append(rows, row{kind: rowHeader, label: "not set up"})
		rows = append(rows, setup...)
	}
	return rows
}

func (m *model) playlistRows(provider string, list []ipc.PlaylistInfo) []row {
	rows := make([]row, 0, len(list))
	for _, p := range list {
		rows = append(rows, row{kind: rowPlaylist, label: p.Name, right: p.Section, key: p.ID, provider: provider,
			current: m.snap != nil && m.snap.Playlist == provider+":"+p.ID})
	}
	return rows
}

func trackRow(t ipc.TrackInfo, right string) row {
	return row{kind: rowTrack, label: trackTitle(t.Artist, t.Title), right: right, track: &t}
}

func (m *model) queueRows(tracks []ipc.TrackInfo) []row {
	rows := make([]row, 0, len(tracks))
	for i, t := range tracks {
		r := trackRow(t, durationText(t.DurationSecs))
		r.index = i
		r.current = m.snap != nil && m.snap.Index == i
		rows = append(rows, r)
	}
	return rows
}

func historyRows(items []ipc.HistoryInfo) []row {
	rows := make([]row, 0, len(items))
	for _, h := range items {
		when := ""
		if t, err := time.Parse(time.RFC3339, h.PlayedAt); err == nil {
			when = t.Local().Format("Jan 2 15:04")
		}
		rows = append(rows, trackRow(h.Track, when))
	}
	return rows
}

func searchRows(groups []searchGroup) []row {
	var rows []row
	for _, g := range groups {
		switch {
		case g.err != nil:
			rows = append(rows, row{kind: rowHeader, label: g.name + " · " + g.err.Error()})
		case len(g.tracks) == 0:
			rows = append(rows, row{kind: rowHeader, label: g.name + " · no matches"})
		default:
			rows = append(rows, row{kind: rowHeader, label: g.name})
			for _, t := range g.tracks {
				rows = append(rows, trackRow(t, durationText(t.DurationSecs)))
			}
		}
	}
	return rows
}

func durationText(secs int) string {
	if secs <= 0 {
		return ""
	}
	return mmss(float64(secs))
}
