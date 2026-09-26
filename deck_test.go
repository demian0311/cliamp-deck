package main

import (
	"strings"
	"testing"

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

func TestTiers(t *testing.T) {
	cases := []struct {
		w, h int
		want tier
	}{{38, 6, tierXS}, {60, 24, tierS}, {90, 40, tierM}, {130, 38, tierL}, {170, 30, tierL}, {170, 44, tierXL}}
	for _, c := range cases {
		if got := tierFor(c.w, c.h); got != c.want {
			t.Errorf("tierFor(%d,%d) = %d, want %d", c.w, c.h, got, c.want)
		}
	}
}

// Every tier must fill exactly w×h cells: no panel may spill past the edge.
func TestRenderFitsEveryTier(t *testing.T) {
	shuffle := false
	m := newModel(client{sock: "/nonexistent"})
	m.snap = &ipc.RuntimeSnapshot{State: "playing", Position: 107, Duration: 151, Volume: -4, Shuffle: &shuffle,
		EQPreset: "Rock", EQBands: []float64{5, 4, 1, -1, -2, -1, 2, 4, 5, 5},
		Track: &ipc.TrackInfo{Title: "Roygbiv", Artist: "Boards of Canada", Album: "Music Has the Right to Children"}}
	m.providers = []ipc.ProviderInfo{{Key: "radio", Name: "Radio"}, {Key: "local", Name: "Local"}}
	m.showProviders()
	m.hi = resample([]float64{.9, .8, .6, .7, .5, .4, .3, .2, .1, .05}, hiRes)
	m.pk.update(m.hi)
	for _, sz := range [][2]int{{210, 52}, {170, 44}, {130, 38}, {90, 40}, {60, 24}, {38, 6}, {28, 3}} {
		m.w, m.h = sz[0], sz[1]
		lines := strings.Split(stripANSI(m.render()), "\n")
		if len(lines) != sz[1] {
			t.Fatalf("%dx%d: %d lines", sz[0], sz[1], len(lines))
		}
		for i, l := range lines {
			if n := len([]rune(l)); n != sz[0] {
				t.Fatalf("%dx%d: line %d is %d cells", sz[0], sz[1], i, n)
			}
		}
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
