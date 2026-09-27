package fx

import (
	"math"
	"math/rand/v2"
)

// Patterns that grow or drift on their own and are pushed by the music:
// they keep state between frames, so each one reallocates and reseeds when
// the stage resizes.

// loudestBand is the index of the band playing loudest, or 0 with no bands.
func loudestBand(a Audio) int {
	best := 0
	for i, v := range a.Bands {
		if v > a.Bands[best] {
			best = i
		}
	}
	return best
}

// underBand is a column under band i of the n bands, bass on the left.
func underBand(i, n, w int) int {
	return min(w-1, int((float64(i)+rand.Float64())/float64(max(1, n))*float64(w)))
}

// Coral: a Gray–Scott reaction–diffusion, two chemicals that react and
// spread into coral and fingerprint patterns. Every beat drops a seed under
// the loudest band, bass on the left, which blooms outward; bass speeds the
// growth, mids move the feed rate and so the kind of pattern, and loudness
// sets how brightly it glows.
type Coral struct {
	a, b, na, nb []float64
	acc, prev    float64
}

func (*Coral) Name() string { return "coral" }

const coralHz = 90.0 // reaction steps a second at rest

func (e *Coral) seed(w, h, x, y, r int) {
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			e.b[((y+dy+h)%h)*w+(x+dx+w)%w] = 1
		}
	}
}

func (e *Coral) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	w, h := f.W, f.H
	if w < 3 || h < 3 {
		return
	}
	if len(e.a) != w*h {
		e.a, e.b = make([]float64, w*h), make([]float64, w*h)
		e.na, e.nb = make([]float64, w*h), make([]float64, w*h)
		for i := range e.a {
			e.a[i] = 1
		}
		for range max(4, w*h/300) {
			e.seed(w, h, rand.IntN(w), rand.IntN(h), 2)
		}
	}
	if onset(a, &e.prev) {
		e.seed(w, h, underBand(loudestBand(a), len(a.Bands), w), rand.IntN(h), 2)
	}
	feed, kill := 0.03+0.028*a.Mid, 0.057+0.006*a.Mid
	e.acc = math.Min(e.acc+dt*coralHz*(0.4+1.2*a.Bass), 12)
	for ; e.acc >= 1; e.acc-- {
		for y := range h {
			up, dn := ((y-1+h)%h)*w, ((y+1)%h)*w
			for x := range w {
				l, r, i := y*w+(x-1+w)%w, y*w+(x+1)%w, y*w+x
				la := e.a[l] + e.a[r] + e.a[up+x] + e.a[dn+x] - 4*e.a[i]
				lb := e.b[l] + e.b[r] + e.b[up+x] + e.b[dn+x] - 4*e.b[i]
				abb := e.a[i] * e.b[i] * e.b[i]
				e.na[i] = clamp01(e.a[i] + 0.2*la - abb + feed*(1-e.a[i]))
				e.nb[i] = clamp01(e.b[i] + 0.1*lb + abb - (kill+feed)*e.b[i])
			}
		}
		e.a, e.na = e.na, e.a
		e.b, e.nb = e.nb, e.b
	}
	live := 0.0
	g := th.Heat("magenta", "accent", "cyan")
	lum := 0.45 + 0.55*(a.Bass+a.Mid+a.Treble)/3
	for i, v := range e.b {
		live += v
		f.Px[i] = th.Glow(g.At(v*2.6*lum), a.Beat*0.4*clamp01(v*3))
	}
	if live < float64(w*h)*0.01 { // died out: start again from the middle
		e.seed(w, h, w/2, h/2, 3)
	}
}

// Flow: particles riding a slowly turning current, after the flow-field
// drawings of generative art. Each carries one band and paints its trail in
// that band's heat, cool when quiet and hot when loud; treble speeds the
// current, loudness thickens the ink, and a beat throws a burst of them out
// from the centre.
type Flow struct {
	ps   []flowDot
	buf  []float64 // light over the stage, 3 floats a pixel, as in Vortex
	prev float64
}

type flowDot struct {
	x, y, life float64
	band       int
}

func (*Flow) Name() string { return "flow" }

