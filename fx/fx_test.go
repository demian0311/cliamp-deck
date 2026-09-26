package fx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestLoadThemeReadsColorsAndFallsBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "colors.toml")
	os.WriteFile(path, []byte("mode = \"dark\"\nbackground = \"#161b22\"\nred = '#e07b6e'\n# comment = \"#ffffff\"\n"), 0o644)
	th, err := LoadTheme(path)
	if err != nil {
		t.Fatal(err)
	}
	if th.Background != (RGB{0x16, 0x1b, 0x22}) || th.Colors["red"] != (RGB{0xe0, 0x7b, 0x6e}) {
		t.Errorf("parsed background %v red %v", th.Background, th.Colors["red"])
	}
	if _, ok := th.Colors["# comment"]; ok {
		t.Error("comment line parsed as a colour")
	}
	if th.Colors["cyan"] == (RGB{}) {
		t.Error("missing key did not fall back")
	}
	if _, err := LoadTheme(filepath.Join(dir, "missing.toml")); err == nil {
		t.Error("missing file returned no error")
	}
}

func TestGradientEndpoints(t *testing.T) {
	g := Gradient{Stops: []RGB{{0, 0, 0}, {100, 200, 250}}}
	if g.At(0) != (RGB{}) || g.At(1) != (RGB{100, 200, 250}) || g.At(2) != (RGB{100, 200, 250}) {
		t.Errorf("At(0)=%v At(1)=%v At(2)=%v", g.At(0), g.At(1), g.At(2))
	}
	if mid := g.At(0.5); mid != (RGB{50, 100, 125}) {
		t.Errorf("At(0.5)=%v", mid)
	}
}

func loadCapture(t *testing.T, name string) [][]float64 {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var frames [][]float64
	for _, line := range strings.Split(string(b), "\n") {
		var fr struct{ Bands []float64 }
		if json.Unmarshal([]byte(line), &fr) == nil && len(fr.Bands) > 0 {
			frames = append(frames, fr.Bands)
		}
	}
	return frames
}

// Replays spectrum.get captures from a real radio stream (cliamp visstream,
// 30 fps). The first capture is the one where the effects visibly did nothing:
// bass/mid barely moved and 2 beats fired in 10 s. After normalisation each
// signal must use most of its range and beats must arrive at a musical rate.
func TestAnalyzerOnRecordedRadio(t *testing.T) {
	for _, name := range []string{"dancing-through-it-10s.ndjson", "thunderstruck-20s.ndjson"} {
		frames := loadCapture(t, name)
		secs := float64(len(frames)) / 30
		var an Analyzer
		var bass, mid []float64
		beats := 0
		for i, b := range frames {
			prev := an.Beat
			a := an.Update(b, 1.0/30)
			if i < 60 { // first 2 s: the range is still being learned
				continue
			}
			bass, mid = append(bass, a.Bass), append(mid, a.Mid)
			if a.Beat == 1 && prev < 1 {
				beats++
			}
		}
		for sig, v := range map[string][]float64{"bass": bass, "mid": mid} {
			if spread := pct(v, .95) - pct(v, .05); spread < 0.5 {
				t.Errorf("%s: %s spans only %.2f of 0..1", name, sig, spread)
			}
		}
		if rate := float64(beats) / (secs - 2); rate < 0.8 || rate > 5 {
			t.Errorf("%s: %.1f beats/s (%d in %.0f s)", name, rate, beats, secs-2)
		}
		t.Logf("%s: bass p5–p95 %.2f–%.2f, mid %.2f–%.2f, %d beats in %.0f s",
			name, pct(bass, .05), pct(bass, .95), pct(mid, .05), pct(mid, .95), beats, secs-2)
	}
}

func pct(v []float64, p float64) float64 {
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return s[int(p*float64(len(s)-1))]
}

// Each effect paints every pixel from the theme and is not a flat fill.
func TestStockEffectsPaintTheWholeFrame(t *testing.T) {
	th, _ := LoadTheme("/nonexistent")
	a := Audio{Bass: .7, Mid: .5, Treble: .4, Beat: .8}
	for _, e := range Stock() {
		f := &Frame{}
		f.Resize(60, 40)
		for i := range 10 {
			e.Render(f, a, float64(i)/30, 1.0/30, th)
		}
		seen := map[RGB]bool{}
		for _, p := range f.Px {
			seen[p] = true
		}
		if len(seen) < 10 {
			t.Errorf("%s: only %d distinct colours", e.Name(), len(seen))
		}
	}
}
