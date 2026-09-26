package main

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
}

const (
	sideAspect = 4.5 // side-by-side when cols >= sideAspect * rows (ultrawide 280×58 yes; 200×58, 100×28 no)
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
	case float64(w) >= sideAspect*float64(h):
		l.shape = shapeSide
		lw := min(sideMaxL, w*40/100)
		ph, eh := panelHeights(h, s.eqOpen)
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
