package main

import (
	"strings"

	"charm.land/lipgloss/v2"
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
}

// grid is a fixed-size cell buffer. Panels draw into it by coordinate, which
// keeps a many-panel layout exact to the column in a way that joining
// lipgloss blocks is not once borders and titles share rows.
type grid struct {
	w, h int
	ch   []rune
	cl   []cls
}

func newGrid(w, h int) *grid {
	g := &grid{w: max(0, w), h: max(0, h)}
	g.ch = make([]rune, g.w*g.h)
	g.cl = make([]cls, g.w*g.h)
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
		}
		x++
	}
}

func (g *grid) set(x, y int, r rune, c cls) {
	if x >= 0 && x < g.w && y >= 0 && y < g.h {
		g.ch[y*g.w+x] = r
		g.cl[y*g.w+x] = c
	}
}

func (g *grid) paint(x, y, n int, c cls) {
	for i := range n {
		if xx := x + i; xx >= 0 && xx < g.w && y >= 0 && y < g.h {
			g.cl[y*g.w+xx] = c
		}
	}
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
		cur := cNone
		flush := func() {
			if len(run) == 0 {
				return
			}
			if cur == cNone {
				b.WriteString(string(run))
			} else {
				b.WriteString(styles[cur].Render(string(run)))
			}
			run = run[:0]
		}
		for x := range g.w {
			k := y*g.w + x
			if g.cl[k] != cur {
				flush()
				cur = g.cl[k]
			}
			run = append(run, g.ch[k])
		}
		flush()
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
