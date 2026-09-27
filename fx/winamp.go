package fx

import (
	"math"
	"math/rand/v2"
)

// Effects after Winamp-era visualizers. Each makes a change you can see on
// the frame the beat lands, or draws a shape the spectrum sets, because a
// level smoothed into a slow brightness swell reads as not listening
// (measured 2026-09-26: the aurora and ripples these replaced tracked the
// beat at r ≈ 0.05 and ≈ 0).

// bandAt reads the spectrum at u in 0..1, low to high, interpolating between
// cliamp's bands. With no bands it falls back to the mid level.
func bandAt(a Audio, u float64) float64 {
	n := len(a.Bands)
	if n == 0 {
		return a.Mid
	}
	p := clamp01(u) * float64(n-1)
	i := min(n-2, int(p))
	if i < 0 {
		return clamp01(a.Bands[0])
	}
	return clamp01(a.Bands[i] + (a.Bands[i+1]-a.Bands[i])*(p-float64(i)))
}

// Vortex, after Geiss and MilkDrop: each frame is the last one zoomed,
// turned and faded, with a ring drawn on top whose radius is the spectrum
// (bass at the top, treble at the bottom, mirrored so it closes). Bass pushes
// the zoom, mids the spin; a beat kicks the zoom, throws an outer ring and
// sometimes reverses the spin, so the trails spiral out from the music.
type Vortex struct {
	buf, tmp  []float64 // light over the background, 3 floats a pixel
	spin, hue float64
	prev      float64
}

func (*Vortex) Name() string { return "vortex" }

func (e *Vortex) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	if len(e.buf) != 3*f.W*f.H {
		e.buf, e.tmp = make([]float64, 3*f.W*f.H), make([]float64, 3*f.W*f.H)
	}
	if e.spin == 0 {
		e.spin = 1
	}
	beat := onset(a, &e.prev)
	if beat && rand.Float64() < 0.3 {
		e.spin = -e.spin
	}
	W, H := float64(f.W), float64(f.H)
	cx, cy := W/2, H/2
	steps := dt * 30 // the constants below are per 30 fps frame
	zoom := math.Pow(1.015+0.035*a.Bass+0.09*a.Beat, steps)
	rot := e.spin * (0.008 + 0.03*a.Mid) * steps
	cs, sn := math.Cos(rot), math.Sin(rot)
	decay := math.Pow(0.9, steps)
	for y := range f.H {
		for x := range f.W {
			// Where this pixel's light was last frame: back toward the centre,
			// turned back, with a slow wobble so the trails bend.
			dx, dy := (float64(x)-cx)/zoom, (float64(y)-cy)/zoom
			sx := cx + dx*cs + dy*sn + 0.4*math.Sin(float64(y)*0.13+t*0.7)
			sy := cy - dx*sn + dy*cs + 0.4*math.Sin(float64(x)*0.11-t*0.5)
			x0, y0 := int(math.Floor(sx)), int(math.Floor(sy))
			fx, fy := sx-float64(x0), sy-float64(y0)
			o := 3 * (y*f.W + x)
			for ch := range 3 {
				at := func(px, py int) float64 {
					if px < 0 || px >= f.W || py < 0 || py >= f.H {
						return 0
					}
					return e.buf[3*(py*f.W+px)+ch]
				}
				top := at(x0, y0) + (at(x0+1, y0)-at(x0, y0))*fx
				bot := at(x0, y0+1) + (at(x0+1, y0+1)-at(x0, y0+1))*fx
				e.tmp[o+ch] = (top + (bot-top)*fy) * decay
			}
		}
	}
	e.buf, e.tmp = e.tmp, e.buf

	e.hue += dt/scopeLap + a.Beat*dt*0.3
	hues := th.Gradient(append(metaballHues, metaballHues[0])...)
	// Light adds to a dark stage; on a light theme the trails are ink taken
	// away from the page instead, so sign flips which way from Stage they go.
	deep, sign := th.Stage, 1.0
	if th.Light {
		sign = -1
	}
	R := math.Min(W, H)
	stamp := func(x, y float64, c RGB, k float64) {
		ix, iy := int(math.Round(x)), int(math.Round(y))
		if ix < 0 || ix >= f.W || iy < 0 || iy >= f.H {
			return
		}
		o := 3 * (iy*f.W + ix)
		// max, not sum: overlapping light would add up to white and lose the hue
		e.buf[o] = math.Max(e.buf[o], sign*(float64(c.R)-float64(deep.R))*k)
		e.buf[o+1] = math.Max(e.buf[o+1], sign*(float64(c.G)-float64(deep.G))*k)
		e.buf[o+2] = math.Max(e.buf[o+2], sign*(float64(c.B)-float64(deep.B))*k)
	}
	points := 500 * max(f.W, f.H) / 96
	for s := range points {
		u := float64(s) / float64(points)
		ang := 2*math.Pi*u + t*0.25*e.spin - math.Pi/2
		r := R * (0.1 + 0.3*bandAt(a, 1-math.Abs(2*u-1)) + 0.05*a.Beat)
		c := hues.At(frac(e.hue + 0.25*math.Abs(2*u-1)))
		stamp(cx+r*math.Cos(ang), cy+r*math.Sin(ang), c, 1)
		if beat {
			stamp(cx+R*0.45*math.Cos(ang), cy+R*0.45*math.Sin(ang), th.Bright, 0.9)
		}
	}
	for i := range f.Px {
		o := 3 * i
		f.Px[i] = RGB{u8(float64(deep.R) + sign*e.buf[o]), u8(float64(deep.G) + sign*e.buf[o+1]), u8(float64(deep.B) + sign*e.buf[o+2])}
	}
}

