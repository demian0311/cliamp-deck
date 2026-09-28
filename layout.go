package main

import (
	"math"
	"time"
)

// Layout per docs/design/layout.md: a Winamp stack (player, [EQ], visualizer,
// sources), side-by-side when the terminal is much wider than tall, a
// three-line mini form for tiny terminals, and a full-terminal takeover.

type rect struct{ x, y, w, h int }

func (r rect) inner() rect { return rect{r.x + 1, r.y + 1, max(0, r.w-2), max(0, r.h-2)} }

func (r rect) has(x, y int) bool { return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h }

func (r rect) empty() bool { return r.w <= 0 || r.h <= 0 }

type focusArea int

const (
	focusPlayer focusArea = iota
	focusVis
	focusSources
	focusCount
)

type shape int

const (
	shapeStack shape = iota
	shapeSide
	shapeMini
	shapeTakeover
)

type layout struct {
	shape                    shape
	player, eq, vis, sources rect
	status                   int  // row of the status line; -1 when there is none
	visBoxed                 bool // the visualizer has a frame (and a ‹ › control)
	miniTitle                int  // mini shape: row of the title line (time is the next row); -1 otherwise
}

type layoutState struct {
	take, eqOpen bool
	focus        focusArea
	compact      bool // spectrum is showing: the visualizer keeps to the player's height
}

const (
	sideCols   = 160 // side-by-side needs at least this many columns…
	sideAspect = 2.5 // …and cols >= sideAspect * rows (200×58, 280×58 yes; 100×58, 100×28 no)
	sideMaxL   = 100 // left column width cap in side-by-side
	miniCols   = 60  // below this many columns, or miniRows rows, the mini shape applies
	miniRows   = 16
)

func computeLayout(w, h int, s layoutState) layout {
	l := layout{status: h - 1, miniTitle: -1, visBoxed: true}
	if w <= 0 || h <= 0 {
		return l
	}
	body := h - 1
	switch {
	case s.take:
		l.shape, l.status, l.visBoxed = shapeTakeover, -1, false
		l.vis = rect{0, 0, w, h}
	case w < miniCols || h < miniRows:
		l.shape = shapeMini
		if s.focus == focusSources {
			l.sources = rect{0, 0, w, body}
		} else {
			l.miniTitle, l.visBoxed = 0, false
			l.vis = rect{0, 2, w, max(0, body-2)}
		}
	case w >= sideCols && float64(w) >= sideAspect*float64(h):
		l.shape = shapeSide
		lw := min(sideMaxL, w*40/100)
		ph, eh := panelHeights(h, s.eqOpen)
		if s.compact { // player and spectrum share the top row; EQ and sources run full width below
			l.player, l.vis = rect{0, 0, lw, ph}, rect{lw, 0, w - lw, ph}
			if eh > 0 {
				l.eq = rect{0, ph, w, eh}
			}
			l.sources = rect{0, ph + eh, w, max(0, body-ph-eh)}
			break
		}
		l.player = rect{0, 0, lw, ph}
		if eh > 0 {
			l.eq = rect{0, ph, lw, eh}
		}
		l.sources = rect{0, ph + eh, lw, max(0, body-ph-eh)}
		l.vis = rect{lw, 0, w - lw, body}
	default:
		l.shape = shapeStack
		ph, eh := panelHeights(h, s.eqOpen)
		minSrc := 8
		if h < 30 {
			minSrc = 5
		}
		// Split as if the EQ were closed, then take the EQ's rows from the
		// visualizer, so opening the EQ never shortens the list.
		avail := max(0, body-ph)
		if s.compact { // spectrum stays the player's height; the EQ comes out of the list
			vh := min(ph, max(0, avail-eh))
			l.player, l.vis = rect{0, 0, w, ph}, rect{0, ph + eh, w, vh}
			if eh > 0 {
				l.eq = rect{0, ph, w, eh}
			}
			l.sources = rect{0, ph + eh + vh, w, max(0, avail-eh-vh)}
			break
		}
		vh := int(float64(avail)*0.6 + 0.5)
		if s.focus == focusSources {
			vh = max(3, int(float64(avail)*0.25+0.5)) // the list grows while it has focus
		}
		sh := avail - vh
		if sh < minSrc {
			sh = min(max(0, avail-3), minSrc)
			vh = avail - sh
		}
		if vh -= eh; vh < 3 { // no room: the list gives up what the visualizer can't
			sh = max(0, sh+vh-3)
			vh = min(3, max(0, avail-eh))
		}
		l.player = rect{0, 0, w, ph}
		if eh > 0 {
			l.eq = rect{0, ph, w, eh}
		}
		l.vis = rect{0, ph + eh, w, vh}
		l.sources = rect{0, ph + eh + vh, w, sh}
	}
	return l
}

func panelHeights(h int, eqOpen bool) (player, eq int) {
	player, eq = 6, 0
	if eqOpen {
		eq = 10
	}
	if h < 30 {
		player = 5
		if eqOpen {
			eq = 7
		}
	}
	return player, eq
}

// visInner is the pixel area of the visualizer: inside the frame when it has one.
func (l layout) visInner() rect {
	if l.vis.empty() {
		return rect{}
	}
	if l.visBoxed {
		return l.vis.inner()
	}
	return l.vis
}

// layoutAnim eases the panels from the layout on screen when the layout state
// changed (focus, EQ, takeover) to the new one, rather than snapping.
type layoutAnim struct {
	from layout
	to   layoutState
	at   time.Time
	w, h int // terminal size the animation started at; a resize snaps
}

// layoutDuration is how long panels take to reach their new size.
const layoutDuration = 180 * time.Millisecond

// layout is the layout to draw and hit-test now.
func (m model) layout() layout { return m.layoutAt(time.Now()) }

func (m model) layoutAt(now time.Time) layout {
	to := computeLayout(m.w, m.h, m.ls())
	a := m.anim
	t := float64(now.Sub(a.at)) / float64(layoutDuration)
	if a.at.IsZero() || t >= 1 || a.w != m.w || a.h != m.h || a.from.shape != to.shape || a.to != m.ls() {
		return to
	}
	t = 1 - (1-t)*(1-t)*(1-t) // ease out
	out := to
	out.player = lerpRect(a.from.player, to.player, t)
	out.eq = lerpRect(a.from.eq, to.eq, t)
	out.vis = lerpRect(a.from.vis, to.vis, t)
	out.sources = lerpRect(a.from.sources, to.sources, t)
	return out
}

// lerpRect moves a panel's edges. A panel appearing grows from nothing at its
// new top; one disappearing shrinks into its old top. Edges are rounded, not
// sizes, so panels that share an edge never gap or overlap.
func lerpRect(a, b rect, t float64) rect {
	switch {
	case a.empty() && b.empty():
		return b
	case a.empty():
		a = rect{b.x, b.y, b.w, 0}
	case b.empty():
		b = rect{a.x, a.y, a.w, 0}
	}
	at := func(p, q int) int { return int(math.Round(float64(p) + float64(q-p)*t)) }
	x, y := at(a.x, b.x), at(a.y, b.y)
	r := rect{x, y, at(a.x+a.w, b.x+b.w) - x, at(a.y+a.h, b.y+b.h) - y}
	if r.h <= 0 || r.w <= 0 {
		return rect{}
	}
	return r
}
