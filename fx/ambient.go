package fx

import (
	"math"
	"math/rand/v2"
)

// Cells is a W×H grid of coloured glyphs, one per terminal cell, for effects
// that draw characters rather than pixels. A zero Ch is an empty cell.
type Cells struct {
	W, H int
	Ch   []rune
	Fg   []RGB
}

func (c *Cells) Resize(w, h int) {
	if c.W == w && c.H == h {
		return
	}
	c.W, c.H = w, h
	c.Ch, c.Fg = make([]rune, max(0, w*h)), make([]RGB, max(0, w*h))
}

// GlyphEffect is an Effect that draws text cells. The deck calls RenderCells
// in place of Render for it; Render stays as a pixel fallback.
type GlyphEffect interface {
	Effect
	RenderCells(c *Cells, a Audio, t, dt float64, th *Theme)
}

// onset reports a beat that has just landed: Beat jumped since last frame.
func onset(a Audio, prev *float64) bool {
	hit := a.Beat > 0.6 && *prev < 0.45
	*prev = a.Beat
	return hit
}

func blend(f *Frame, x, y int, c RGB, k float64) {
	if x < 0 || x >= f.W || y < 0 || y >= f.H {
		return
	}
	f.Px[y*f.W+x] = lerp(f.Px[y*f.W+x], c, k)
}

// Ridges: stacked waveform lines scrolling back into the dark, like the
// Unknown Pleasures sleeve, drawn in braille dots (a GlyphEffect: 2×4 dots a
// cell). The front ridge is live, following the bands every frame; every
// ridgeEvery it freezes and a new one takes the front, while the stack glides
// back continuously. Colour is height, cool to hot, kept as a ridge recedes
// and dims; both sides fade into the background.
type Ridges struct {
	lines [][]float64 // profiles per dot column, newest (live) first
	seeds []float64   // each ridge's jagged-skyline seed
	since float64
	dots  []int8 // ridge index owning each dot, -1 for none
	cells Cells  // Render's pixel fallback draws through this
}

// Ridge pitch and height are fixed in dots rather than shares of the height,
// so a small pane shows fewer ridges instead of crushing all of them
// together. Both match the old 16-ridge layout on an 80-row visualizer
// (fullscreen foot on a 2560×1440 display), so that view is unchanged.
const (
	ridgeSpacing = 15.5 // dots between ridges
	ridgeAmp     = 61.0 // dots a full-height peak rises; smaller panes scale it down
	ridgeEvery   = 0.11 // seconds between new ridges
	ridgeEdge    = 0.2  // share of the width each side takes to fade in
)

func (*Ridges) Name() string { return "ridges" }

func (e *Ridges) RenderCells(c *Cells, a Audio, t, dt float64, th *Theme) {
	W, H := c.W*2, c.H*4 // dot resolution
	if W == 0 || H == 0 {
		return
	}
	if len(e.lines) > 0 && len(e.lines[0]) != W {
		e.lines, e.seeds = nil, nil
	}
	bottom, top := float64(H)-2, float64(H)*0.22
	count := max(1, int(math.Ceil((bottom-top)/ridgeSpacing)))
	e.since += dt
	if e.since >= ridgeEvery || len(e.lines) == 0 { // freeze the live ridge, start a new one
		e.since = math.Mod(e.since, ridgeEvery)
		live := make([]float64, W)
		if len(e.lines) > 0 {
			copy(live, e.lines[0])
		}
		e.lines = append([][]float64{live}, e.lines...)
		e.seeds = append([]float64{rand.Float64() * 100}, e.seeds...)
	}
	if len(e.lines) > count+1 { // one spare to glide out at the back
		e.lines, e.seeds = e.lines[:count+1], e.seeds[:count+1]
	}
	live, seed := e.lines[0], e.seeds[0]
	for x := range live {
		b := a.Bass
		if n := len(a.Bands); n > 1 { // interpolate so the profile has no band steps
			pos := float64(x) / float64(max(1, W-1)) * float64(n-1)
			i := min(int(pos), n-2)
			b = clamp01(a.Bands[i] + (a.Bands[i+1]-a.Bands[i])*(pos-float64(i)))
		}
		d := (float64(x) - float64(W)/2) / (float64(W) * 0.2)
		u := float64(x) * 192 / float64(W) // noise in fixed units, whatever the width
		jag := 0.5 + 0.3*math.Sin(u*0.45+seed) + 0.2*math.Sin(u*1.15+seed*1.7)
		target := math.Exp(-d*d) * (0.3 + b) * (0.4 + jag)
		live[x] += (target - live[x]) * math.Min(1, dt*14) // smooth, but quick enough to ride the beat
	}
	if len(e.dots) != W*H {
		e.dots = make([]int8, W*H)
	}
	for i := range e.dots {
		e.dots[i] = -1
	}
	phase := e.since / ridgeEvery
	spacing := ridgeSpacing
	amp := math.Min(ridgeAmp, float64(H)*0.19)
	for k := len(e.lines) - 1; k >= 0; k-- { // back to front, each hiding what is behind it
		base := bottom - (float64(k)+phase)*spacing
		prev := 0
		for x := range W {
			y := int(math.Round(base - e.lines[k][x]*amp))
			for yy := max(0, y+1); yy < min(H, int(base)+4); yy++ {
				e.dots[yy*W+x] = -1
			}
			lo, hi := y, y // join to the previous column so the ridge is one line
			if x > 0 {
				lo, hi = min(y, (y+prev)/2), max(y, (y+prev)/2)
			}
			for yy := max(0, lo); yy <= min(H-1, hi); yy++ {
				e.dots[yy*W+x] = int8(k)
			}
			prev = y
		}
	}
	// Colour is height: a ridge is cool where it lies flat and hot where it
	// peaks, and keeps those colours as it recedes, only dimmer. Both sides
	// dissolve into the background so the flat lines don't run to the edge.
	heat := th.Gradient("blue", "cyan", "green", "yellow", "orange", "red")
	bits := [4][2]rune{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}
	for cy := range c.H {
		for cx := range c.W {
			var ch rune
			front := int8(len(e.lines))
			for r := range 4 {
				for s := range 2 {
					if k := e.dots[(cy*4+r)*W+cx*2+s]; k >= 0 {
						ch |= bits[r][s]
						front = min(front, k)
					}
				}
			}
			i := cy*c.W + cx
			u := (float64(cx) + 0.5) / float64(c.W)
			edge := smoothstep(0, ridgeEdge, u) * smoothstep(0, ridgeEdge, 1-u)
			if ch == 0 || edge < 0.06 {
				c.Ch[i] = 0
				continue
			}
			depth := (float64(front) + phase) / float64(count)
			col := heat.At(clamp01(e.lines[front][cx*2] / 1.1))
			if front == 0 {
				col = th.Glow(col, 0.2)
			}
			col = lerp(th.Background, col, edge*(1-clamp01(depth*0.75)))
			c.Ch[i], c.Fg[i] = 0x2800+ch, col
		}
	}
}

