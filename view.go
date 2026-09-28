package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/demian0311/cliamp-deck/fx"
)

func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "cliamp-deck"
	v.MouseMode = tea.MouseModeCellMotion
	if m.take {
		v.MouseMode = tea.MouseModeAllMotion // any movement brings the track bar back
	}
	return v
}

func (m model) render() string {
	if m.w <= 0 || m.h <= 0 {
		return ""
	}
	g := newGrid(m.w, m.h)
	g.useTheme(m.theme)
	if m.snap == nil {
		msg := "waiting for cliamp on " + m.c.sock + " …"
		g.put(max(0, (m.w-len([]rune(msg)))/2), m.h/2, fit(msg, m.w), cDim)
		return g.String()
	}
	l := m.layout()
	if !l.vis.empty() {
		m.drawVisual(g, l)
	}
	switch l.shape {
	case shapeTakeover:
		if time.Since(m.lastAct) < barLinger {
			m.drawTakeoverBar(g, m.h-1)
		}
		return g.String()
	case shapeMini:
		if l.miniTitle >= 0 {
			m.drawTitleLine(g, 0, 0, m.w)
			m.drawTimeLine(g, 0, 1, m.w)
		}
	default:
		m.drawPlayer(g, l.player)
		if !l.eq.empty() {
			m.drawEQ(g, l.eq)
		}
	}
	if !l.sources.empty() {
		m.drawSources(g, l.sources)
	}
	m.drawStatus(g, l.status)
	return g.String()
}

// frame draws a panel border, highlighted when the panel has keyboard focus;
// the focused panel's title sits on an accent tint.
func (m model) drawFrame(g *grid, r rect, title string, c cls, area focusArea) {
	tc := cTitle
	if m.has(area) {
		c, tc = cFocus, cSel
	}
	g.box(r.x, r.y, r.w, r.h, title, c, tc)
}

// has reports whether keys go to area. The EQ belongs to the player: while it
// is open, the player's keys drive it, so it rather than the player looks
// focused.
func (m model) has(area focusArea) bool {
	if area == focusPlayer && m.eqOpen {
		return false
	}
	return m.focus == area
}

// eqFocused reports whether the open EQ takes the keys.
func (m model) eqFocused() bool { return m.eqOpen && m.focus == focusPlayer }

func (m model) drawVisual(g *grid, l layout) {
	in := l.visInner()
	if l.visBoxed {
		m.drawFrame(g, l.vis, m.modeName(), cAmber, focusVis)
		if prev, _ := visControls(l); !prev.empty() {
			label := fmt.Sprintf("‹ %d/%d ›", m.mode+1, len(m.effects))
			g.put(prev.x-1, prev.y, " "+label+" ", cWhite)
			g.paint(prev.x+2, prev.y, len([]rune(label))-4, cDim)
		}
	}
	if in.empty() {
		return
	}
	if _, ok := m.effects[m.mode].(fx.GlyphEffect); ok {
		m.drawCells(g, in)
		return
	}
	if m.frame.W == in.w && m.frame.H == in.h*2 {
		g.blit(in.x, in.y, m.frame)
	}
}

// drawCells puts a glyph effect's cells on screen, each in its own colour.
func (m model) drawCells(g *grid, r rect) {
	c := m.cells
	if c.W != r.w || c.H != r.h {
		return
	}
	for y := range c.H {
		for x := range c.W {
			if ch := c.Ch[y*c.W+x]; ch != 0 {
				g.setRGB(r.x+x, r.y+y, ch, c.Fg[y*c.W+x])
			}
		}
	}
}

func (m model) drawTakeoverBar(g *grid, y int) {
	title, source := m.trackText()
	glyph, gc := m.stateGlyph()
	hint := fmt.Sprintf("‹ %s › · click or esc to return", m.modeName())
	g.put(0, y, strings.Repeat(" ", g.w), cNone)
	g.put(1, y, glyph, gc)
	text := title
	if source != "" {
		text += " · " + source
	}
	gw := len([]rune(glyph)) // the plain pause is two cells
	room := max(1, g.w-4-gw-len([]rune(hint)))
	g.put(2+gw, y, m.marquee(text, room), cWhite)
	g.put(g.w-len([]rune(hint))-1, y, hint, cDim)
}