func (e *Flow) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	W, H := float64(f.W), float64(f.H)
	if len(e.buf) != 3*f.W*f.H {
		e.buf = make([]float64, 3*f.W*f.H)
		e.ps = make([]flowDot, f.W*f.H/12)
		for i := range e.ps {
			e.ps[i] = flowDot{rand.Float64() * W, rand.Float64() * H, rand.Float64() * 4, i % max(1, len(a.Bands))}
		}
	}
	decay := math.Pow(0.93, dt*30)
	for i := range e.buf {
		e.buf[i] *= decay
	}
	deep, sign := th.Stage, 1.0
	if th.Light {
		sign = -1 // ink taken from the page, as in Vortex
	}
	heat := th.Gradient("blue", "magenta", "red", "orange", "yellow")
	burst := onset(a, &e.prev)
	speed := H * 0.25 * (0.5 + 2*a.Treble)
	ink := 0.35 + 0.65*(a.Bass+a.Mid+a.Treble)/3 + 0.3*a.Beat
	nb := max(1, len(a.Bands))
	for i := range e.ps {
		p := &e.ps[i]
		if burst && rand.Float64() < 0.15 {
			p.x, p.y, p.life = W/2+rand.NormFloat64()*2, H/2+rand.NormFloat64()*2, 2+rand.Float64()*2
		}
		ang := fbm(p.x*0.035, p.y*0.035+t*0.07) * 4 * math.Pi
		p.x += math.Cos(ang) * speed * dt
		p.y += math.Sin(ang) * speed * dt
		p.life -= dt
		if p.x < 0 || p.x >= W || p.y < 0 || p.y >= H || p.life < 0 {
			p.x, p.y, p.life = rand.Float64()*W, rand.Float64()*H, 2+rand.Float64()*3
		}
		c := heat.At(bandAt(a, float64(p.band%nb)/float64(max(1, nb-1))))
		o := 3 * (int(p.y)*f.W + int(p.x))
		e.buf[o] = math.Max(e.buf[o], sign*(float64(c.R)-float64(deep.R))*ink)
		e.buf[o+1] = math.Max(e.buf[o+1], sign*(float64(c.G)-float64(deep.G))*ink)
		e.buf[o+2] = math.Max(e.buf[o+2], sign*(float64(c.B)-float64(deep.B))*ink)
	}
	for i := range f.Px {
		o := 3 * i
		f.Px[i] = RGB{u8(float64(deep.R) + sign*e.buf[o]), u8(float64(deep.G) + sign*e.buf[o+1]), u8(float64(deep.B) + sign*e.buf[o+2])}
	}
}

// Life: Conway's Game of Life, stepped at the music's pace. Every beat drops
// a fresh cluster under the loudest band, bass on the left; treble sets how
// many generations a second run, cells that die leave embers that cool, and
// a beat flashes the living.
type Life struct {
	g, n      []uint8
	heat      []float64
	acc, prev float64
}

func (*Life) Name() string { return "life" }

func (e *Life) drop(w, h, cx, cy int) {
	for dy := -4; dy <= 4; dy++ {
		for dx := -4; dx <= 4; dx++ {
			if rand.Float64() < 0.45 {
				e.g[((cy+dy+h)%h)*w+(cx+dx+w)%w] = 1
			}
		}
	}
}

func (e *Life) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	w, h := f.W, f.H
	if w < 3 || h < 3 {
		return
	}
	if len(e.g) != w*h {
		e.g, e.n, e.heat = make([]uint8, w*h), make([]uint8, w*h), make([]float64, w*h)
		for range max(3, w*h/400) {
			e.drop(w, h, rand.IntN(w), rand.IntN(h))
		}
	}
	if onset(a, &e.prev) {
		e.drop(w, h, underBand(loudestBand(a), len(a.Bands), w), rand.IntN(h))
	}
	e.acc = math.Min(e.acc+dt*(6+14*a.Treble), 3)
	pop := 0
	for ; e.acc >= 1; e.acc-- {
		pop = 0
		for y := range h {
			for x := range w {
				s := 0
				for dy := -1; dy <= 1; dy++ {
					row := ((y + dy + h) % h) * w
					for dx := -1; dx <= 1; dx++ {
						if dx != 0 || dy != 0 {
							s += int(e.g[row+(x+dx+w)%w])
						}
					}
				}
				i := y*w + x
				e.n[i] = 0
				if s == 3 || (s == 2 && e.g[i] == 1) {
					e.n[i] = 1
					pop++
				}
			}
		}
		e.g, e.n = e.n, e.g
		if pop < w*h/60 { // settled into still lifes: stir it
			e.drop(w, h, rand.IntN(w), rand.IntN(h))
		}
	}
	alive := th.Glow(th.Color("accent"), 0.3+0.6*a.Beat)
	embers := th.Heat("magenta", "red", "orange")
	cool := math.Pow(0.08, dt)
	for i := range f.Px {
		if e.g[i] == 1 {
			e.heat[i] = 1
			f.Px[i] = alive
			continue
		}
		e.heat[i] *= cool
		f.Px[i] = embers.At(e.heat[i] * 0.85)
	}
}

// Moire: two sets of rings drifting across each other, after the op art of
// Bridget Riley, each painted in a different theme hue so their overlap
// makes a third. The hues creep round the theme and jump on every beat;
// bass tightens the rings, mids speed the drift, and loudness sets how
// strongly they stand off the stage.
type Moire struct{ ph, hue, prev float64 }

func (*Moire) Name() string { return "moire" }