// Water, after AVS's Water Bump: a height field running the wave equation.
// Every beat drops a stone in the centre, sized by the bass, so the rhythm
// shows as rings marching outward; each band drips into its own strip
// (bass on the left) as often as it is loud. Crests catch the light,
// troughs go dark.
type Water struct {
	cur, old []float64
	acc      float64 // simulated time not yet stepped
	prev     float64
}

func (*Water) Name() string { return "water" }

const (
	waterHz   = 60.0 // simulation steps a second; the waves travel a pixel a step
	waterDamp = 0.98 // per step, so a splash settles before the next beat
)

func (e *Water) drop(w, h int, x, y, r, amp float64) {
	for py := max(1, int(y-r)); py <= min(h-2, int(y+r)); py++ {
		for px := max(1, int(x-r)); px <= min(w-2, int(x+r)); px++ {
			if d := math.Hypot(float64(px)-x, float64(py)-y); d < r {
				e.cur[py*w+px] += amp * (0.5 + 0.5*math.Cos(math.Pi*d/r))
			}
		}
	}
}

func (e *Water) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	w, h := f.W, f.H
	if len(e.cur) != w*h {
		e.cur, e.old = make([]float64, w*h), make([]float64, w*h)
	}
	if w < 3 || h < 3 {
		return
	}
	W, H := float64(w), float64(h)
	if onset(a, &e.prev) { // always the centre, so the beat reads as rings marching out
		e.drop(w, h, W/2, H/2, H*(0.06+0.08*a.Bass), 10*(0.5+a.Bass))
	}
	bands := a.Bands
	if len(bands) == 0 {
		bands = []float64{a.Mid}
	}
	for j, v := range bands { // each band rains in its own strip, bass on the left, as often as it is loud
		if rand.Float64() < 4*v*v*dt {
			e.drop(w, h, (float64(j)+0.5)/float64(len(bands))*W, H*(0.15+0.7*rand.Float64()), 1.5+1.5*v, 2+4*v)
		}
	}
	e.acc = math.Min(e.acc+dt, 4/waterHz)
	for ; e.acc >= 1/waterHz; e.acc -= 1 / waterHz {
		for y := 1; y < h-1; y++ {
			for x := 1; x < w-1; x++ {
				i := y*w + x
				e.old[i] = ((e.cur[i-1]+e.cur[i+1]+e.cur[i-w]+e.cur[i+w])/2 - e.old[i]) * waterDamp
			}
		}
		e.cur, e.old = e.old, e.cur
	}
	deep, blue, cyan := th.Stage, th.Color("blue"), th.Color("cyan")
	rest := lerp(deep, blue, 0.2)
	if th.Light {
		rest = lerp(deep, blue, 0.35) // a tint on paper needs more pigment than a glow in the dark
	}
	light := 0.5 + 0.3*a.Bass + 0.2*a.Beat // the pool brightens with the bass and flashes on the beat
	for y := range h {
		for x := range w {
			i := y*w + x
			slope := 0.0
			if x > 0 && x < w-1 && y > 0 && y < h-1 {
				slope = e.cur[i-1] - e.cur[i+1] + e.cur[i-w] - e.cur[i+w] // lit from the top left
			}
			s := e.cur[i]*0.05 + slope*0.25
			c := rest
			switch {
			case s > 0.5:
				c = th.Glow(cyan, clamp01(s-0.5))
			case s > 0:
				c = lerp(rest, cyan, s*2)
			default:
				c = lerp(rest, deep, clamp01(-s*2))
			}
			f.Px[i] = lerp(deep, c, light)
		}
	}
}

