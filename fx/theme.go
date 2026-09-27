package fx

import (
	"bufio"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// OmarchyColors is where `omarchy theme set` leaves the applied theme. The
// whole directory is replaced on a theme change, so watch the file's mtime.
func OmarchyColors() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "omarchy", "current", "theme", "colors.toml")
}

// Theme holds the named colours of an Omarchy colors.toml. Keys are the
// colors.toml names: background, darker_background, foreground,
// bright_foreground, accent, red, orange, yellow, green, cyan, blue, magenta,
// bright_magenta, and whatever else the theme defines. Every key in fallback
// is always present.
type Theme struct {
	Name   string
	Colors map[string]RGB

	Background, Bright RGB // background and bright_foreground, used by most effects

	// Light is true when the background is brighter than the text. Effects
	// that model light on a dark stage invert on paper: bright_foreground is
	// then near-black and darker_background a grey band, so both go through
	// Stage and Glow rather than by name.
	Light bool
	// Stage is the colour an effect's empty space takes: darker_background
	// on a dark theme, the page itself on a light one.
	Stage RGB
}

// Glow pushes c toward full intensity by k (0..1): toward white-hot on a dark
// theme, and toward a deeper ink of the same hue on a light one, where
// burning toward the text colour would read as soot.
func (th *Theme) Glow(c RGB, k float64) RGB {
	if th.Light {
		return lerp(c, RGB{c.R * 3 / 5, c.G * 3 / 5, c.B * 3 / 5}, k)
	}
	return lerp(c, th.Bright, k)
}

// Heat builds a gradient that rises from Stage through the named colours to
// Glow at full intensity. The names run from coolest to hottest as a dark
// theme shows them; a light theme runs them the other way, so the faintest
// heat is the colour nearest the page and the hottest the deepest ink.
func (th *Theme) Heat(names ...string) Gradient {
	g := Gradient{Stops: []RGB{th.Stage}}
	for i := range names {
		n := names[i]
		if th.Light {
			n = names[len(names)-1-i]
		}
		g.Stops = append(g.Stops, th.Color(n))
	}
	return Gradient{Stops: append(g.Stops, th.Glow(g.Stops[len(g.Stops)-1], 1))}
}

// luminance is relative luminance (WCAG), 0..1.
func luminance(c RGB) float64 {
	ch := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
}

// Color returns a named theme colour, or the background for an unknown name.
func (th *Theme) Color(name string) RGB {
	if c, ok := th.Colors[name]; ok {
		return c
	}
	return th.Background
}

// Gradient builds a gradient through named theme colours, in order.
func (th *Theme) Gradient(names ...string) Gradient {
	g := Gradient{Stops: make([]RGB, len(names))}
	for i, n := range names {
		g.Stops[i] = th.Color(n)
	}
	return g
}

// fallback is used for any key a colors.toml omits, and when there is no
// Omarchy at all: a neutral dark palette.
var fallback = map[string]string{
	"background": "#15171c", "darker_background": "#0c0d10", "foreground": "#e2e4ea",
	"bright_foreground": "#f5f6f8", "accent": "#6c9ef8",
	"red": "#e5707a", "orange": "#e8a15c", "yellow": "#e3c86b", "green": "#8cc37a",
	"cyan": "#5cc1c9", "blue": "#6c9ef8", "magenta": "#b88ae6", "bright_magenta": "#cfa6f2",
}

// LoadTheme reads a colors.toml. A missing or unreadable file yields the
// fallback palette and the error, so the caller can still draw.
func LoadTheme(path string) (*Theme, error) {
	colors := map[string]RGB{}
	name := "default"
	f, err := os.Open(path)
	if err == nil {
		defer f.Close()
		name = filepath.Base(filepath.Dir(path))
		// Omarchy records the display name beside the theme directory.
		if b, err := os.ReadFile(filepath.Join(filepath.Dir(path), "..", "theme.name")); err == nil {
			name = strings.TrimSpace(string(b))
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			k, v, ok := strings.Cut(line, "=")
			if !ok || strings.HasPrefix(line, "#") {
				continue
			}
			if c, ok := parseHex(strings.Trim(strings.TrimSpace(v), `"'`)); ok {
				colors[strings.TrimSpace(k)] = c
			}
		}
	}
	for k, v := range fallback {
		if _, ok := colors[k]; !ok {
			colors[k], _ = parseHex(v)
		}
	}
	th := &Theme{Name: name, Colors: colors}
	th.Background, th.Bright = th.Color("background"), th.Color("bright_foreground")
	th.Light = luminance(th.Background) > luminance(th.Color("foreground"))
	th.Stage = th.Color("darker_background")
	if th.Light {
		th.Stage = th.Background
	}
	return th, err
}

func parseHex(s string) (RGB, bool) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return RGB{}, false
	}
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return RGB{}, false
	}
	return RGB{uint8(n >> 16), uint8(n >> 8), uint8(n)}, true
}

// Gradient maps 0..1 onto evenly spaced colour stops.
type Gradient struct{ Stops []RGB }

func (g Gradient) At(x float64) RGB {
	n := len(g.Stops)
	if n == 0 {
		return RGB{}
	}
	if n == 1 {
		return g.Stops[0]
	}
	p := clamp01(x) * float64(n-1)
	i := min(int(p), n-2)
	return lerp(g.Stops[i], g.Stops[i+1], p-float64(i))
}