// drawPlayer rows: title with the level meter, source, time line, transport.
func (m model) drawPlayer(g *grid, r rect) {
	m.drawFrame(g, r, "player", cGreen, focusPlayer)
	ix, iw, iy, ih := r.x+2, r.w-4, r.y+1, r.h-2
	if iw < 4 || ih < 1 {
		return
	}
	mw := 0 // the meter's width, plus the gap before it
	meterX, meterW := 0, 0
	volTop := m.spectrumShown() && iw >= 24 // the spectrum panel is the meter; volume takes its place
	if (iw > 34 && ih >= 2) || volTop {
		meterW = min(24, max(6, iw/4))
		meterX = ix + iw - meterW
		mw = meterW + 1
	}
	switch {
	case volTop:
		g.put(meterX-4, iy, "vol", cDim)
		m.drawVolume(g, meterX, iy, meterW, m.snap.Volume)
		mw += 4
	case meterW > 0:
		m.drawMeter(g, meterX, iy, meterW)
	}
	m.drawTitleLine(g, ix, iy, iw-mw)
	if ih == 2 { // two rows: the time matters more than the source
		tw := iw - mw // beside the meter's second row
		if volTop {
			tw = iw // the volume is one row
		}
		m.drawTimeLine(g, ix, iy+1, tw)
		return
	}
	if ih >= 2 {
		_, source := m.trackText()
		parts := []string{}
		for _, p := range []string{source, m.modeLine()} {
			if p != "" {
				parts = append(parts, p)
			}
		}
		sw := iw - 2 - mw
		if volTop {
			sw = iw - 2 // no meter beside it
		}
		g.put(ix+2, iy+1, m.marquee(strings.Join(parts, " · "), max(1, sw)), cCyan)
	}
	if ih >= 3 {
		m.drawTimeLine(g, ix, iy+2, iw)
	}
	if ih >= 4 {
		end := ix
		for _, b := range m.transport(r) {
			g.put(b.at.x, b.at.y, b.glyph, b.colour)
			end = b.at.x + b.at.w
		}
		switch vw := min(12, iw-(end-ix)-6); {
		case meterW > 0 && !volTop: // under the meter, the same width, so the two line up
			g.put(meterX-4, iy+3, "vol", cDim)
			m.drawVolume(g, meterX, iy+3, meterW, m.snap.Volume)
		case meterW == 0 && vw >= 4: // too narrow for the meter: a short bar beside the transport
			g.put(ix+iw-vw-4, iy+3, "vol", cDim)
			m.drawVolume(g, ix+iw-vw, iy+3, vw, m.snap.Volume)
		}
	}
}

// drawMeter is a braille meter two text rows high with a bar per dot row:
// highs on top, lows at the bottom (see updateLevels), coloured by how far
// each cell reaches, with white peak dots trailing back to the bars.
func (m model) drawMeter(g *grid, x, y, w int) {
	for row := range meterRanges / 4 {
		fills, peaks := make([]float64, 4), make([]float64, 4)
		for d := range 4 {
			r := meterRanges - 1 - row*4 - d
			fills[d], peaks[d] = m.levels[r], m.levelPeak[r]
		}
		cells, peakOnly := brailleBars(fills, peaks, w)
		for i, ch := range cells {
			switch {
			case ch == 0x2800:
				g.set(x+i, y+row, '⣀', cDim)
			case peakOnly[i]:
				g.set(x+i, y+row, ch, cWhite)
			default:
				m.setRamp(g, x+i, y+row, ch, float64(i)/float64(max(1, w-1)))
			}
		}
	}
}

// drawVolume is the volume as a braille bar in the meter's style and colours,
// full height, across -30..+6 dB.
func (m model) drawVolume(g *grid, x, y, w int, db float64) {
	f := clamp01((db + 30) / 36)
	cells, _ := brailleBars([]float64{f, f, f, f}, nil, w)
	for i, ch := range cells {
		if ch == 0x2800 {
			g.set(x+i, y, '⣀', cDim)
			continue
		}
		m.setRamp(g, x+i, y, ch, float64(i)/float64(max(1, w-1)))
	}
}

