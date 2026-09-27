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
// back continuously. Ridges take their colour from the theme's hues by depth,
// drifting over time, and fade as they recede.
type Ridges struct {
	lines [][]float64 // profiles per dot column, newest (live) first
	seeds []float64   // each ridge's jagged-skyline seed
	since float64
	dots  []int8 // ridge index owning each dot, -1 for none
	cells Cells  // Render's pixel fallback draws through this
}

const (
	ridgeCount = 16
	ridgeEvery = 0.11 // seconds between new ridges
	ridgeLap   = 24.0 // seconds for the colours to drift round the hues once
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
	e.since += dt
	if e.since >= ridgeEvery || len(e.lines) == 0 { // freeze the live ridge, start a new one
		e.since = math.Mod(e.since, ridgeEvery)
		live := make([]float64, W)
		if len(e.lines) > 0 {
			copy(live, e.lines[0])
		}
		e.lines = append([][]float64{live}, e.lines...)
		e.seeds = append([]float64{rand.Float64() * 100}, e.seeds...)
		if len(e.lines) > ridgeCount+1 { // one spare to glide out at the back
			e.lines, e.seeds = e.lines[:ridgeCount+1], e.seeds[:ridgeCount+1]
		}
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
	bottom, top := float64(H)-2, float64(H)*0.22
	spacing := (bottom - top) / ridgeCount
	amp := float64(H) * 0.19
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
	hues := th.Gradient(append(metaballHues, metaballHues[0])...)
	colours := make([]RGB, len(e.lines))
	for k := range colours {
		depth := (float64(k) + phase) / ridgeCount
		col := hues.At(frac(t/ridgeLap + depth*0.7))
		col = lerp(col, th.Background, clamp01(depth*0.8))
		if k == 0 {
			col = lerp(col, th.Bright, 0.3)
		}
		colours[k] = col
	}
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
			if ch == 0 {
				c.Ch[i] = 0
				continue
			}
			c.Ch[i], c.Fg[i] = 0x2800+ch, colours[front]
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

// Aurora: slow curtains of light over a still sky. Brightness follows the
// music through heavy smoothing and never jumps, so it can sit in the corner
// of the eye all day.
type Aurora struct{ level float64 }

func (*Aurora) Name() string { return "aurora" }

func (e *Aurora) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	e.level += ((a.Bass+a.Mid)/2 - e.level) * math.Min(1, dt*0.6)
	deep, bg, fg := th.Color("darker_background"), th.Background, th.Color("foreground")
	W, H := float64(f.W), float64(f.H)
	for y := range f.H {
		sky := lerp(deep, bg, float64(y)/H)
		for x := range f.W {
			f.Px[y*f.W+x] = sky
		}
	}
	for i := range 40 { // fixed stars: a hash of i, so they hold still
		x, y := int(frac(float64(i)*0.618034)*W), int(frac(float64(i)*0.414214+0.3)*H*0.5)
		blend(f, x, y, fg, 0.25+0.15*math.Sin(t*0.8+float64(i)))
	}
	layers := []struct {
		c      RGB
		sp     float64
		offset float64
		gain   float64
	}{
		{th.Color("green"), 0.19, 0, 0.8},
		{th.Color("cyan"), 0.13, 1.7, 0.55},
		{th.Color("magenta"), 0.09, 3.1, 0.55},
	}
	for k, l := range layers {
		for x := range f.W {
			fx := float64(x) * 96 / W // shape in deck-preview units, whatever the width
			top := H*(0.22+0.1*float64(k)) + math.Sin(fx*0.055+t*l.sp+l.offset)*H*0.1 + math.Sin(fx*0.017-t*l.sp*0.6)*H*0.07
			rays := 0.55 + 0.45*math.Sin(fx*0.42+t*(0.5+float64(k)*0.17)+math.Sin(fx*0.09+t*0.3)*2)
			for y := max(0, int(top)-1); y < f.H; y++ {
				d := float64(y) - top
				v := math.Exp(-d / (8 + 5*rays) * 56 / H)
				if d < 0 {
					v = math.Exp(d * 2.5)
				}
				blend(f, x, y, l.c, clamp01(v*rays*(0.35+e.level*0.55)*l.gain))
			}
		}
	}
}

// Ripples: dark water. Each beat drops a ring somewhere that spreads,
// overlaps and fades; treble adds small pale drops. Calm between beats.
type Ripples struct {
	rings []ripple
	prev  float64
}

type ripple struct {
	x, y, t0, s float64
	c           RGB
}

func (*Ripples) Name() string { return "ripples" }

func (e *Ripples) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	W, H := float64(f.W), float64(f.H)
	hues := []RGB{th.Color("cyan"), th.Color("blue"), th.Color("magenta"), th.Color("green")}
	if onset(a, &e.prev) {
		e.rings = append(e.rings, ripple{W*0.1 + rand.Float64()*W*0.8, H*0.15 + rand.Float64()*H*0.7, t, 0.4 + a.Bass, hues[rand.IntN(len(hues))]})
	}
	if rand.Float64() < a.Treble*1.5*dt {
		e.rings = append(e.rings, ripple{rand.Float64() * W, rand.Float64() * H, t, 0.25, th.Color("foreground")})
	}
	for len(e.rings) > 0 && t-e.rings[0].t0 > 6 {
		e.rings = e.rings[1:]
	}
	bg := th.Background
	speed := W * 0.135 // pixels a second
	for y := range f.H {
		for x := range f.W {
			r, g, b := float64(bg.R), float64(bg.G), float64(bg.B)
			for _, rg := range e.rings {
				age := t - rg.t0
				d := math.Hypot(float64(x)-rg.x, float64(y)-rg.y) - age*speed
				if math.Abs(d) > 7 {
					continue
				}
				k := math.Max(-0.25, math.Min(0.8, 0.9*rg.s*math.Exp(-age*0.7)*math.Cos(d*0.9)*math.Exp(-d*d/14)))
				r += (float64(rg.c.R) - float64(bg.R)) * k
				g += (float64(rg.c.G) - float64(bg.G)) * k
				b += (float64(rg.c.B) - float64(bg.B)) * k
			}
			f.Px[y*f.W+x] = RGB{u8(r), u8(g), u8(b)}
		}
	}
}