// Fountain, after AVS's Dot Fountain: one jet per band along the floor, bass
// on the left. Each jet throws its spray as high as its band is loud, so the
// skyline of the spray is the spectrum; a beat fires every jet at once.
type Fountain struct {
	drops []spray
	carry []float64 // fractional drops owed to each jet
	prev  float64
}

type spray struct {
	x, y, vx, vy float64
	c            RGB
}

func (*Fountain) Name() string { return "fountain" }

func (e *Fountain) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	deep := th.Stage
	for i := range f.Px { // trails
		f.Px[i] = lerp(f.Px[i], deep, 0.4)
	}
	W, H := float64(f.W), float64(f.H)
	n := max(1, len(a.Bands))
	if len(e.carry) != n {
		e.carry = make([]float64, n)
	}
	hues := th.Gradient("red", "orange", "yellow", "green", "cyan", "blue", "magenta")
	g := H * 2.2 // pixels a second², so a full-height jet takes ~2 s to fall back
	beat := onset(a, &e.prev)
	for j := range n {
		u := (float64(j) + 0.5) / float64(n)
		lvl := bandAt(a, float64(j)/math.Max(1, float64(n-1)))
		c := hues.At(u)
		launch := func(boost float64, c RGB) {
			v := math.Sqrt(2*g*H*(0.08+0.85*lvl)) * boost * (0.9 + 0.2*rand.Float64())
			e.drops = append(e.drops, spray{u*W + (rand.Float64()-0.5)*W/float64(n)*0.4, H - 1, (rand.Float64() - 0.5) * W / float64(n) * 1.6, -v, c})
		}
		for e.carry[j] += (10 + 70*lvl) * dt; e.carry[j] >= 1; e.carry[j]-- {
			launch(1, c)
		}
		if beat {
			for range 8 {
				launch(1.15, th.Glow(c, 0.6))
			}
		}
	}
	if len(e.drops) > 4000 {
		e.drops = e.drops[len(e.drops)-4000:]
	}
	live := e.drops[:0]
	for _, d := range e.drops {
		px, py := d.x, d.y
		d.vy += g * dt
		d.x += d.vx * dt
		d.y += d.vy * dt
		if d.y >= H || d.x < 0 || d.x >= W {
			continue
		}
		steps := max(1, int(math.Ceil(math.Hypot(d.x-px, d.y-py))))
		for k := 0; k <= steps; k++ {
			q := float64(k) / float64(steps)
			blend(f, int(px+(d.x-px)*q), int(py+(d.y-py)*q), d.c, 0.5+0.4*q)
		}
		live = append(live, d)
	}
	e.drops = live
}

// Spectrum, the Winamp 2 main-window analyzer drawn in braille dots (a
// GlyphEffect: 2×4 dots a cell): bars are the bands, green at the floor
// through yellow to red at the top, and a cap hangs at each bar's peak for a
// moment before dropping.
type Spectrum struct {
	peaks, hold []float64 // cap heights in dots; seconds each cap still hangs
	lit         []uint8   // per dot: 0 dark, 1 bar, 2 cap
	cells       Cells
}

func (*Spectrum) Name() string { return "spectrum" }

const (
	capHold = 0.25 // seconds a cap hangs at its peak
	capFall = 0.85 // frame heights a second a cap drops once it lets go
)

func (e *Spectrum) RenderCells(c *Cells, a Audio, t, dt float64, th *Theme) {
	W, H := c.W*2, c.H*4 // dot resolution
	n := max(8, c.W/3)
	if len(e.peaks) != n {
		e.peaks, e.hold = make([]float64, n), make([]float64, n)
	}
	if len(e.lit) != W*H {
		e.lit = make([]uint8, W*H)
	}
	clear(e.lit)
	bw := float64(W) / float64(n)
	for i := range n {
		h := math.Round(bandAt(a, float64(i)/float64(n-1)) * float64(H-2))
		if h >= e.peaks[i] {
			e.peaks[i], e.hold[i] = h, capHold
		} else if e.hold[i] -= dt; e.hold[i] < 0 {
			e.peaks[i] = math.Max(0, e.peaks[i]-capFall*float64(H)*dt)
		}
		capY := H - 2 - int(math.Round(e.peaks[i]))
		for x := int(math.Round(float64(i) * bw)); x < int(math.Round(float64(i+1)*bw))-1; x++ { // a dot column of gap
			for y := range int(h) {
				e.lit[(H-1-y)*W+x] = 1
			}
			if capY >= 0 && e.lit[capY*W+x] == 0 {
				e.lit[capY*W+x] = 2
			}
		}
	}
	ramp := th.Gradient("green", "green", "yellow", "orange", "red")
	bits := [4][2]rune{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}
	for cy := range c.H {
		for cx := range c.W {
			var ch rune
			bar := false
			for r := range 4 {
				for s := range 2 {
					if k := e.lit[(cy*4+r)*W+cx*2+s]; k > 0 {
						ch |= bits[r][s]
						bar = bar || k == 1
					}
				}
			}
			i := cy*c.W + cx
			switch {
			case ch == 0:
				c.Ch[i] = 0
			case bar:
				c.Ch[i], c.Fg[i] = 0x2800+ch, ramp.At(1-(float64(cy)+0.5)/float64(c.H))
			default:
				c.Ch[i], c.Fg[i] = 0x2800+ch, th.Color("foreground")
			}
		}
	}
}

