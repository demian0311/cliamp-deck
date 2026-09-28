package main

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/bjarneo/cliamp/ipc"
)

// Keys and mouse per docs/design/layout.md (Focus, keys, mouse).

func (m model) key(k string) (tea.Model, tea.Cmd) {
	if k == "ctrl+c" {
		m.rememberSource()
		return m, tea.Quit
	}
	if m.searching {
		return m.searchKey(k)
	}
	if m.take {
		switch k {
		case "esc", "enter", "V":
			m.take = false
			return m, nil
		case "left":
			m.cycle(-1)
			return m, nil
		case "right", "v":
			m.cycle(1)
			return m, nil
		}
		return m.common(k)
	}
	if m.eqFocused() && k != "tab" && k != "shift+tab" {
		switch k {
		case "left", "h":
			m.eqBand = (m.eqBand + 9) % 10
			return m, nil
		case "right", "l":
			m.eqBand = (m.eqBand + 1) % 10
			return m, nil
		case "up", "k":
			cmd := m.setEQBand(m.eqBand, 1)
			return m, cmd
		case "down", "j":
			cmd := m.setEQBand(m.eqBand, -1)
			return m, cmd
		case "p":
			return m, m.nextPreset()
		case "0":
			return m, m.setPreset("Flat")
		case "e", "esc":
			m.eqOpen = false
			return m, nil
		}
		return m.common(k)
	}
	switch k {
	case "q":
		m.rememberSource()
		return m, tea.Quit
	case "tab":
		m.focus = (m.focus + 1) % focusCount
		return m, nil
	case "shift+tab":
		m.focus = (m.focus + focusCount - 1) % focusCount
		return m, nil
	case "V":
		m.take = true
		return m, nil
	case "e": // shows the EQ (or returns to it); inside it, e hides it
		m.eqOpen, m.focus = true, focusPlayer
		return m, nil
	case "/":
		m.focus, m.tab, m.searching, m.query = focusSources, tabSources, true, ""
		return m, nil
	case "[", "]":
		d := 1
		if k == "[" {
			d = tabCount - 1
		}
		return m.switchTab((m.tab + d) % tabCount)
	}
	switch m.focus {
	case focusPlayer:
		switch k {
		case "up", "k":
			return m, m.run("", "volume.adjust", map[string]float64{"value": 2})
		case "down", "j":
			return m, m.run("", "volume.adjust", map[string]float64{"value": -2})
		case "right", "l":
			return m, m.run("", "seek", map[string]float64{"value": 5})
		case "left", "h":
			return m, m.run("", "seek", map[string]float64{"value": -5})
		}
	case focusVis:
		switch k {
		case "left":
			m.cycle(-1)
			return m, nil
		case "right":
			m.cycle(1)
			return m, nil
		case "enter":
			m.take = true
			return m, nil
		}
	case focusSources:
		l := &m.lists[m.tab]
		switch k {
		case "down", "j":
			l.move(1)
			return m.moreIfAtEnd()
		case "up", "k":
			l.move(-1)
			return m, nil
		case "pgdown":
			l.move(10)
			return m.moreIfAtEnd()
		case "pgup":
			l.move(-10)
			return m, nil
		case "right", "l":
			if r, ok := m.selected(); ok && (r.kind == rowProvider || r.kind == rowCountry) {
				return m.activate()
			}
			return m, nil
		case "left", "h":
			if m.tab == tabSources && (m.inResults || m.provider != "") {
				return m.back()
			}
			return m, nil
		case "home":
			l.sel = firstSelectable(l.rows)
			return m, nil
		case "end":
			l.move(len(l.rows))
			return m.moreIfAtEnd()
		case "enter":
			return m.activate()
		case "a", "A":
			return m.enqueue(k == "A")
		case "s":
			if r, ok := m.selected(); ok && r.kind == rowSetup {
				return m.connect(r)
			}
		case "esc", "backspace":
			return m.back()
		}
	}
	return m.common(k)
}

// common are the keys that work whatever has focus.
func (m model) common(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "space":
		return m, m.run("", "toggle", nil)
	case "n":
		return m, m.run("next", "next", nil)
	case "p":
		return m, m.run("previous", "prev", nil)
	case "s":
		return m, m.run("stopped", "stop", nil)
	case "+", "=":
		return m, m.run("", "volume.adjust", map[string]float64{"value": 2})
	case "-", "_":
		return m, m.run("", "volume.adjust", map[string]float64{"value": -2})
	case ".":
		return m, m.run("", "seek", map[string]float64{"value": 5})
	case ",":
		return m, m.run("", "seek", map[string]float64{"value": -5})
	case "v":
		m.cycle(1)
	case "{":
		m.nudgeSync(-syncStepMs)
	case "}":
		m.nudgeSync(syncStepMs)
	}
	return m, nil
}