// rampStops are the theme colours the meter, volume and EQ run through.
var rampStops = []string{"green", "cyan", "blue", "magenta", "red"}

// setRamp writes ch in the colour at pos (0..1) along rampStops, or along the
// ANSI height classes when no theme is loaded.
func (m model) setRamp(g *grid, x, y int, ch rune, pos float64) {
	if m.theme == nil {
		g.set(x, y, ch, heightClass(pos*0.95))
		return
	}
	g.setRGB(x, y, ch, m.theme.Gradient(rampStops...).At(pos))
}

var eqLabels = [10]string{"70", "180", "320", "600", "1k", "3k", "6k", "12k", "14k", "16k"}

// eqColumns is the band column geometry, shared by drawing and mouse hits.
func eqColumns(r rect) (x0, cw int) {
	in := r.inner()
	return in.x + 5, max(3, min(8, (in.w-6)/10))
}

// eqBandAt is the band under column x in the EQ panel, or -1.
func eqBandAt(r rect, x int) int {
	x0, cw := eqColumns(r)
	if b := (x - x0) / cw; x >= x0 && b < 10 {
		return b
	}
	return -1
}

func (m model) drawEQ(g *grid, r rect) {
	preset := strings.ToLower(m.snap.EQPreset)
	if preset == "" {
		preset = "flat"
	}
	sel := 0.0
	if m.eqBand < len(m.snap.EQBands) {
		sel = m.snap.EQBands[m.eqBand]
	}
	c, tc := cMagenta, cTitle
	if m.eqFocused() {
		c, tc = cFocus, cSel
	}
	g.box(r.x, r.y, r.w, r.h, fmt.Sprintf("eq · %s · %s Hz %+.0f dB · ←→ band ↑↓ gain p preset 0 off e close", preset, eqLabels[m.eqBand], sel), c, tc)
	in := r.inner()
	ih := in.h - 1 // last inner row holds the labels
	if ih < 3 || in.w < 30 {
		return
	}
	mid, half := in.y+ih/2, ih/2
	x0, cw := eqColumns(r)
	g.put(in.x+1, in.y, "+12", cDim)
	g.put(in.x+2, mid, " 0", cDim)
	g.put(in.x+1, in.y+ih-1, "-12", cDim)
	g.put(x0, mid, strings.Repeat("┄", cw*10-1), cDim)
	for i, lab := range eqLabels {
		gain := 0.0
		if i < len(m.snap.EQBands) {
			gain = m.snap.EQBands[i]
		}
		cx := x0 + i*cw
		// Each band has its own colour along the ramp, low to high.
		bar := func(y int, ch rune) {
			for j := range 2 {
				m.setRamp(g, cx+j, y, ch, float64(i)/9)
			}
		}
		// Bars in eighths of a row, so ±1 dB shows even on a short panel.
		eighths := int(math.Round(math.Abs(gain) / 12 * float64(half*8)))
		n := eighths / 8
		for k := 1; k <= n; k++ {
			if gain > 0 {
				bar(mid-k, '█')
			} else {
				bar(mid+k, '█')
			}
		}
		if part := eighths % 8; part > 0 {
			if gain > 0 {
				bar(mid-n-1, []rune(" ▁▂▃▄▅▆▇")[part])
			} else if part >= 4 {
				bar(mid+n+1, '▀')
			} else {
				bar(mid+n+1, '▔')
			}
		}
		if eighths == 0 { // a flat band gets a marker on the zero line
			knob := cWhite
			if i == m.eqBand {
				knob = cKey
			}
			g.put(cx, mid, "▬▬", knob)
		} else if i == m.eqBand {
			for yy := in.y; yy < in.y+ih; yy++ { // the selected band's bar turns amber
				if g.ch[yy*g.w+cx] != ' ' && g.ch[yy*g.w+cx] != '┄' {
					g.paint(cx, yy, 2, cKey)
				}
			}
		}
		lc := cDim
		if i == m.eqBand {
			lc = cKey
		}
		g.put(cx, in.y+ih, lab, lc)
	}
}

