package main

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/demian0311/cliamp-deck/fx"
)

// cls is a cell's colour role. Roles map to the terminal's ANSI palette so the
// deck follows whatever theme the terminal (and Omarchy) is wearing.
type cls uint8

const (
	cNone cls = iota
	cGreen
	cYellow
	cAmber
	cRed
	cCyan
	cMagenta
	cDim
	cWhite
	cTitle
	cSel
	cKey
	cFocus
)

var styles = map[cls]lipgloss.Style{
	cGreen:   lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
	cYellow:  lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
	cAmber:   lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
	cRed:     lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
	cCyan:    lipgloss.NewStyle().Foreground(lipgloss.Color("6")),
	cMagenta: lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
	cDim:     lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
	cWhite:   lipgloss.NewStyle().Foreground(lipgloss.Color("15")),
	cTitle:   lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true),
	cSel:     lipgloss.NewStyle().Reverse(true),
	cKey:     lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true),
	cFocus:   lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true),
}

// grid is a fixed-size cell buffer. Panels draw into it by coordinate, which
// keeps a many-panel layout exact to the column in a way that joining
// lipgloss blocks is not once borders and titles share rows.
type grid struct {
	w, h int
	ch   []rune
	cl   []cls
	// Pixel cells: a ▀ whose foreground is the top pixel and background the
	// bottom one. Emitted as raw 24-bit SGR; lipgloss per cell is too slow for
	// a few thousand cells at 30 fps.
	px       []bool
	top, bot []fx.RGB
	// Shaded cells keep their colour on a subtle background: the selected
	// list row. shade is that background, a lipgloss colour string.
	shaded []bool
	shade  string
}

func newGrid(w, h int) *grid {
	g := &grid{w: max(0, w), h: max(0, h)}
	g.ch = make([]rune, g.w*g.h)
	g.cl = make([]cls, g.w*g.h)
	g.px = make([]bool, g.w*g.h)
	g.top = make([]fx.RGB, g.w*g.h)
	g.bot = make([]fx.RGB, g.w*g.h)
	g.shaded = make([]bool, g.w*g.h)
	g.shade = "236"
	for i := range g.ch {
		g.ch[i] = ' '
	}
	return g
}

func (g *grid) put(x, y int, s string, c cls) {
	if y < 0 || y >= g.h {
		return
	}
	for _, r := range s {
		if x >= 0 && x < g.w {
			g.ch[y*g.w+x] = r
			g.cl[y*g.w+x] = c
			g.px[y*g.w+x] = false
		}
		x++
	}
}

func (g *grid) set(x, y int, r rune, c cls) {
	if x >= 0 && x < g.w && y >= 0 && y < g.h {
		g.ch[y*g.w+x] = r
		g.cl[y*g.w+x] = c
		g.px[y*g.w+x] = false
	}
}

// blit copies a pixel frame (2 pixel rows per cell row) to cells at x, y.
func (g *grid) blit(x, y int, f *fx.Frame) {
	for r := 0; r*2+1 < f.H; r++ {
		for c := range f.W {
			xx, yy := x+c, y+r
			if xx < 0 || xx >= g.w || yy < 0 || yy >= g.h {
				continue
			}
			k := yy*g.w + xx
			g.px[k], g.ch[k] = true, '▀'
			g.top[k], g.bot[k] = f.Px[r*2*f.W+c], f.Px[(r*2+1)*f.W+c]
		}
	}
}

func (g *grid) paint(x, y, n int, c cls) {
	for i := range n {
		if xx := x + i; xx >= 0 && xx < g.w && y >= 0 && y < g.h {
			g.cl[y*g.w+xx] = c
		}
	}
}

// shadeRow puts n cells from x, y on the selection background.
func (g *grid) shadeRow(x, y, n int) {
	for i := range n {
		if xx := x + i; xx >= 0 && xx < g.w && y >= 0 && y < g.h {
			g.shaded[y*g.w+xx] = true
		}
	}
}

// useTheme shades selections a step from the theme's background toward its
// foreground, so the highlight reads as a tint in any theme.
func (g *grid) useTheme(th *fx.Theme) {
	if th == nil {
		return
	}
	bg, fg := th.Background, th.Bright
	mix := func(a, b uint8) uint8 { return uint8(int(a) + (int(b)-int(a))*14/100) }
	g.shade = fmt.Sprintf("#%02x%02x%02x", mix(bg.R, fg.R), mix(bg.G, fg.G), mix(bg.B, fg.B))
}

func (g *grid) box(x, y, w, h int, title string, c cls) {
	if w < 4 || h < 2 {
		return
	}
	g.put(x, y, "╭"+strings.Repeat("─", w-2)+"╮", c)
	for yy := y + 1; yy < y+h-1; yy++ {
		g.set(x, yy, '│', c)
		g.set(x+w-1, yy, '│', c)
	}
	g.put(x, y+h-1, "╰"+strings.Repeat("─", w-2)+"╯", c)
	title = fit(title, w-6)
	if title != "" {
		n := len([]rune(title))
		g.set(x+2, y, '┤', c)
		g.put(x+3, y, title, cTitle)
		g.set(x+3+n, y, '├', c)
	}
}

func (g *grid) String() string {
	var b strings.Builder
	for y := range g.h {
		run := make([]rune, 0, g.w)
		cur, curShaded := cNone, false
		inPx := false
		var lt, lb fx.RGB
		flush := func() {
			if len(run) == 0 {
				return
			}
			switch {
			case curShaded:
				st, ok := styles[cur]
				if !ok {
					st = lipgloss.NewStyle()
				}
				b.WriteString(st.Background(lipgloss.Color(g.shade)).Render(string(run)))
			case cur == cNone:
				b.WriteString(string(run))
			default:
				b.WriteString(styles[cur].Render(string(run)))
			}
			run = run[:0]
		}
		for x := range g.w {
			k := y*g.w + x
			if g.px[k] {
				flush()
				if !inPx || g.top[k] != lt || g.bot[k] != lb {
					writeSGR(&b, g.top[k], g.bot[k])
					lt, lb, inPx = g.top[k], g.bot[k], true
				}
				b.WriteRune('▀')
				continue
			}
			if inPx {
				b.WriteString("\x1b[0m")
				inPx = false
			}
			if g.cl[k] != cur || g.shaded[k] != curShaded {
				flush()
				cur, curShaded = g.cl[k], g.shaded[k]
			}
			run = append(run, g.ch[k])
		}
		flush()
		if inPx {
			b.WriteString("\x1b[0m")
		}
		if y < g.h-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// fit truncates s to w cells with an ellipsis. Every glyph the deck draws is
// single-width, so runes and cells agree.
func fit(s string, w int) string {
	r := []rune(s)
	if w <= 0 {
		return ""
	}
	if len(r) <= w {
		return s
	}
	return string(r[:w-1]) + "…"
}

func writeSGR(b *strings.Builder, fg, bg fx.RGB) {
	var buf [48]byte
	o := append(buf[:0], "\x1b[38;2;"...)
	o = strconv.AppendUint(o, uint64(fg.R), 10)
	o = append(o, ';')
	o = strconv.AppendUint(o, uint64(fg.G), 10)
	o = append(o, ';')
	o = strconv.AppendUint(o, uint64(fg.B), 10)
	o = append(o, ";48;2;"...)
	o = strconv.AppendUint(o, uint64(bg.R), 10)
	o = append(o, ';')
	o = strconv.AppendUint(o, uint64(bg.G), 10)
	o = append(o, ';')
	o = strconv.AppendUint(o, uint64(bg.B), 10)
	o = append(o, 'm')
	b.Write(o)
}
