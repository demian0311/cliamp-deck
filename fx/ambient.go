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
// Unknown Pleasures sleeve. Each ridge is a moment of the spectrum, so the
// song's shape stays readable a couple of seconds back.
type Ridges struct {
	lines [][]float64
	since float64
}

const (
	ridgeCount = 16
	ridgeEvery = 0.11 // seconds between new ridges
)

func (*Ridges) Name() string { return "ridges" }

func (e *Ridges) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	if len(e.lines) > 0 && len(e.lines[0]) != f.W {
		e.lines = nil
	}
	if e.since += dt; e.since >= ridgeEvery || len(e.lines) == 0 {
		e.since = 0
		prof := make([]float64, f.W)
		seed := rand.Float64() * 100 // each ridge gets its own jagged skyline
		for x := range prof {
			b := a.Bass
			if n := len(a.Bands); n > 1 { // interpolate so the profile has no band steps
				pos := float64(x) / float64(max(1, f.W-1)) * float64(n-1)
				i := min(int(pos), n-2)
				b = clamp01(a.Bands[i] + (a.Bands[i+1]-a.Bands[i])*(pos-float64(i)))
			}
			d := (float64(x) - float64(f.W)/2) / (float64(f.W) * 0.2)
			centre := math.Exp(-d * d)
			u := float64(x) * 96 / float64(max(1, f.W)) // noise in deck-preview units
			jag := 0.5 + 0.3*math.Sin(u*0.9+seed) + 0.2*math.Sin(u*2.3+seed*1.7)
			prof[x] = centre * (0.3 + b) * (0.4 + jag)
		}
		e.lines = append([][]float64{prof}, e.lines...)
		if len(e.lines) > ridgeCount {
			e.lines = e.lines[:ridgeCount]
		}
	}
	bg, fg := th.Background, th.Color("foreground")
	for i := range f.Px {
		f.Px[i] = bg
	}
	top := float64(f.H) * 0.22
	spacing := (float64(f.H) - top - 2) / ridgeCount
	amp := float64(f.H) * 0.19
	for k := len(e.lines) - 1; k >= 0; k-- {
		base := top + float64(ridgeCount-1-k)*spacing
		col := lerp(fg, bg, float64(k)/(ridgeCount+2))
		if k == 0 {
			col = lerp(fg, th.Color("cyan"), 0.35)
		}
		prev := 0
		for x := range f.W {
			y := int(math.Round(base - e.lines[k][x]*amp))
			for yy := max(0, y+1); yy < min(f.H, int(base)+3); yy++ { // hide the ridges behind
				f.Px[yy*f.W+x] = bg
			}
			lo, hi := y, y // join to the previous column so the ridge is one line
			if x > 0 {
				lo, hi = min(y, (y+prev)/2), max(y, (y+prev)/2)
			}
			for yy := max(0, lo); yy <= min(f.H-1, hi); yy++ {
				f.Px[yy*f.W+x] = col
			}
			prev = y
		}
	}
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

// Scope: a phosphor oscilloscope tracing Lissajous figures with afterglow.
// cliamp sends bands, not the waveform, so the figure is built from them:
// bass sets its size, mids its knot ratio, treble makes it fizz.
type Scope struct{ acc []float64 }

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
	deep, green := th.Color("darker_background"), th.Color("green")
	for i, v := range e.acc {
		if v < 1 {
			f.Px[i] = lerp(deep, green, v)
		} else {
			f.Px[i] = lerp(green, th.Bright, (v-1)/0.6)
		}
	}
	gx, gy := max(2, f.W/8), max(2, f.H/4) // graticule
	for y := range f.H {
		for x := range f.W {
			if (x%gx == 0 && y%2 == 0) || (y%gy == 0 && x%2 == 0) {
				blend(f, x, y, green, 0.07)
			}
		}
	}
}

// Rain: falling columns of glyphs, green with cyan and white leading edges.
// Speed follows the energy; each beat starts new streams and flashes the
// lead glyphs. It draws text cells (GlyphEffect).
type Rain struct {
	drops []drop
	glyph []rune
	prev  float64
	cells Cells // Render's pixel fallback draws through this
}

type drop struct {
	y, v float64
	n    int
}

var rainGlyphs = []rune("01ｱｲｳｴｵｶｷｸｹｺｻｼｽｾｿﾀﾁﾂﾃﾄﾅﾆﾇﾈﾉ0123456789ABCDEF<>:;=+*")

func (*Rain) Name() string { return "rain" }

func (e *Rain) RenderCells(c *Cells, a Audio, t, dt float64, th *Theme) {
	if len(e.drops) != c.W || len(e.glyph) != c.W*c.H {
		e.drops = make([]drop, c.W)
		for i := range e.drops {
			e.drops[i] = drop{-rand.Float64() * float64(c.H) * 2, 4 + rand.Float64()*6, 6 + rand.IntN(12)}
		}
		e.glyph = make([]rune, c.W*c.H)
		for i := range e.glyph {
			e.glyph[i] = rainGlyphs[rand.IntN(len(rainGlyphs))]
		}
	}
	if c.W == 0 || c.H == 0 {
		return
	}
	if onset(a, &e.prev) {
		for range 3 + int(a.Bass*6) {
			if d := &e.drops[rand.IntN(c.W)]; d.y > float64(c.H) {
				d.y = -1
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
		d := &e.drops[x]
		d.y += d.v * (0.35 + (a.Bass+a.Mid)*0.8) * dt
		if d.y-float64(d.n) > float64(c.H) && rand.Float64() < 0.02 {
			d.y, d.n = -rand.Float64()*6, 6+rand.IntN(12)
		}
		for k := range d.n {
			y := int(math.Floor(d.y)) - k
			if y < 0 || y >= c.H {
				continue
			}
			tail := green
			if k < 3 {
				tail = cyan
			}
			col := lerp(deep, tail, (1-float64(k)/float64(d.n))*0.95)
			if k == 0 {
				col = lerp(fg, th.Bright, a.Beat)
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
	deep := th.Color("darker_background")
	for y := range f.H {
		for x := range f.W {
			f.Px[y*f.W+x] = deep
			if cy := y / 2; cy < e.cells.H && e.cells.Ch[cy*f.W+x] != 0 {
				f.Px[y*f.W+x] = e.cells.Fg[cy*f.W+x]
			}
		}
	}
}