func (m model) drawSources(g *grid, r rect) {
	focused := m.focus == focusSources
	m.drawFrame(g, r, "", cYellow, focusSources)
	for i, t := range tabRects(r, m.tab) {
		c := cDim
		switch {
		case i == m.tab && focused:
			c = cSel
		case i == m.tab:
			c = cTitle
		}
		label := " " + tabNames[i] + " "
		if tabsCollapsed(r) {
			label = " ‹ " + tabNames[i] + " › "
		}
		if !t.empty() && t.x+t.w < r.x+r.w-1 {
			g.put(t.x, t.y, label, c)
		}
	}
	ctx := ""
	switch {
	case m.tab != tabSources:
	case m.inResults:
		ctx = "search: " + m.query
	case m.inCountry:
		ctx = m.providerName + " › " + countryLabel(m.country)
	case m.providerName != "":
		ctx = m.providerName
	}
	if m.loading {
		ctx += " …"
	}
	if ctx != "" {
		end := 0
		for _, t := range tabRects(r, m.tab) {
			end = max(end, t.x+t.w+1)
		}
		if room := r.x + r.w - 2 - end; room > 4 {
			g.put(end, r.y, fit(" "+ctx+" ", room), cWhite)
		}
	}
	in := r.inner()
	if m.tab == tabSources && m.searching {
		g.put(in.x+1, in.y, fit("/ "+m.query+"█", in.w-2), cCyan)
	} else if m.tab == tabSources && m.inResults {
		g.put(in.x+1, in.y, fit("/ "+m.query+"   esc to go back", in.w-2), cDim)
	}
	rows := m.listRows(r)
	list := m.lists[m.tab]
	if rows.h <= 0 {
		return
	}
	if len(list.rows) == 0 {
		empty := map[int]string{tabSources: "no sources yet", tabQueue: "queue is empty", tabHistory: "nothing played yet"}[m.tab]
		g.put(rows.x+1, rows.y, fit(empty, rows.w-2), cDim)
		return
	}
	start := listStart(len(list.rows), list.sel, rows.h)
	for i := 0; i < rows.h && start+i < len(list.rows); i++ {
		it := list.rows[start+i]
		y := rows.y + i
		mark, c := "  ", cNone
		spin := m.pending.matches(m.tab, it)
		switch {
		case it.kind == rowHeader:
			line := []rune("── " + it.label + " " + strings.Repeat("─", max(0, rows.w)))
			g.put(rows.x+1, y, string(line[:max(0, min(len(line), rows.w-2))]), cDim)
			continue
		case it.kind == rowSetup:
			c = cDim
		case spin: // still loading: a throbber where the ► will go
			mark, c = throbberFrame(time.Now())+" ", it.color
		case it.current:
			mark, c = "» ", cGreen
		default:
			c = it.color
		}
		right := it.right
		if len([]rune(right)) > rows.w/3 || rows.w < 40 { // narrow: the label keeps the room
			right = ""
		}
		left := " " + mark + it.label
		width := rows.w - len([]rune(right)) - 1
		line := fit(left, width)
		line += strings.Repeat(" ", max(0, width-len([]rune(line)))) + right + " "
		selected := start+i == list.sel && focused
		if selected && c == cNone {
			c = cWhite
		}
		g.put(rows.x, y, line, c)
		if spin {
			g.paint(rows.x+1, y, 1, cYellow)
		}
		if right != "" {
			g.paint(rows.x+width, y, len([]rune(right)), cDim)
		}
		if selected {
			g.shadeRow(rows.x, y, rows.w)
		}
	}
	if n := len(list.rows); n > rows.h {
		pos := fmt.Sprintf(" %d/%d ", list.sel+1, n)
		g.put(r.x+r.w-len(pos)-2, r.y+r.h-1, pos, cDim)
	}
}

