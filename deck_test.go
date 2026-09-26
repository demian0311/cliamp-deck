package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bjarneo/cliamp/ipc"
)

func TestResampleHitsBandCentresAndStaysInRange(t *testing.T) {
	bands := []float64{0.9, 0.1, 0.8, 0, 1}
	out := resample(bands, 9) // every second sample lands on a band centre
	for i, b := range bands {
		if got := out[i*2]; got < b-1e-9 || got > b+1e-9 {
			t.Errorf("out[%d] = %v, want band %v", i*2, got, b)
		}
	}
	for i, v := range resample(bands, 200) {
		if v < 0 || v > 1 {
			t.Fatalf("out[%d] = %v escapes [0,1]", i, v)
		}
	}
}

func TestBrailleCell(t *testing.T) {
	cases := []struct {
		a, b, pa, pb, row int
		want              rune
	}{
		{0, 0, 0, 0, 0, '⠀'},
		{4, 4, 0, 0, 0, '⣿'},
		{4, 4, 0, 0, 1, '⠀'}, // bar is one cell tall
		{1, 0, 0, 0, 0, '⡀'}, // bottom-left dot
		{2, 4, 0, 0, 0, '⣼'}, // left half-height, right full
		{0, 0, 3, 0, 0, '⠂'}, // lone peak dot, third from bottom
		{5, 0, 0, 0, 1, '⡀'}, // fifth dot spills into the next cell
	}
	for _, c := range cases {
		if got := brailleCell(c.a, c.b, c.pa, c.pb, c.row); got != c.want {
			t.Errorf("brailleCell(%d,%d,%d,%d,%d) = %q, want %q", c.a, c.b, c.pa, c.pb, c.row, got, c.want)
		}
	}
}

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
	for mode := 0; mode <= len(m.effects); mode++ {
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
		"Iran International": {"Iran International", "", ""},
	} {
		n, b, c := splitStation(in)
		if [3]string{n, b, c} != want {
			t.Errorf("%q: got %q %q %q", in, n, b, c)
		}
	}
}

// Catalog stations group under their country in order of each country's most
// popular station; pinned entries stay on top and stationless countries last.
func TestRadioCatalogGroupsByCountry(t *testing.T) {
	m := testModel(t)
	rows := m.playlistRows("radio", []ipc.PlaylistInfo{
		{ID: "p:0", Name: "United States (near you)"},
		{ID: "c:0", Name: "A [128k] · Germany"},
		{ID: "c:1", Name: "B [64k]"},
		{ID: "c:2", Name: "C · France"},
		{ID: "c:3", Name: "D [320k] · Germany"},
	})
	var got []string
	for _, r := range rows {
		got = append(got, r.label+"|"+r.right)
	}
	want := []string{"United States (near you)|", "Germany|2", "A|128k", "D|320k", "France|1", "C|", "elsewhere|1", "B|64k"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("rows:\n got %v\nwant %v", got, want)
	}

	l := listState{rows: rows}
	l.move(1) // steps over the Germany header
	if l.sel != 2 {
		t.Fatalf("move landed on %d", l.sel)
	}
	l.jumpGroup(1)
	if rows[l.sel].label != "C" {
		t.Fatalf("next group landed on %q", rows[l.sel].label)
	}
	l.jumpGroup(-1)
	if rows[l.sel].label != "A" {
		t.Fatalf("previous group landed on %q", rows[l.sel].label)
	}
}
