package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type tier int

const (
	tierXS tier = iota // three lines: a tmux split or status pane
	tierS              // classic Winamp stack
	tierM              // player + spectrum over sources
	tierL              // wider player, full-width sources
	tierXL             // btop grid: player · spectrum · levels / eq · sources
)

func tierFor(w, h int) tier {
	switch {
	case w < 40 || h < 9:
		return tierXS
	case w < 72:
		return tierS
	case w < 110:
		return tierM
	case w < 150 || h < 34:
		return tierL
	}
	return tierXL
}

func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "cliamp-deck"
	return v
}

func (m model) render() string {
	if m.w <= 0 || m.h <= 0 {
		return ""
	}
	g := newGrid(m.w, m.h)
	if m.snap == nil {
		msg := "waiting for cliamp on " + m.c.sock + " …"
		g.put(max(0, (m.w-len([]rune(msg)))/2), m.h/2, fit(msg, m.w), cDim)
		return g.String()
	}
	W, H, body := m.w, m.h, m.h-1
	switch tierFor(W, H) {
	case tierXS:
		m.drawMini(g)
		return g.String()
	case tierS:
		ph := min(6, body)
		m.drawPlayer(g, 0, 0, W, ph)
		sh := max(4, min(9, int(float64(body)*0.3)))
		if body-ph >= sh {
			m.drawSpectrum(g, 0, ph, W, sh, true)
		} else {
			sh = 0
		}
		if body-ph-sh >= 3 {
			m.drawSources(g, 0, ph+sh, W, body-ph-sh)
		}
	default:
		t := tierFor(W, H)
		top := max(9, min(14, int(float64(body)*0.4)))
		pw := map[tier]int{tierM: min(52, W*58/100), tierL: 54, tierXL: 58}[t]
		lw := 0
		if t == tierXL {
			lw = 36
		}
		m.drawPlayer(g, 0, 0, pw, top)
		m.drawSpectrum(g, pw, 0, W-pw-lw, top, true)
		if lw > 0 {
			m.drawLevels(g, W-lw, 0, lw, top)
		}
		if bh := body - top; bh >= 3 {
			if t == tierXL {
				const ew = 50
				m.drawEQ(g, 0, top, ew, bh)
				m.drawSources(g, ew, top, W-ew, bh)
			} else {
				m.drawSources(g, 0, top, W, bh)
			}
		}
	}
	m.drawStatus(g, H-1)
	return g.String()
}

var ledGlyphs = map[rune][3]string{
	'0': {"█▀█", "█ █", "▀▀▀"}, '1': {" ▀█", "  █", "  ▀"}, '2': {"▀▀█", "█▀▀", "▀▀▀"},
	'3': {"▀▀█", " ▀█", "▀▀▀"}, '4': {"█ █", "▀▀█", "  ▀"}, '5': {"█▀▀", "▀▀█", "▀▀▀"},
	'6': {"█▀▀", "█▀█", "▀▀▀"}, '7': {"▀▀█", "  █", "  ▀"}, '8': {"█▀█", "█▀█", "▀▀▀"},
	'9': {"█▀█", "▀▀█", "▀▀▀"}, ':': {" ", "▪", " "},
}

func (m model) trackText() (title, source string) {
	t := m.snap.Track
	if t == nil {
		return "nothing playing — pick a source ↓", ""
	}
	title = t.Title
	if t.Artist != "" {
		title = t.Artist + " — " + t.Title
	}
	source = t.Album
	if t.Station != "" {
		source = t.Station
	}
	return title, source
}

func (m model) stateLabel() (string, cls) {
	switch m.snap.State {
	case "playing":
		return "► PLAYING", cGreen
	case "paused":
		return "‖ PAUSED", cYellow
	}
	return "■ STOPPED", cDim
}

func (m model) live() bool {
	t := m.snap.Track
	return t != nil && (t.Realtime || (t.Stream && m.snap.Duration == 0))
}

func (m model) marquee(text string, w int) string {
	r := []rune(text)
	if len(r) <= w {
		return text
	}
	r = append(r, []rune("   ·   ")...)
	off := int(m.marq) % len(r)
	out := make([]rune, w)
	for i := range out {
		out[i] = r[(off+i)%len(r)]
	}
	return string(out)
}