func (m model) searchKey(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "esc":
		m.searching = false
		return m, nil
	case "enter":
		m.searching = false
		if m.query == "" {
			return m, nil
		}
		scope := m.providers
		if m.provider != "" {
			scope = nil
			for _, p := range m.providers {
				if p.Key == m.provider {
					scope = append(scope, p)
				}
			}
		} else {
			scope = nil
			for _, p := range m.providers {
				if p.Searchable {
					scope = append(scope, p)
				}
			}
		}
		m.loading = true
		m.say("searching " + m.query + "…")
		return m, search(m.c, m.query, scope)
	case "backspace":
		if r := []rune(m.query); len(r) > 0 {
			m.query = string(r[:len(r)-1])
		}
	case "space":
		m.query += " "
	default:
		if r := []rune(k); len(r) == 1 {
			m.query += k
		}
	}
	return m, nil
}

func (m model) switchTab(t int) (tea.Model, tea.Cmd) {
	m.tab, m.focus = t, focusSources
	switch t {
	case tabQueue:
		if m.snap != nil {
			m.queueRev = m.snap.PlaylistRevision
		}
		return m, fetchQueue(m.c)
	case tabHistory:
		return m, fetchHistory(m.c)
	}
	return m, nil
}

func (m model) selected() (row, bool) {
	l := m.lists[m.tab]
	if l.sel < 0 || l.sel >= len(l.rows) {
		return row{}, false
	}
	return l.rows[l.sel], true
}

// activate is Enter or a double-click on the selected row.
func (m model) activate() (tea.Model, tea.Cmd) {
	r, ok := m.selected()
	if !ok || m.loading {
		return m, nil
	}
	switch r.kind {
	case rowProvider:
		m.loading = true
		m.pending = &pendingRow{tab: m.tab, key: r.key, since: time.Now()}
		m.say("opening " + r.label + "…")
		return m, m.openProvider(r.key, r.label, false)
	case rowPlaylist:
		m.pending = &pendingRow{tab: m.tab, key: r.key, playlist: r.provider + ":" + r.key, since: time.Now()}
		m.say("loading " + r.label + "…")
		return m, m.run("playing "+r.label, "provider.load", map[string]string{"provider": r.provider, "playlist": r.key})
	case rowTrack:
		m.pending = &pendingRow{tab: m.tab, path: r.track.Path, since: time.Now()}
		if m.tab == tabQueue {
			return m, m.run("playing "+r.label, "queue.play", map[string]int{"index": r.index})
		}
		return m, m.run("playing "+r.label, "track.play", map[string]*ipc.TrackInfo{"track": r.track})
	case rowSetup:
		return m.connect(r)
	case rowCountry:
		m.inCountry, m.country = true, strings.TrimPrefix(r.key, countryKey)
		m.lists[tabSources] = listState{rows: m.playlistRows(m.provider, m.stations)}
		m.rememberSource()
		return m, nil
	}
	return m, nil
}

// enqueue is a (append to cliamp's live playlist) or A (play next).
func (m model) enqueue(next bool) (tea.Model, tea.Cmd) {
	r, ok := m.selected()
	if !ok || r.kind != rowTrack {
		m.say("a and A work on tracks: search with / or open queue or history")
		return m, nil
	}
	switch {
	case next && m.tab == tabQueue:
		return m, m.run("next up: "+r.label, "queue.enqueue", map[string]int{"index": r.index})
	case next:
		return m, m.run("next up: "+r.label, "track.queue", map[string]*ipc.TrackInfo{"track": r.track})
	default:
		return m, m.run("queued "+r.label, "queue", map[string]string{"path": r.track.Path})
	}
}

// connect runs cliamp's setup wizard, then restarts cliamp so the source loads.
func (m model) connect(r row) (tea.Model, tea.Cmd) {
	m.say("setting up " + r.label + " with cliamp setup — if you save, cliamp restarts and playback stops")
	return m, runSetup(m.c)
}

func (m model) back() (tea.Model, tea.Cmd) {
	switch {
	case m.inResults:
		m.inResults, m.query = false, ""
		if m.provider != "" {
			return m.reopenProvider()
		}
		m.lists[tabSources] = listState{rows: m.topRows()}
	case m.tab == tabSources && m.inCountry:
		m.inCountry = false
		m.lists[tabSources] = listState{rows: m.playlistRows(m.provider, m.stations)}
		for i, r := range m.lists[tabSources].rows {
			if r.key == countryKey+m.country {
				m.lists[tabSources].sel = i
			}
		}
	case m.tab == tabSources && m.provider != "":
		key := m.provider
		m.provider, m.providerName = "", ""
		m.lists[tabSources] = listState{rows: m.topRows()}
		for i, r := range m.lists[tabSources].rows {
			if r.key == key {
				m.lists[tabSources].sel = i
			}
		}
	default:
		m.focus = focusVis
	}
	if m.tab == tabSources {
		m.rememberSource()
	}
	return m, nil
}

func (m model) reopenProvider() (tea.Model, tea.Cmd) {
	m.loading = true
	return m, m.openProvider(m.provider, m.providerName, false)
}

const catalogPage = 100

