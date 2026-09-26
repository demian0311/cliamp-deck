package main

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/bjarneo/cliamp/ipc"
)

func TestLayoutShapes(t *testing.T) {
	cases := []struct {
		w, h int
		want shape
	}{{200, 58, shapeStack}, {100, 58, shapeStack}, {100, 28, shapeStack}, {60, 24, shapeStack},
		{280, 58, shapeSide}, {50, 20, shapeMini}, {100, 12, shapeMini}}
	for _, c := range cases {
		if got := computeLayout(c.w, c.h, layoutState{}).shape; got != c.want {
			t.Errorf("%dx%d: shape %d, want %d", c.w, c.h, got, c.want)
		}
	}
	if l := computeLayout(100, 28, layoutState{take: true}); l.shape != shapeTakeover || l.vis != (rect{0, 0, 100, 28}) {
		t.Errorf("takeover: %+v", l)
	}
}

// The stack splits spare rows 60/40, the list grows while focused, and an
// open EQ takes its rows from the visualizer.
func TestStackHeights(t *testing.T) {
	base := computeLayout(100, 58, layoutState{})
	if base.player.h != 6 || base.vis.h+base.sources.h != 57-6 {
		t.Fatalf("heights %+v", base)
	}
	if base.vis.h <= base.sources.h {
		t.Errorf("visualizer %d should out-size sources %d", base.vis.h, base.sources.h)
	}
	grown := computeLayout(100, 58, layoutState{focus: focusSources})
	if grown.sources.h <= base.sources.h || grown.vis.h >= base.vis.h {
		t.Errorf("focused list did not grow: %d→%d", base.sources.h, grown.sources.h)
	}
	eq := computeLayout(100, 58, layoutState{eqOpen: true})
	if eq.eq.h != 10 || eq.eq.y != 6 || eq.sources.h < base.sources.h-1 || eq.vis.h >= base.vis.h {
		t.Errorf("eq open: %+v", eq)
	}
	side := computeLayout(280, 58, layoutState{})
	if side.vis.x != side.player.w || side.vis.h != 57 || side.sources.w != side.player.w {
		t.Errorf("side: %+v", side)
	}
}

func testModel(t *testing.T) model {
	t.Helper()
	shuffle := false
	m := newModel(client{sock: "/nonexistent"}, "/nonexistent/colors.toml", filepath.Join(t.TempDir(), "state.toml"))
	m.snap = &ipc.RuntimeSnapshot{State: "playing", Position: 107, Duration: 151, Volume: -4, Shuffle: &shuffle,
		EQPreset: "Rock", EQBands: []float64{5, 4, 2, -1, -2, 2, 4, 5, 5, 5}, Playlist: "radio:c:3",
		Track: &ipc.TrackInfo{Title: "Roygbiv", Artist: "Boards of Canada", Album: "Music Has the Right to Children"}}
	m.providers = []ipc.ProviderInfo{{Key: "radio", Name: "Radio", Searchable: true}, {Key: "local", Name: "Local"}}
	m.lists[tabSources].rows = m.topRows()
	m.w, m.h = 100, 28
	return m
}

func upd(m model, msg tea.Msg) model {
	next, _ := m.Update(msg)
	return next.(model)
}

// Every shape, visualizer, focus and panel combination fills exactly w×h cells.
func TestRenderFitsEverywhere(t *testing.T) {
	m := testModel(t)
	bands := specMsg{[]float64{.9, .8, .6, .7, .5, .4, .3, .2, .1, .05}}
	sizes := [][2]int{{280, 58}, {200, 58}, {100, 58}, {100, 28}, {60, 24}, {50, 20}, {38, 6}, {28, 3}}
	for mode := range len(m.effects) {
		for _, st := range []layoutState{{}, {focus: focusSources}, {eqOpen: true}, {take: true}, {focus: focusPlayer, eqOpen: true}} {
			m.mode, m.take, m.eqOpen, m.focus = mode, st.take, st.eqOpen, st.focus
			for _, sz := range sizes {
				m.w, m.h = sz[0], sz[1]
				mm := upd(m, bands)
				lines := strings.Split(stripANSI(mm.render()), "\n")
				if len(lines) != sz[1] {
					t.Fatalf("mode %d %+v %dx%d: %d lines", mode, st, sz[0], sz[1], len(lines))
				}
				for i, l := range lines {
					if n := len([]rune(l)); n != sz[0] {
						t.Fatalf("mode %d %+v %dx%d: line %d is %d cells", mode, st, sz[0], sz[1], i, n)
					}
				}
			}
		}
	}
}