func u8(v float64) uint8 { return uint8(math.Max(0, math.Min(255, v))) }

// Warp: a starfield in theme colours, flown through. Bass sets the speed and
// each beat lurches the field forward into streaks.
type Warp struct{ stars []star }

type star struct {
	x, y, z float64
	c       RGB
}

func (*Warp) Name() string { return "warp" }

func (e *Warp) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	hues := []RGB{th.Color("cyan"), th.Color("blue"), th.Color("magenta"), th.Color("red"), th.Color("orange"),
		th.Color("yellow"), th.Color("green"), th.Bright, th.Bright, th.Bright}
	spawn := func(z float64) star {
		return star{rand.Float64()*2 - 1, rand.Float64()*2 - 1, z, hues[rand.IntN(len(hues))]}
	}
	if len(e.stars) == 0 {
		for range 170 {
			e.stars = append(e.stars, spawn(rand.Float64()))
		}
	}
	deep := th.Color("darker_background")
	for i := range f.Px { // trails: last frame fades toward the void
		f.Px[i] = lerp(f.Px[i], deep, 0.45)
	}
	W, H := float64(f.W), float64(f.H)
	proj := func(s star) (float64, float64) { return W/2 + s.x/s.z*W*0.35, H/2 + s.y/s.z*H*0.35 }
	speed := 0.12 + a.Bass*0.9 + a.Beat*1.4
	for i := range e.stars {
		s := &e.stars[i]
		px, py := proj(*s)
		s.z -= speed * dt
		if s.z <= 0.03 {
			*s = spawn(1)
			continue
		}
		nx, ny := proj(*s)
		if nx < 0 || nx >= W || ny < 0 || ny >= H {
			*s = spawn(1)
			continue
		}
		n := max(1, int(math.Ceil(math.Hypot(nx-px, ny-py))))
		for k := 0; k <= n; k++ {
			u := float64(k) / float64(n)
			blend(f, int(math.Round(px+(nx-px)*u)), int(math.Round(py+(ny-py)*u)), s.c, clamp01(1.15-s.z)*(0.4+0.6*u))
		}
	}
}

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
	deep, ink := th.Color("darker_background"), hues.At(frac(e.hue))
	for i, v := range e.acc {
		if v < 1 {
			f.Px[i] = lerp(deep, ink, v)
		} else {
			f.Px[i] = lerp(ink, th.Bright, (v-1)/0.6)
		}
	}
}

