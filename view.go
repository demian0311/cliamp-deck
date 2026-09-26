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
	tierXS tier = iota // two lines of track over the stage: a tmux split or status pane
	tierS              // player / stage / sources stacked, the "quarter" size
	tierM              // player and sources left, stage right
	tierXL             // btop grid: player+sources · stage · levels+eq
)

func tierFor(w, h int) tier {
	switch {
	case w < 40 || h < 12:
		return tierXS
	case w < 72:
		return tierS
	case w < 150 || h < 34:
		return tierM
	}
	return tierXL
}

type rect struct{ x, y, w, h int }

func (r rect) inner() rect { return rect{r.x + 1, r.y + 1, max(0, r.w-2), max(0, r.h-2)} }

// layout places every panel for the current size. Update uses it too, to size
// the effect frame to the stage before rendering into it.
type layout struct {
	tier                   tier
	full, mini             bool
	player, sources, stage rect
	levels, eq             rect
	stageBoxed             bool
}

func (l layout) stageInner() rect {
	if l.stageBoxed {
		return l.stage.inner()
	}
	return l.stage
}

const playerH = 6

func (m model) layout() layout {
	W, body := m.w, m.h-1
	l := layout{tier: tierFor(m.w, m.h), stageBoxed: true}
	switch {
	case m.full:
		l.full, l.stageBoxed = true, false
		l.stage = rect{0, 0, W, body}
	case l.tier == tierXS:
		l.mini, l.stageBoxed = true, false
		l.stage = rect{0, 2, W, max(0, m.h-2)}
	case l.tier == tierS:
		sh := max(5, min(8, body*3/10))
		l.player = rect{0, 0, W, playerH}
		l.sources = rect{0, body - sh, W, sh}
		l.stage = rect{0, playerH, W, max(0, body-sh-playerH)}
	case l.tier == tierM:
		lw := min(48, W*38/100)
		l.player = rect{0, 0, lw, playerH}
		l.sources = rect{0, playerH, lw, body - playerH}
		l.stage = rect{lw, 0, W - lw, body}
	default:
		const lw, rw = 50, 40
		l.player = rect{0, 0, lw, playerH}
		l.sources = rect{0, playerH, lw, body - playerH}
		l.stage = rect{lw, 0, W - lw - rw, body}
		l.levels = rect{W - rw, 0, rw, body / 2}
		l.eq = rect{W - rw, body / 2, rw, body - body/2}
	}
	return l
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
	l := m.layout()
	m.drawStage(g, l)
	switch {
	case l.full:
		m.drawOverlay(g, m.h-1)
		return g.String()
	case l.mini:
		m.drawTrackLines(g, 0, 0, m.w, 2)
		return g.String()
	}
	m.drawPlayer(g, l.player)
	if l.sources.h >= 3 {
		m.drawSources(g, l.sources.x, l.sources.y, l.sources.w, l.sources.h)
	}
	if l.levels.w > 0 {
		m.drawLevels(g, l.levels.x, l.levels.y, l.levels.w, l.levels.h)
		m.drawEQ(g, l.eq.x, l.eq.y, l.eq.w, l.eq.h)
	}
	m.drawStatus(g, m.h-1)
	return g.String()
}

func (m model) drawStage(g *grid, l layout) {
	r := l.stage
	if r.w < 2 || r.h < 1 {
		return
	}
	if m.mode == 0 {
		m.drawSpectrum(g, r.x, r.y, r.w, r.h, l.stageBoxed)
		return
	}
	if l.stageBoxed {
		g.box(r.x, r.y, r.w, r.h, "²stage · "+m.modeName(), cAmber)
	}
	in := l.stageInner()
	if m.frame.W == in.w && m.frame.H == in.h*2 {
		g.blit(in.x, in.y, m.frame)
	}
}