func key(m model, k string) model {
	next, _ := m.key(k)
	return next.(model)
}

func TestFocusAndVisualizerKeys(t *testing.T) {
	m := testModel(t)
	if m.focus != focusVis {
		t.Fatalf("starts focused on %d", m.focus)
	}
	m = key(m, "tab")
	if m.focus != focusSources {
		t.Fatalf("tab → %d", m.focus)
	}
	m = key(key(m, "shift+tab"), "shift+tab")
	if m.focus != focusPlayer {
		t.Fatalf("shift+tab twice → %d", m.focus)
	}
	m = key(m, "tab") // visualizer
	mode := m.mode
	if m = key(m, "right"); m.mode != mode+1 {
		t.Errorf("→ on the visualizer did not cycle: %d", m.mode)
	}
	if m = key(m, "left"); m.mode != mode {
		t.Errorf("← did not cycle back")
	}
	if m = key(m, "enter"); !m.take {
		t.Fatal("enter on the visualizer did not take over")
	}
	if m = key(m, "right"); m.mode != mode+1 {
		t.Errorf("→ in takeover did not cycle")
	}
	if m = key(m, "esc"); m.take {
		t.Error("esc did not leave takeover")
	}
	if m = key(m, "V"); !m.take {
		t.Error("V did not take over")
	}
	if got := loadState(m.statePath).Visualizer; got != m.modeName() {
		t.Errorf("saved visualizer %q, showing %q", got, m.modeName())
	}
}

func click(m model, x, y int) model {
	next, _ := m.click(tea.Mouse{X: x, Y: y, Button: tea.MouseLeft})
	return next.(model)
}

func TestMouseTakeoverControlsAndRows(t *testing.T) {
	m := testModel(t)
	l := computeLayout(m.w, m.h, m.ls())
	prev, next := visControls(l)
	mode := m.mode
	if m = click(m, next.x, next.y); m.mode != mode+1 || m.take {
		t.Fatalf("› cycled to %d, take=%v", m.mode, m.take)
	}
	if m = click(m, prev.x, prev.y); m.mode != mode {
		t.Fatalf("‹ went to %d", m.mode)
	}
	c := l.vis.inner()
	if m = click(m, c.x+c.w/2, c.y+c.h/2); !m.take {
		t.Fatal("clicking the visualizer did not take over")
	}
	if m = click(m, 3, 3); m.take {
		t.Fatal("a click in takeover did not return")
	}
	src := computeLayout(m.w, m.h, m.ls()).sources
	rows := m.listRows(src)
	m = click(m, rows.x+3, rows.y+1)
	if m.focus != focusSources || m.lists[tabSources].sel != 1 {
		t.Fatalf("row click: focus %d sel %d", m.focus, m.lists[tabSources].sel)
	}
	tabs := tabRects(computeLayout(m.w, m.h, m.ls()).sources)
	if m = click(m, tabs[tabHistory].x+1, tabs[tabHistory].y); m.tab != tabHistory {
		t.Errorf("tab click → %d", m.tab)
	}
}

func TestDoubleClickActivatesOnlyOnceFocused(t *testing.T) {
	m := testModel(t)
	rowsAt := func(m model) rect { return m.listRows(computeLayout(m.w, m.h, m.ls()).sources) }
	r := rowsAt(m)
	m = click(m, r.x+3, r.y) // focuses the list; the layout reflows
	r = rowsAt(m)
	m = click(m, r.x+3, r.y)
	next, cmd := m.click(tea.Mouse{X: r.x + 3, Y: r.y, Button: tea.MouseLeft})
	if cmd == nil || !next.(model).loading {
		t.Fatal("double-click on a focused row did not open it")
	}
}

func TestUnconfiguredSourcesSitAtTheBottom(t *testing.T) {
	m := testModel(t)
	rows := m.lists[tabSources].rows
	if rows[0].kind != rowProvider || rows[1].kind != rowProvider || rows[2].kind != rowHeader {
		t.Fatalf("top of list: %+v", rows[:3])
	}
	for _, r := range rows[3:] {
		if r.kind != rowSetup {
			t.Errorf("after the header: %+v", r)
		}
	}
	if !rows[0].current {
		t.Error("the provider playing now is not marked")
	}
	m.providers = append(m.providers, ipc.ProviderInfo{Key: "ytmusic", Name: "YouTube Music"})
	for _, r := range m.topRows() {
		if r.kind == rowSetup && r.label == "YouTube Music" {
			t.Error("configured YouTube Music still offered for setup")
		}
	}
}

func TestStateRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "d", "state.toml")
	want := deckState{Visualizer: "tunnel", EQPreset: "Custom", EQBands: []float64{1, -2.5, 0, 0, 3, 0, 0, 0, 0, 12}}
	if err := saveState(p, want); err != nil {
		t.Fatal(err)
	}
	if got := loadState(p); got.Visualizer != want.Visualizer || got.EQPreset != want.EQPreset || !slices.Equal(got.EQBands, want.EQBands) {
		t.Errorf("got %+v", got)
	}
	if got := loadState(filepath.Join(t.TempDir(), "missing")); got.Visualizer != "" {
		t.Errorf("missing file: %+v", got)
	}
}

func TestEQPanelKeys(t *testing.T) {
	m := testModel(t)
	if m = key(m, "e"); !m.eqOpen {
		t.Fatal("e did not open the EQ")
	}
	if m = key(key(m, "right"), "right"); m.eqBand != 2 {
		t.Errorf("band %d", m.eqBand)
	}
	if _, cmd := m.key("up"); cmd == nil {
		t.Error("↑ sent no EQ change")
	}
	if m = key(m, "esc"); m.eqOpen {
		t.Error("esc did not close the EQ")
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func TestTrackTitleDropsARepeatedArtist(t *testing.T) {
	cases := []struct{ artist, title, want string }{
		{"Stella Jacobs", "Stella Jacobs - Ngak'thola (ft. Charlie SD)", "Stella Jacobs — Ngak'thola (ft. Charlie SD)"},
		{"stella jacobs", "Stella Jacobs – Ngak'thola", "stella jacobs — Ngak'thola"},
		{"Airglow", "Blueshift", "Airglow — Blueshift"},
		{"", "Lofi Stream", "Lofi Stream"},
		{"Boards of Canada", "Boards of Canada", "Boards of Canada"},
		{"Air", "Airglow - Blueshift", "Air — Airglow - Blueshift"}, // a prefix that is not the whole artist stays
	}
	for _, c := range cases {
		if got := trackTitle(c.artist, c.title); got != c.want {
			t.Errorf("trackTitle(%q, %q) = %q, want %q", c.artist, c.title, got, c.want)
		}
	}
}

func TestSplitStation(t *testing.T) {
	for in, want := range map[string][3]string{
		"MANGORADIO [128k] · Germany":  {"MANGORADIO", "128k", "Germany"},
		"REYFM - #original [192k]":     {"REYFM - #original", "192k", ""},
		"RFE/RL Radio Farda · Czechia": {"RFE/RL Radio Farda", "", "Czechia"},
		"Classic FM UK [128k] · United Kingdom Of Great Britain And Northern Ireland": {"Classic FM UK", "128k", "United Kingdom"},
		"Iran International":                            {"Iran International", "", ""},
		"SomaFM Groove Salad (128k MP3)":                {"SomaFM Groove Salad", "", ""},
		"TranceBase.FM - AAC HD 256k":                   {"TranceBase.FM", "", ""},
		"Radio Paradise Main Mix (EU) 320k AAC [320k]":  {"Radio Paradise Main Mix (EU)", "320k", ""},
		"Hit FM (UKraine) - 128kb/s":                    {"Hit FM (UKraine)", "", ""},
		"DiscoverTranceRadio (MP3 HQ stereo 192kBit/s)": {"DiscoverTranceRadio", "", ""},
		"Deutschlandfunk | DLF | MP3 128k":              {"Deutschlandfunk | DLF", "", ""},
		"Classic Vinyl HD Opus":                         {"Classic Vinyl HD", "", ""},
		"Classic Vinyl HD":                              {"Classic Vinyl HD", "", ""},
		"90s90s Dance HQ":                               {"90s90s Dance HQ", "", ""},
		"Polskie Radio - Czwórka (Program 4) (AAC+)":    {"Polskie Radio - Czwórka (Program 4)", "", ""},
		"98.1 KBEAR":                                    {"98.1 KBEAR", "", ""},
		"---Tarateel---":                                {"---Tarateel---", "", ""},
		"MP3":                                           {"MP3", "", ""},
	} {
		n, b, c := splitStation(in)
		if [3]string{n, b, c} != want {
			t.Errorf("%q: got %q %q %q", in, n, b, c)
		}
	}
}

// The radio level lists pinned entries, then countries by station count, ties
// alphabetical, no country last; → opens a country, ← returns to it.
func TestRadioCountriesOpenWithArrows(t *testing.T) {
	m := testModel(t)
	m.focus = focusSources
	m.snap.Playlist = "radio:c:3"
	m = upd(m, playlistsMsg{provider: "radio", name: "Radio", catalog: true, added: 4, list: []ipc.PlaylistInfo{
		{ID: "p:0", Name: "United States (near you)"},
		{ID: "c:0", Name: "A [128k] · Germany"},
		{ID: "c:1", Name: "B [64k]"},
		{ID: "c:2", Name: "C · France"},
		{ID: "c:3", Name: "D [320k] · Germany"},
		{ID: "c:4", Name: "E · Austria"},
		{ID: "c:5", Name: "F"},
	}})
	labels := func() string {
		var got []string
		for _, r := range m.lists[tabSources].rows {
			got = append(got, r.label+"|"+r.right)
		}
		return strings.Join(got, ",")
	}
	if got := labels(); got != "United States (near you)|,countries|,Germany|2 ›,Austria|1 ›,France|1 ›,elsewhere|2 ›" {
		t.Fatalf("radio level: %s", got)
	}
	if !m.lists[tabSources].rows[2].current {
		t.Error("the country playing now is not marked")
	}
	m = key(m, "down") // steps over the countries header
	if r, _ := m.selected(); r.label != "Germany" {
		t.Fatalf("down landed on %q", r.label)
	}
	m = key(key(key(m, "down"), "down"), "right")
	if got := labels(); !m.inCountry || got != "C|" {
		t.Fatalf("opened France: %s", got)
	}
	m = key(m, "left")
	if r, _ := m.selected(); m.inCountry || r.label != "France" {
		t.Fatalf("back landed on %q", r.label)
	}
	m = key(m, "left")
	if m.provider != "" {
		t.Fatal("← at the radio level did not return to the sources")
	}
	if r, _ := m.selected(); r.key != "radio" {
		t.Fatalf("back to sources landed on %q", r.key)
	}
	m = key(m, "left")
	if m.focus != focusSources {
		t.Error("← at the top level left the list")
	}
}

// e shows and hides the EQ; while shown it takes the player's arrows, and tab
// still moves focus. The focused panel's title turns negative.
func TestEKeyTogglesTheEQ(t *testing.T) {
	m := testModel(t)
	m.focus = focusVis
	m = key(m, "e")
	if !m.eqFocused() {
		t.Fatal("e did not open and focus the EQ")
	}
	m = key(m, "left")
	if !m.eqOpen || m.eqBand != 9 {
		t.Fatalf("← from the first band: open %v, band %d", m.eqOpen, m.eqBand)
	}
	m = key(m, "tab")
	if m.focus != focusVis || !m.eqOpen || m.eqFocused() {
		t.Fatalf("tab with the EQ open: focus %d, open %v", m.focus, m.eqOpen)
	}
	if !m.has(focusVis) || m.has(focusPlayer) {
		t.Error("focus highlight is on the wrong panel")
	}
	m = key(m, "e")
	if !m.eqFocused() {
		t.Fatal("e from another panel did not bring focus back to the EQ")
	}
	m = key(m, "e")
	if m.eqOpen {
		t.Fatal("e did not hide the EQ")
	}
}

func TestStreamsHideSkipButtons(t *testing.T) {
	m := testModel(t)
	m.h = 40 // tall enough for the transport row
	if !strings.Contains(stripANSI(m.render()), "◄◄") {
		t.Fatal("a track lost its skip buttons")
	}
	m.snap.Track.Stream, m.snap.Duration = true, 0
	if strings.Contains(stripANSI(m.render()), "◄◄") {
		t.Fatal("a stream still shows skip buttons")
	}
}

func TestBrailleBarsStackFourRows(t *testing.T) {
	cells, _ := brailleBars([]float64{1, 0.5, 0, 0.25}, nil, 2)
	got := string(cells)
	// top row full, second half, third empty, bottom a quarter (one dot)
	if got != "⡛⠉" {
		t.Fatalf("got %q", got)
	}
}

// A country picked in one run is where the sources list reopens in the next.
func TestSourcesReopenWhereLeft(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.toml")
	list := []ipc.PlaylistInfo{{ID: "c:0", Name: "A · Germany"}, {ID: "c:1", Name: "B · France"}, {ID: "c:2", Name: "C · France"}}
	providers := providersMsg{list: []ipc.ProviderInfo{{Key: "radio", Name: "Radio"}}}

	m := newModel(client{sock: "/nonexistent"}, "/nonexistent/colors.toml", path)
	m.focus = focusSources
	m = upd(m, providers)
	m = upd(m, playlistsMsg{provider: "radio", name: "Radio", list: list})
	m = key(m, "right") // France, the first country (most stations)
	m = key(m, "down")  // C
	m = key(m, "q")

	m = newModel(client{sock: "/nonexistent"}, "/nonexistent/colors.toml", path)
	next, cmd := m.Update(providers)
	m = next.(model)
	if cmd == nil || !m.restoring {
		t.Fatal("the remembered source was not reopened")
	}
	m = key(m, "q") // quitting mid-restore keeps the remembered place
	m = upd(m, playlistsMsg{provider: "radio", name: "Radio", list: list})
	if r, _ := m.selected(); !m.inCountry || m.country != "France" || r.label != "C" {
		t.Fatalf("reopened in %q (in country %v) on %q", m.country, m.inCountry, r.label)
	}
}

// Enter on a station spins a throbber on its row until cliamp reports it
// playing; a failed load stops it.
func TestLoadingRowThrobs(t *testing.T) {
	m := testModel(t)
	m.focus = focusSources
	m.snap.Playlist, m.snap.Position = "", 0
	m = upd(m, playlistsMsg{provider: "radio", name: "Radio", list: []ipc.PlaylistInfo{{ID: "l:0", Name: "cliamp radio"}}})
	m = key(m, "enter")
	spinning := func() bool {
		return strings.ContainsAny(stripANSI(m.render()), string(throbber))
	}
	if m.pending == nil || !spinning() {
		t.Fatal("no throbber after Enter")
	}
	m = upd(m, stateMsg{&ipc.RuntimeSnapshot{State: "playing", Playlist: "radio:l:0"}})
	if !spinning() {
		t.Fatal("throbber stopped before playback started")
	}
	m = upd(m, stateMsg{&ipc.RuntimeSnapshot{State: "playing", Playlist: "radio:l:0", Position: 1}})
	if m.pending != nil || spinning() {
		t.Fatal("throbber still spinning once playing")
	}
	m = key(m, "enter")
	m = upd(m, opMsg{label: "playing cliamp radio", err: errors.New("no route")})
	if m.pending != nil {
		t.Fatal("throbber still spinning after the load failed")
	}
}

// Focusing the list eases it taller over layoutDuration instead of snapping,
// and the panels stay edge to edge on the way.
func TestPanelsEaseToNewSizes(t *testing.T) {
	m := testModel(t)
	m.w, m.h = 100, 40
	m.focus = focusVis
	m = upd(m, tea.WindowSizeMsg{Width: 100, Height: 40})
	before := m.layout().sources.h
	m = upd(m, tea.KeyPressMsg{Code: tea.KeyTab}) // visualizer → sources
	if m.focus != focusSources {
		t.Fatalf("focus %d", m.focus)
	}
	start := m.anim.at
	after := computeLayout(m.w, m.h, m.ls()).sources.h
	mid := m.layoutAt(start.Add(layoutDuration / 3))
	if !(mid.sources.h > before && mid.sources.h < after) {
		t.Fatalf("sources %d → %d, a third of the way %d", before, after, mid.sources.h)
	}
	if mid.vis.y+mid.vis.h != mid.sources.y {
		t.Errorf("visualizer ends at %d, sources start at %d", mid.vis.y+mid.vis.h, mid.sources.y)
	}
	if end := m.layoutAt(start.Add(layoutDuration)); end.sources.h != after {
		t.Fatalf("settled at %d, want %d", end.sources.h, after)
	}
}

// Every frame of every transition still fills exactly w×h cells.
func TestRenderFitsMidAnimation(t *testing.T) {
	for _, sz := range [][2]int{{280, 58}, {100, 40}, {100, 28}, {60, 24}} {
		for _, k := range []string{"tab", "e", "V", "e", "tab"} {
			m := testModel(t)
			m.w, m.h = sz[0], sz[1]
			m = upd(m, tea.KeyPressMsg{Text: k, Code: []rune(k)[0]})
			if k == "tab" {
				m = upd(m, tea.KeyPressMsg{Code: tea.KeyTab})
			}
			for f := 0; f <= 6; f++ {
				m.anim.at = time.Now().Add(-layoutDuration * time.Duration(f) / 6)
				lines := strings.Split(stripANSI(m.render()), "\n")
				if len(lines) != sz[1] || len([]rune(lines[0])) != sz[0] {
					t.Fatalf("%v after %q, frame %d: %d lines", sz, k, f, len(lines))
				}
			}
		}
	}
}