func (m model) drawPlayer(g *grid, x, y, w, h int) {
	g.box(x, y, w, h, "¹player", cGreen)
	ix, iw, iy, ih := x+2, w-4, y+1, h-2
	if iw < 4 || ih < 1 {
		return
	}
	pos, dur := m.position(), m.snap.Duration
	title, source := m.trackText()
	label, lc := m.stateLabel()
	row := iy
	if ih >= 6 && iw >= 36 {
		cx := ix
		for _, ch := range mmss(pos) {
			gl := ledGlyphs[ch]
			for r := range 3 {
				g.put(cx, iy+r, gl[r], cGreen)
			}
			cx += len([]rune(gl[0])) + 1
		}
		sx, sw := ix+21, iw-21
		g.put(sx, iy, label, lc)
		g.put(sx, iy+1, fit(source, sw), cCyan)
		g.put(sx, iy+2, fit(m.modeLine(), sw), cDim)
		row = iy + 3
		if ih >= 7 {
			row++
		}
	} else {
		g.put(ix, row, fit(label+"  "+source, iw), lc)
		row++
	}
	if row < iy+ih {
		g.put(ix, row, m.marquee(title, iw), cWhite)
		row++
	}
	if row < iy+ih {
		bw := max(4, iw-14)
		g.put(ix, row, mmss(pos), cDim)
		if m.live() {
			g.put(ix+6, row, "● LIVE "+strings.Repeat("·", max(0, bw-7)), cRed)
		} else {
			p := 0
			if dur > 0 {
				p = int(math.Round(pos / dur * float64(bw-1)))
			}
			g.put(ix+6, row, strings.Repeat("━", p), cAmber)
			g.put(ix+6+p, row, "●", cWhite)
			g.put(ix+7+p, row, strings.Repeat("─", max(0, bw-p-1)), cDim)
			g.put(ix+7+bw, row, mmss(dur), cDim)
		}
		row++
	}
	if row < iy+ih {
		g.put(ix, row, "◄◄  ►  ‖  ■  ►►", cWhite)
		if vw := min(10, iw-28); vw > 3 {
			frac := (m.snap.Volume + 30) / 36
			n := int(math.Round(clamp01(frac) * float64(vw)))
			vol := fmt.Sprintf("vol %s%s %+.0fdB", strings.Repeat("▰", n), strings.Repeat("▱", vw-n), m.snap.Volume)
			g.put(ix+iw-len([]rune(vol)), row, vol, cCyan)
		}
	}
}

func (m model) modeLine() string {
	parts := []string{}
	if m.snap.Shuffle != nil && *m.snap.Shuffle {
		parts = append(parts, "shuffle")
	}
	if m.snap.Repeat != "" && m.snap.Repeat != "Off" {
		parts = append(parts, "repeat "+strings.ToLower(m.snap.Repeat))
	}
	if m.snap.EQPreset != "" {
		parts = append(parts, "eq "+strings.ToLower(m.snap.EQPreset))
	}
	if m.snap.Total > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d", m.snap.Index+1, m.snap.Total))
	}
	return strings.Join(parts, " · ")
}

// sample picks the value for dot column i of n from a higher-resolution curve.
func sample(v []float64, i, n int) float64 {
	if len(v) == 0 {
		return 0
	}
	if n <= 1 {
		return v[0]
	}
	return v[i*(len(v)-1)/(n-1)]
}

func heightClass(frac float64) cls {
	switch {
	case frac < 0.45:
		return cGreen
	case frac < 0.7:
		return cYellow
	case frac < 0.88:
		return cAmber
	}
	return cRed
}