// openProvider lists a provider's playlists. A paged catalog (Radio) starts
// empty in a fresh daemon, so its first page is requested explicitly; more is
// the next page, fetched when the selection reaches the end of the list.
func (m model) openProvider(key, name string, more bool) tea.Cmd {
	isCatalog := false
	for _, p := range m.providers {
		if p.Key == key {
			isCatalog = p.Catalog
		}
	}
	c := m.c
	offset := 0
	if more {
		for _, r := range m.lists[tabSources].rows {
			if strings.HasPrefix(r.key, "c:") {
				offset++
			}
		}
	}
	return func() tea.Msg {
		if isCatalog {
			l, added, err := c.catalog(key, offset, catalogPage)
			return playlistsMsg{provider: key, name: name, list: l, err: err, catalog: true, added: added, keepSel: more}
		}
		l, err := c.playlists(key)
		return playlistsMsg{provider: key, name: name, list: l, err: err}
	}
}

// moreIfAtEnd fetches the next catalog page once the selection hits the last row.
func (m model) moreIfAtEnd() (tea.Model, tea.Cmd) {
	l := m.lists[tabSources]
	if m.tab != tabSources || !m.catalog || m.loading || m.inResults || l.sel < len(l.rows)-1 {
		return m, nil
	}
	m.loading = true
	return m, m.openProvider(m.provider, m.providerName, true)
}

// visControls are the clickable ‹ and › on the visualizer frame.
func visControls(l layout) (prev, next rect) {
	if !l.visBoxed || l.vis.w < 16 {
		return rect{}, rect{}
	}
	x := l.vis.x + l.vis.w - 10
	return rect{x, l.vis.y, 2, 1}, rect{x + 6, l.vis.y, 2, 1}
}

// tabsCollapsed reports a list too narrow for every tab label: only the
// active tab shows, as ‹ name ›, and clicking it moves to the next.
func tabsCollapsed(r rect) bool {
	w := 4
	for _, n := range tabNames {
		w += len(n) + 3
	}
	return r.w < w+10 // room left for the list's context label
}

// tabRects are the clickable tab labels on the list's top border.
func tabRects(r rect, active int) [tabCount]rect {
	var out [tabCount]rect
	if tabsCollapsed(r) {
		out[active] = rect{r.x + 2, r.y, len(tabNames[active]) + 6, 1}
		return out
	}
	x := r.x + 2
	for i, n := range tabNames {
		w := len(n) + 2
		out[i] = rect{x, r.y, w, 1}
		x += w + 1
	}
	return out
}

// listRows is the area of list rows inside the list panel, below any query line.
func (m model) listRows(r rect) rect {
	in := r.inner()
	if m.tab == tabSources && (m.searching || m.inResults) {
		in.y++
		in.h--
	}
	return in
}

func (m model) click(ev tea.Mouse) (tea.Model, tea.Cmd) {
	if ev.Button != tea.MouseLeft {
		return m, nil
	}
	if m.take {
		m.take = false
		return m, nil
	}
	l := m.layout()
	x, y := ev.X, ev.Y
	prev, next := visControls(l)
	switch {
	case prev.has(x, y):
		m.focus = focusVis
		m.cycle(-1)
	case next.has(x, y):
		m.focus = focusVis
		m.cycle(1)
	case l.vis.has(x, y):
		m.focus, m.take = focusVis, true
	case l.sources.has(x, y):
		if y == l.sources.y {
			for i, t := range tabRects(l.sources, m.tab) {
				if t.has(x, y) {
					if tabsCollapsed(l.sources) {
						return m.switchTab((i + 1) % tabCount)
					}
					return m.switchTab(i)
				}
			}
			return m, nil
		}
		wasFocused := m.focus == focusSources
		m.focus = focusSources
		rows := m.listRows(l.sources)
		list := &m.lists[m.tab]
		if !rows.has(x, y) || len(list.rows) == 0 {
			return m, nil
		}
		i := listStart(len(list.rows), list.sel, rows.h) + (y - rows.y)
		if i >= len(list.rows) {
			return m, nil
		}
		now := time.Now()
		double := wasFocused && i == m.lastClickRow && now.Sub(m.lastClick) < doubleClick
		list.sel, m.lastClick, m.lastClickRow = i, now, i
		if !wasFocused {
			m.lastClickRow = -1 // focusing reflows the list, so this click can't start a double-click
		}
		if double {
			m.lastClickRow = -1
			return m.activate()
		}
	case l.eq.has(x, y):
		if b := eqBandAt(l.eq, x); b >= 0 {
			m.eqBand = b
		}
	case l.player.has(x, y):
		m.focus = focusPlayer
	}
	return m, nil
}

func (m model) wheel(ev tea.Mouse) (tea.Model, tea.Cmd) {
	l := m.layout()
	if m.take || !l.sources.has(ev.X, ev.Y) {
		return m, nil
	}
	m.focus = focusSources
	switch ev.Button {
	case tea.MouseWheelDown:
		m.lists[m.tab].move(3)
		return m.moreIfAtEnd()
	case tea.MouseWheelUp:
		m.lists[m.tab].move(-3)
	}
	return m, nil
}
