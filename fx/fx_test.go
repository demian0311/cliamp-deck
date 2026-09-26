package fx

import (
	"os"
	"path/filepath"
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

func TestAnalyzerDetectsABeatThenDecays(t *testing.T) {
	var an Analyzer
	quiet := []float64{.1, .1, .1, .2, .2, .2, .2, .1, .1, .1}
	loud := []float64{.9, .9, .9, .2, .2, .2, .2, .1, .1, .1}
	for range 60 {
		an.Update(quiet, 1.0/30)
	}
	if a := an.Update(loud, 1.0/30); a.Beat != 1 {
		t.Fatalf("no beat on a bass jump: %+v", a)
	}
	for range 15 {
		an.Update(loud, 1.0/30)
	}
	if an.Beat != 0 {
		t.Errorf("beat still %v after 0.5 s of steady bass", an.Beat)
	}
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