func (m model) trackText() (title, source string) {
	t := m.snap.Track
	if t == nil {
		return "nothing playing — pick a source", ""
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

func (m model) stateGlyph() (string, cls) {
	switch m.snap.State {
	case "playing":
		return "►", cGreen
	case "paused":
		return "‖", cYellow
	}
	return "■", cDim
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

// drawTrackLines writes the title line and, when there is room, the time line.
func (m model) drawTrackLines(g *grid, x, y, w, rows int) {
	if w < 4 || rows < 1 {
		return
	}
	m.drawTitleLine(g, x, y, w)
	if rows >= 2 {
		m.drawTimeLine(g, x, y+1, w)
	}
}

func (m model) drawTitleLine(g *grid, x, y, w int) {
	glyph, gc := m.stateGlyph()
	title, _ := m.trackText()
	g.put(x, y, glyph, gc)
	g.put(x+2, y, m.marquee(title, max(1, w-2)), cWhite)
}

// drawTimeLine is a thin progress bar for a track. A stream has no length, so
// it gets LIVE and how long it has been on air instead.
func (m model) drawTimeLine(g *grid, x, y, w int) {
	pos, dur := m.position(), m.snap.Duration
	if m.live() {
		g.put(x, y, "● LIVE", cRed)
		g.put(x+8, y, fit("on air "+mmss(pos), w-8), cDim)
		return
	}
	bw := max(2, w-12)
	p := 0
	if dur > 0 {
		p = int(math.Round(pos / dur * float64(bw-1)))
	}
	g.put(x, y, mmss(pos), cGreen)
	g.put(x+6, y, strings.Repeat("━", p), cAmber)
	g.put(x+6+p, y, "●", cWhite)
	g.put(x+7+p, y, strings.Repeat("─", max(0, bw-p-1)), cDim)
	if w >= 12 {
		g.put(x+w-5, y, mmss(dur), cDim)
	}
}

// drawPlayer rows: title, source, time, transport.
func (m model) drawPlayer(g *grid, r rect) {
	g.box(r.x, r.y, r.w, r.h, "¹player", cGreen)
	ix, iw, iy, ih := r.x+2, r.w-4, r.y+1, r.h-2
	if iw < 4 || ih < 1 {
		return
	}
	m.drawTitleLine(g, ix, iy, iw)
	if ih >= 2 {
		_, source := m.trackText()
		parts := []string{}
		for _, p := range []string{source, m.modeLine()} {
			if p != "" {
				parts = append(parts, p)
			}
		}
		g.put(ix+2, iy+1, fit(strings.Join(parts, " · "), iw-2), cCyan)
	}
	if ih >= 3 {
		m.drawTimeLine(g, ix, iy+2, iw)
	}
	if ih >= 4 {
		g.put(ix, iy+3, "◄◄  ►  ‖  ■  ►►", cWhite)
		if vw := min(10, iw-28); vw > 3 {
			n := int(math.Round(clamp01((m.snap.Volume+30)/36) * float64(vw)))
			vol := fmt.Sprintf("vol %s%s %+.0fdB", strings.Repeat("▰", n), strings.Repeat("▱", vw-n), m.snap.Volume)
			g.put(ix+iw-len([]rune(vol)), iy+3, vol, cCyan)
		}
	}
}

func (m model) drawOverlay(g *grid, y int) {
	glyph, gc := m.stateGlyph()
	title, source := m.trackText()
	hint := "  v " + m.modeName() + " · V exit"
	g.put(1, y, glyph, gc)
	text := title
	if source != "" {
		text += " · " + source
	}
	room := max(1, g.w-4-len([]rune(hint)))
	g.put(3, y, m.marquee(text, room), cWhite)
	g.put(g.w-len([]rune(hint))-1, y, hint, cDim)
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
		if cw >= 4 || i%2 == 0 { // three-cell columns can't fit ten 3-char labels
			g.put(ix+4+i*cw+max(0, (cw-len(lab))/2), iy+ih, lab, cDim)
		}
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
		keys := [][2]string{{"␣", "play"}, {"n/p", "skip"}, {"←→", "seek"}, {"+/-", "vol"}, {"v", "fx"}, {"V", "full"}, {"⏎", "open"}, {"esc", "back"}, {"q", "quit"}}
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