// Render draws the bars as pixels, for anything that cannot show glyphs.
func (e *Spectrum) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	e.cells.Resize(f.W, f.H/2)
	e.RenderCells(&e.cells, a, t, dt, th)
	cellsToPixels(f, &e.cells, th.Stage)
}

// Timescope, after AVS: a spectrogram scrolling left, bass at the bottom and
// colour for loudness, so the song's structure goes by. The screen always
// holds timescopeSpan seconds whatever its width; a beat ticks the floor.
type Timescope struct {
	acc float64 // columns owed
}

const timescopeSpan = 4.0

func (*Timescope) Name() string { return "timescope" }

func (e *Timescope) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	heat := th.Heat("blue", "magenta", "orange", "yellow")
	e.acc += dt * float64(f.W) / timescopeSpan
	for ; e.acc >= 1; e.acc-- {
		for y := range f.H {
			row := f.Px[y*f.W : (y+1)*f.W]
			copy(row, row[1:])
			row[f.W-1] = heat.At(math.Pow(bandAt(a, 1-float64(y)/float64(max(1, f.H-1))), 1.3) * 0.95)
		}
		if a.Beat > 0.95 {
			for y := max(0, f.H-3); y < f.H; y++ {
				f.Px[y*f.W+f.W-1] = th.Bright
			}
		}
	}
}

// Synaesthesia, after the XMMS plugin: pitch is position, bass on the left.
// Each band breathes colour onto the centre line, as bright as it is loud,
// and it streams outward, up and down, fading as it goes. No blur: a blurred
// version read as fuzz.
type Synaesthesia struct {
	buf, tmp []float64 // 3 floats a pixel
	acc      float64
}

func (*Synaesthesia) Name() string { return "synaesthesia" }

const (
	smokeHz   = 30.0 // drift steps a second: the colour moves a pixel a step
	smokeFade = 0.95 // per step
)

func (e *Synaesthesia) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	w, h := f.W, f.H
	deep := th.Stage
	if len(e.buf) != 3*w*h {
		e.buf, e.tmp = make([]float64, 3*w*h), make([]float64, 3*w*h)
		for i := range w * h {
			e.buf[3*i], e.buf[3*i+1], e.buf[3*i+2] = float64(deep.R), float64(deep.G), float64(deep.B)
		}
	}
	if w < 2 || h < 2 {
		return
	}
	d := [3]float64{float64(deep.R), float64(deep.G), float64(deep.B)}
	hues := th.Gradient(metaballHues...)
	mid := h / 2
	e.acc = math.Min(e.acc+dt, 3/smokeHz)
	for ; e.acc >= 1/smokeHz; e.acc -= 1 / smokeHz {
		for y := range h {
			sy := max(mid, y-1) // below the line, smoke comes from the row above
			if y < mid {
				sy = min(mid-1, y+1)
			}
			for x := range w {
				o, src := 3*(y*w+x), 3*(sy*w+x)
				for c := range 3 {
					e.tmp[o+c] = d[c] + (e.buf[src+c]-d[c])*smokeFade
				}
			}
		}
		e.buf, e.tmp = e.tmp, e.buf
		for x := range w {
			u := float64(x) / float64(w-1)
			v := bandAt(a, u)
			c := th.Glow(hues.At(u), v*v*v)                                                 // loud bands burn at the core
			v *= v * (0.5 + 0.5*math.Sin(float64(x)*2.1+math.Sin(float64(x)*0.37+t*0.4)*3)) // quiet bands stay dark; filaments keep it crisp
			for _, y := range []int{mid - 1, mid} {
				o := 3 * (y*w + x)
				e.buf[o] += (float64(c.R) - e.buf[o]) * v
				e.buf[o+1] += (float64(c.G) - e.buf[o+1]) * v
				e.buf[o+2] += (float64(c.B) - e.buf[o+2]) * v
			}
		}
	}
	for i := range f.Px {
		f.Px[i] = RGB{u8(e.buf[3*i]), u8(e.buf[3*i+1]), u8(e.buf[3*i+2])}
	}
}