func (m model) drawSpectrum(g *grid, x, y, w, h int, boxed bool) {
	ix, iy, iw, ih := x, y, w, h
	if boxed {
		g.box(x, y, w, h, "²spectrum", cAmber)
		ix, iy, iw, ih = x+2, y+1, w-4, h-2
		if axis := "70 · 320 · 1k · 6k · 16k Hz"; w > len([]rune(axis))+20 {
			g.put(x+w-len([]rune(axis))-3, y+h-1, axis, cDim)
		}
	}
	if iw < 1 || ih < 1 {
		return
	}
	dots, H := iw*2, float64(ih*4)
	for cx := range iw {
		a := int(math.Round(sample(m.hi, cx*2, dots) * H))
		b := int(math.Round(sample(m.hi, cx*2+1, dots) * H))
		pa := int(math.Round(sample(m.pk.v, cx*2, dots) * H))
		pb := int(math.Round(sample(m.pk.v, cx*2+1, dots) * H))
		for r := range ih {
			ch := brailleCell(a, b, pa, pb, r)
			if ch == 0x2800 {
				continue
			}
			c := heightClass(float64(r) / float64(ih))
			if a <= r*4 && b <= r*4 {
				c = cWhite // only peak caps in this cell
			}
			g.set(ix+cx, iy+ih-1-r, ch, c)
		}
	}
}

// drawGraph plots a scrolling history as a filled braille area, newest right.
func drawGraph(g *grid, x, y, w, h int, data []float64, gain float64) {
	if w < 1 || h < 1 {
		return
	}
	need := w * 2
	d := make([]float64, need)
	if len(data) > need {
		data = data[len(data)-need:]
	}
	copy(d[need-len(data):], data)
	H := float64(h * 4)
	for cx := range w {
		a := int(math.Round(clamp01(d[cx*2]*gain) * H))
		b := int(math.Round(clamp01(d[cx*2+1]*gain) * H))
		for r := range h {
			if ch := brailleCell(a, b, 0, 0, r); ch != 0x2800 {
				g.set(x+cx, y+h-1-r, ch, heightClass(float64(r)/float64(h)))
			}
		}
	}
}

func (m model) drawLevels(g *grid, x, y, w, h int) {
	g.box(x, y, w, h, "⁵levels", cMagenta)
	ix, iw, iy, ih := x+2, w-4, y+1, h-2
	if iw < 8 || ih < 1 {
		return
	}
	lvl := 0.0
	if len(m.hist) > 0 {
		lvl = clamp01(m.hist[len(m.hist)-1] * 2)
	}
	mw := iw - 2
	n := int(math.Round(lvl * float64(mw)))
	g.put(ix, iy, "▕", cDim)
	g.put(ix+1, iy, strings.Repeat("█", n), heightClass(lvl*0.95))
	g.put(ix+1+n, iy, strings.Repeat("░", mw-n), cDim)
	if ih > 2 {
		drawGraph(g, ix, iy+2, iw, ih-2, m.hist, 2)
	}
}

var eqLabels = [10]string{"70", "180", "320", "600", "1k", "3k", "6k", "12k", "14k", "16k"}

func (m model) drawEQ(g *grid, x, y, w, h int) {
	title := "³eq"
	if m.snap.EQPreset != "" {
		title += " · " + strings.ToLower(m.snap.EQPreset)
	}
	g.box(x, y, w, h, title, cCyan)
	ix, iw, iy, ih := x+2, w-4, y+1, h-3
	if ih < 3 || iw < 30 {
		return
	}
	mid, half := iy+ih/2, ih/2
	cw := max(3, (iw-4)/len(eqLabels))
	g.put(ix, iy, "+12", cDim)
	g.put(ix+1, mid, " 0", cDim)
	g.put(ix, iy+ih-1, "-12", cDim)
	g.put(ix+4, mid, strings.Repeat("┄", min(iw-4, cw*len(eqLabels))), cDim)
	for i, lab := range eqLabels {
		gain := 0.0
		if i < len(m.snap.EQBands) {
			gain = m.snap.EQBands[i]
		}
		cx := ix + 4 + i*cw + (cw-2)/2
		n := int(math.Round(math.Abs(gain) / 12 * float64(half)))
		for k := 1; k <= n; k++ {
			if gain > 0 {
				g.put(cx, mid-k, "██", heightClass(0.3+0.7*float64(k)/float64(half)))
			} else {
				g.put(cx, mid+k, "██", cCyan)
			}
		}
		if gain >= 0 {
			g.put(cx, mid-n-1, "▬▬", cWhite)
		} else {
			g.put(cx, mid+n+1, "▬▬", cWhite)
		}
		g.put(ix+4+i*cw+max(0, (cw-len(lab))/2), iy+ih, lab, cDim)
	}
}