func (m model) drawStatus(g *grid, y int) {
	if y < 0 {
		return
	}
	right := "● cliamp"
	rc := cGreen
	if !m.online {
		right, rc = "○ cliamp offline", cRed
	}
	room := g.w - len([]rune(right)) - 3
	if m.note != "" && time.Since(m.noteAt) < 5*time.Second {
		g.put(1, y, fit(m.note, room), cYellow)
	} else {
		x := 1
		for _, k := range m.hints() {
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

// hints are the status-line keys for what currently has focus.
func (m model) hints() [][2]string {
	switch {
	case m.searching:
		return [][2]string{{"⏎", "search"}, {"esc", "cancel"}}
	case m.eqFocused():
		return [][2]string{{"←→", "band"}, {"↑↓", "gain"}, {"p", "preset"}, {"0", "off"}, {"e", "close"}, {"tab", "focus"}}
	case m.focus == focusSources:
		keys := [][2]string{{"↑↓", "move"}, {"→", "open"}, {"←", "back"}, {"⏎", "play"}, {"a", "queue"}, {"A", "play next"}, {"[ ]", "tabs"}, {"/", "search"}}
		if r, ok := m.selected(); ok && r.kind == rowSetup {
			keys = [][2]string{{"s", "connect"}, {"↑↓", "move"}, {"[ ]", "tabs"}}
		}
		return append(keys, [2]string{"tab", "focus"})
	case m.focus == focusVis:
		return [][2]string{{"←→", "visual"}, {"⏎", "take over"}, {"{ }", "sync"}, {"␣", "play"}, {"e", "eq"}, {"/", "search"}, {"tab", "focus"}, {"q", "quit"}}
	}
	if m.live() {
		return [][2]string{{"␣", "play"}, {"↑↓", "vol"}, {"e", "eq"}, {"v", "visual"}, {"tab", "focus"}, {"q", "quit"}}
	}
	return [][2]string{{"␣", "play"}, {"↑↓", "vol"}, {"←→", "seek"}, {"e", "eq"}, {"n/p", "skip"}, {"z", "shuffle"}, {"v", "visual"}, {"tab", "focus"}, {"q", "quit"}}
}

func (m model) trackText() (title, source string) {
	t := m.snap.Track
	if t == nil {
		return "nothing playing — pick a source", ""
	}
	title = trackTitle(t.Artist, t.Title)
	source = t.Album
	if t.Station != "" {
		source = t.Station
	}
	return title, source
}

// trackTitle joins artist and title for display. Some stations send stream
// metadata as "Artist - Artist - Title"; cliamp splits at the first " - ", so
// the title arrives still carrying the artist. Drop that repeat.
func trackTitle(artist, title string) string {
	artist = strings.TrimSpace(artist)
	if artist == "" {
		return title
	}
	for _, sep := range []string{" - ", " – ", " — "} {
		if len(title) > len(artist)+len(sep) && strings.EqualFold(title[:len(artist)], artist) &&
			strings.HasPrefix(title[len(artist):], sep) {
			title = title[len(artist)+len(sep):]
			break
		}
	}
	if strings.EqualFold(strings.TrimSpace(title), artist) {
		return artist
	}
	return artist + " — " + title
}

func (m model) stateGlyph() (string, cls) {
	switch m.snap.State {
	case "playing":
		return glyphs.play, cGreen
	case "paused":
		return glyphs.pause, cYellow
	}
	return glyphs.stop, cDim
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
	gw := len([]rune(glyph)) + 1 // the plain pause is two cells
	g.put(x+gw, y, m.marquee(title, max(1, w-gw)), cWhite)
}

// drawTimeLine is a thin progress bar for a track. A stream has no length, so
// it gets LIVE and how long it has been on air instead.
func (m model) drawTimeLine(g *grid, x, y, w int) {
	pos, dur := m.position(), m.snap.Duration
	if m.live() {
		dot := "●"
		if m.snap.State == "playing" && time.Now().UnixMilli()/600%2 == 1 { // blinks while on air
			dot = " "
		}
		g.put(x, y, dot+" LIVE", cRed)
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