// Rain: falling columns of glyphs, green with cyan and white leading edges.
// Each column follows one of cliamp's bands, laid out low to high across the
// screen: a loud band rains fast, long and bright, a quiet one thins and goes
// dry. A beat surges every stream and restarts the loudest columns, flashing
// their lead glyphs. It draws text cells (GlyphEffect).
type Rain struct {
	drops []drop
	glyph []rune
	prev  float64
	surge float64 // extra speed from the last beat, decaying
	cells Cells   // Render's pixel fallback draws through this
}

type drop struct {
	y, v, n float64 // head row, base speed, trail length
}

// rainGlyphs stay within what common coding fonts carry: half-width katakana
// rendered as missing-glyph boxes in JetBrains Mono.
var rainGlyphs = []rune("0123456789ABCDEFXZ<>:;=+*#%$&@?!/\\|{}[]~^")

func (*Rain) Name() string { return "rain" }

func (e *Rain) RenderCells(c *Cells, a Audio, t, dt float64, th *Theme) {
	if c.W == 0 || c.H == 0 {
		return
	}
	if len(e.drops) != c.W || len(e.glyph) != c.W*c.H {
		e.drops = make([]drop, c.W)
		for i := range e.drops {
			e.drops[i] = drop{-rand.Float64() * float64(c.H), 3 + rand.Float64()*3, 6}
		}
		e.glyph = make([]rune, c.W*c.H)
		for i := range e.glyph {
			e.glyph[i] = rainGlyphs[rand.IntN(len(rainGlyphs))]
		}
	}
	level := func(x int) float64 { // this column's band
		if n := len(a.Bands); n > 0 {
			return clamp01(a.Bands[min(n-1, x*n/c.W)])
		}
		return a.Mid
	}
	e.surge *= math.Exp(-dt * 4)
	if onset(a, &e.prev) {
		e.surge += 10 * a.Beat
		for range 4 + int(a.Bass*10) { // restart loud columns that have run dry
			x := rand.IntN(c.W)
			if d := &e.drops[x]; level(x) > 0.4 && d.y-d.n > float64(c.H)*0.5 {
				d.y = -rand.Float64() * 3
			}
		}
	}
	for range 6 {
		e.glyph[rand.IntN(len(e.glyph))] = rainGlyphs[rand.IntN(len(rainGlyphs))]
	}
	for i := range c.Ch {
		c.Ch[i] = 0
	}
	deep, green, cyan, fg := th.Color("darker_background"), th.Color("green"), th.Color("cyan"), th.Color("foreground")
	for x := range e.drops {
		d, lvl := &e.drops[x], level(x)
		d.y += (d.v*(0.15+lvl*1.6) + e.surge) * dt
		d.n += (3 + lvl*16 - d.n) * math.Min(1, dt*3)
		if d.y-d.n > float64(c.H) && rand.Float64() < lvl*lvl*3*dt { // quiet bands stay dry
			d.y, d.v = -rand.Float64()*4, 3+rand.Float64()*3
		}
		n := int(d.n)
		for k := range n {
			y := int(math.Floor(d.y)) - k
			if y < 0 || y >= c.H {
				continue
			}
			tail := green
			if k < 3 {
				tail = cyan
			}
			col := lerp(deep, tail, (1-float64(k)/float64(n))*(0.35+0.6*lvl))
			if k == 0 {
				col = lerp(lerp(deep, fg, 0.5+0.5*lvl), th.Bright, a.Beat)
			}
			c.Ch[y*c.W+x], c.Fg[y*c.W+x] = e.glyph[y*c.W+x], col
		}
	}
}

// Render draws the rain as pixels, two per cell, for anything that cannot
// show glyphs.
func (e *Rain) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	e.cells.Resize(f.W, f.H/2)
	e.RenderCells(&e.cells, a, t, dt, th)
	cellsToPixels(f, &e.cells, th.Color("darker_background"))
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