func (e *Moire) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	W, H := float64(f.W), float64(f.H)
	e.ph += dt * (0.25 + 0.7*a.Mid)
	e.hue += dt / metaballLap
	if onset(a, &e.prev) {
		e.hue += 1 / float64(len(metaballHues))
	}
	hues := th.Gradient(append(metaballHues, metaballHues[0])...)
	ca, cb := hues.At(frac(e.hue)), hues.At(frac(e.hue+0.4))
	ax, ay := W/2+math.Sin(e.ph)*W*0.28, H/2+math.Cos(e.ph*0.8)*H*0.25
	bx, by := W/2-math.Sin(e.ph*1.1)*W*0.28, H/2-math.Cos(e.ph*0.7)*H*0.25
	fq := 2 * math.Pi / (math.Max(H, 8) / 7) * (1 + 0.4*a.Bass) // about seven rings down the frame
	lum := 0.55 + 0.45*(a.Bass+a.Mid+a.Treble)/3
	for y := range f.H {
		for x := range f.W {
			fx, fy := float64(x), float64(y)
			ka := smoothstep(-0.35, 0.35, math.Sin(math.Hypot(fx-ax, fy-ay)*fq-t*2))
			kb := smoothstep(-0.35, 0.35, math.Sin(math.Hypot(fx-bx, fy-by)*fq-t*2.3+a.Beat*2))
			c := lerp(th.Stage, ca, ka*0.9)
			c = lerp(c, cb, kb*0.6)
			f.Px[y*f.W+x] = th.Glow(lerp(th.Stage, c, lum), a.Beat*0.3)
		}
	}
}

// Braille: the whole pane is braille, and theme colours flow through it as
// slow currents. How many of a cell's eight dots are lit is how loud its
// part of the spectrum is, bass on the left, so the page thickens and thins
// with the music; each dot has a fixed threshold, so a rising level fills
// cells dot by dot instead of flickering. Loudness brightens the colours,
// bass hurries the currents, and a beat sends a filled ring out from the
// centre.
type Braille struct {
	hue, drift float64
	waves      []float64 // ring radii in dots, one per recent beat
	prev       float64
	cells      Cells // Render's pixel fallback draws through this
}

func (*Braille) Name() string { return "braille" }

const (
	brailleFloor = 0.18 // share of dots lit in silence, so the page never empties
	brailleRing  = 3.0  // half-width of a beat ring, in dots
)

func (e *Braille) RenderCells(c *Cells, a Audio, t, dt float64, th *Theme) {
	W, H := float64(c.W*2), float64(c.H*4) // dot resolution
	if W == 0 || H == 0 {
		return
	}
	e.hue += dt/metaballLap + a.Beat*dt*0.2
	e.drift += dt * (0.15 + 0.5*a.Bass)
	if onset(a, &e.prev) {
		e.waves = append(e.waves, 0)
	}
	reach := math.Hypot(W, H) / 2
	live := e.waves[:0]
	for _, r := range e.waves {
		if r += dt * reach * 1.2; r < reach+brailleRing {
			live = append(live, r)
		}
	}
	e.waves = live

	hues := th.Gradient(append(metaballHues, metaballHues[0])...)
	lum := 0.35 + 0.65*(a.Bass+a.Mid+a.Treble)/3
	bits := [4][2]rune{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}
	for cy := range c.H {
		for cx := range c.W {
			var ch rune
			level := 0.0
			for r := range 4 {
				for s := range 2 {
					x, y := float64(cx*2+s), float64(cy*4+r)
					v := math.Sqrt(bandAt(a, x/W)) * (0.3 + 1.1*fbm(x*0.04-e.drift, y*0.05+e.drift*0.5))
					d := math.Hypot(x-W/2, y-H/2)
					for _, wr := range e.waves {
						v += 0.7 * clamp01(1-math.Abs(d-wr)/brailleRing) * (1 - wr/reach)
					}
					v = brailleFloor + (1-brailleFloor)*clamp01(v)
					if hash2(int64(x), int64(y)) < v {
						ch |= bits[r][s]
					}
					level += v / 8
				}
			}
			i := cy*c.W + cx
			if ch == 0 {
				c.Ch[i] = 0
				continue
			}
			fx, fy := float64(cx), float64(cy)
			p := e.hue + 2.2*fbm(fx*0.03+e.drift*0.7, fy*0.07-e.drift*0.4) // several hues at once, in drifting bands
			col := lerp(th.Stage, hues.At(frac(p)), 0.55+0.45*clamp01(level*lum*1.6))
			c.Ch[i], c.Fg[i] = 0x2800+ch, th.Glow(col, a.Beat*0.25)
		}
	}
}

// Render draws the page as pixels, lit where a cell has any dots, for
// anything that cannot show glyphs.
func (e *Braille) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	e.cells.Resize(f.W, f.H/2)
	e.RenderCells(&e.cells, a, t, dt, th)
	cellsToPixels(f, &e.cells, th.Stage)
}
