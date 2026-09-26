package fx

import (
	"bufio"
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