func (m model) drawSources(g *grid, x, y, w, h int) {
	title := "⁴sources"
	name := ""
	if m.provider != "" {
		name = m.provider
		for _, p := range m.providers {
			if p.Key == m.provider {
				name = p.Name
			}
		}
		title += " › " + name
	}
	if m.loading {
		title += " …"
	}
	g.box(x, y, w, h, title, cYellow)
	ix, iw, iy, ih := x+1, w-2, y+1, h-2
	if ih < 1 || iw < 6 {
		return
	}
	if len(m.items) == 0 {
		g.put(ix+1, iy, fit("no sources yet", iw-1), cDim)
		return
	}
	n := len(m.items)
	start := 0
	if n > ih {
		start = max(0, min(n-ih, m.sel-ih/2))
	}
	playing := m.snap.Playlist
	for r := 0; r < min(ih, n); r++ {
		i := start + r
		it := m.items[i]
		on := (m.provider == "" && strings.HasPrefix(playing, it.key+":")) ||
			(m.provider != "" && playing == m.provider+":"+it.key)
		mark := "  "
		if on {
			mark = "► "
		}
		right := ""
		if it.section != "" && iw > 50 {
			right = it.section + " "
		}
		left := " " + mark + fit(it.name, max(1, iw-3-len([]rune(right))-1))
		line := left + strings.Repeat(" ", max(0, iw-len([]rune(left))-len([]rune(right)))) + right
		c := cNone
		if i == m.sel {
			c = cSel
		}
		g.put(ix, iy+r, line, c)
		if i != m.sel {
			if on {
				g.paint(ix, iy+r, iw, cGreen)
			}
			g.paint(ix+iw-len([]rune(right)), iy+r, len([]rune(right)), cDim)
		}
	}
	if n > ih {
		th := max(1, ih*ih/n)
		tp := start * (ih - th) / (n - ih)
		for k := range th {
			g.set(x+w-1, iy+tp+k, '┃', cYellow)
		}
	}
}

func (m model) drawMini(g *grid) {
	title, _ := m.trackText()
	label, lc := m.stateLabel()
	g.put(0, 0, string([]rune(label)[:1])+" ", lc)
	g.put(2, 0, m.marquee(title, max(1, g.w-2)), cWhite)
	if g.h > 1 {
		g.put(0, 1, mmss(m.position())+" ", cGreen)
		bw := max(2, g.w-6)
		if m.live() {
			g.put(6, 1, fit("● LIVE", bw), cRed)
		} else if m.snap.Duration > 0 {
			p := int(float64(bw) * m.position() / m.snap.Duration)
			g.put(6, 1, strings.Repeat("━", p), cAmber)
			g.put(6+p, 1, strings.Repeat("─", max(0, bw-p)), cDim)
		}
	}
	if g.h > 2 {
		m.drawSpectrum(g, 0, 2, g.w, g.h-2, false)
	}
}

func (m model) drawStatus(g *grid, y int) {
	right := "● cliamp"
	rc := cGreen
	if !m.online {
		right, rc = "○ cliamp offline", cRed
	}
	room := g.w - len([]rune(right)) - 2
	if m.note != "" && time.Since(m.noteAt) < 5*time.Second {
		g.put(1, y, fit(m.note, room), cYellow)
	} else {
		keys := [][2]string{{"␣", "play"}, {"n/p", "skip"}, {"←→", "seek"}, {"+/-", "vol"}, {"⏎", "open"}, {"esc", "back"}, {"q", "quit"}}
		x := 1
		for _, k := range keys {
			seg := len([]rune(k[0])) + 1 + len([]rune(k[1])) + 2
			if x+seg > room {
				break
			}
			g.put(x, y, k[0], cKey)
			g.put(x+len([]rune(k[0]))+1, y, k[1], cDim)
			x += seg
		}
	}
	g.put(g.w-len([]rune(right))-1, y, right, rc)
}