// Render draws the ridges as pixels, lit where a cell has dots in that half,
// for anything that cannot show glyphs.
func (e *Ridges) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	e.cells.Resize(f.W, f.H/2)
	e.RenderCells(&e.cells, a, t, dt, th)
	cellsToPixels(f, &e.cells, th.Background)
}

func u8(v float64) uint8 { return uint8(math.Max(0, math.Min(255, v))) }

// Scope: a phosphor oscilloscope tracing Lissajous figures with afterglow,
// its colour drifting through the theme's hues.
// cliamp sends bands, not the waveform, so the figure is built from them:
// bass sets its size, mids its knot ratio, treble makes it fizz.
type Scope struct {
	acc []float64
	hue float64 // position round the hues, in laps
}

// scopeLap is how many seconds the trace takes to go round the hues once.
const scopeLap = 30.0

func (*Scope) Name() string { return "scope" }

func (e *Scope) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	if len(e.acc) != f.W*f.H {
		e.acc = make([]float64, f.W*f.H)
	}
	decay := math.Pow(0.84, dt*30)
	for i := range e.acc {
		e.acc[i] *= decay
	}
	W, H := float64(f.W), float64(f.H)
	R, cx, cy := H*0.42, W/2, H/2
	ra, rb := 2+math.Round(a.Mid*3), 3.0
	size := 0.25 + a.Bass*0.65
	jit := W / 96
	points := 700 * f.W / 96
	for s := range points {
		u := float64(s) / float64(points) * 2 * math.Pi
		x := cx + R*1.6*size*math.Sin(ra*u+t*0.7) + a.Treble*3*jit*math.Sin(17*u+t*9)
		y := cy + R*size*math.Sin(rb*u) + a.Treble*2*jit*math.Sin(13*u-t*7)
		ix, iy := int(math.Round(x)), int(math.Round(y))
		if ix >= 0 && ix < f.W && iy >= 0 && iy < f.H {
			e.acc[iy*f.W+ix] = math.Min(1.6, e.acc[iy*f.W+ix]+0.35)
		}
	}
	// The trace drifts round the theme's hues (the metaballs' palette), one lap
	// per scopeLap seconds, and a beat nudges it along.
	e.hue += dt/scopeLap + a.Beat*dt*0.3
	hues := th.Gradient(append(metaballHues, metaballHues[0])...)
	deep, ink := th.Stage, hues.At(frac(e.hue))
	for i, v := range e.acc {
		if v < 1 {
			f.Px[i] = lerp(deep, ink, v)
		} else {
			f.Px[i] = th.Glow(ink, (v-1)/0.6)
		}
	}
}

// cellsToPixels paints each lit cell's colour into both of its pixels.
func cellsToPixels(f *Frame, c *Cells, bg RGB) {
	for y := range f.H {
		for x := range f.W {
			f.Px[y*f.W+x] = bg
			if cy := y / 2; cy < c.H && c.Ch[cy*c.W+x] != 0 {
				f.Px[y*f.W+x] = c.Fg[cy*c.W+x]
			}
		}
	}
}
