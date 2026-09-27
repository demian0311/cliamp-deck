package fx

import (
	"math"
	"math/rand/v2"
)

// Plasma: summed sine fields run through the theme's hue wheel.
// Palette drift ← bass, field scale ← mid, flash ← beat.
type Plasma struct{ phase float64 }

func (*Plasma) Name() string { return "plasma" }

func (e *Plasma) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	wheel := th.Gradient("blue", "cyan", "green", "yellow", "orange", "red", "magenta", "blue")
	e.phase += dt * (0.8 + a.Bass*2)
	k := (0.16 + a.Mid*0.12) * (1 - a.Beat*0.12) // the field breathes in on a beat
	sp := e.phase
	cx0, cy0 := float64(f.W)/2, float64(f.H)/2
	for y := range f.H {
		for x := range f.W {
			fx, fy := float64(x), float64(y)
			cx, cy := fx-cx0, fy-cy0
			v := math.Sin(fx*k+sp) + math.Sin((fy*k*1.3+sp)*0.8) +
				math.Sin((cx*k+cy*k+sp)*0.6) + math.Sin(math.Hypot(cx, cy)*k*0.9-sp*1.4)
			c := wheel.At(frac(v/8 + sp*0.05 + a.Bass*0.3))
			c = lerp(th.Background, c, 0.55+math.Max(0, v)*0.1)
			f.Px[y*f.W+x] = th.Glow(c, a.Beat*0.35)
		}
	}
}

// Fire: flames as a smooth noise field rising through a flame-shaped mask,
// so tongues billow and merge instead of flickering pixel by pixel. Each
// column's flame follows one of cliamp's bands, laid low to high across the
// screen, so the tongues dance to their part of the music; energy speeds the
// billowing, and a beat flares the whole fire and throws a burst of embers.
type Fire struct {
	cols   []float64 // smoothed flame height per column
	clock  float64   // flame time: runs faster when the music is busy
	flare  float64   // brightness kick from the last beat, decaying
	prev   float64   // last frame's beat, for onsets
	embers []ember
}

type ember struct{ x, y, vx, vy, life float64 }

func (*Fire) Name() string { return "fire" }

func (e *Fire) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	W, H := f.W, f.H
	if W < 1 || H < 2 {
		return
	}
	if len(e.cols) != W {
		e.cols = make([]float64, W)
	}
	e.clock += dt * (0.5 + (a.Bass+a.Mid)*1.1)
	e.flare *= math.Exp(-dt * 5)
	beat := onset(a, &e.prev)
	if beat {
		e.flare = 1
	}
	band := func(x int) float64 { // the column's band, interpolated so neighbours blend
		n := len(a.Bands)
		if n < 2 {
			return a.Bass
		}
		pos := float64(x) / float64(max(1, W-1)) * float64(n-1)
		i := min(int(pos), n-2)
		return clamp01(a.Bands[i] + (a.Bands[i+1]-a.Bands[i])*(pos-float64(i)))
	}
	fh := float64(H)
	tops := make([]float64, W)
	for x := range W {
		target := 0.22 + band(x)*0.55 + e.flare*0.18
		rate := 12.0 // leap up on a hit, settle back slowly, like a flame catching
		if target < e.cols[x] {
			rate = 2.5
		}
		e.cols[x] += (target - e.cols[x]) * math.Min(1, dt*rate)
		tops[x] = e.cols[x] * (0.8 + 0.4*fbm(float64(x)/fh*1.4, e.clock*0.35))
	}
	heatmap := th.Heat("red", "orange", "yellow")
	c := e.clock
	for y := range H {
		v := 1 - float64(y)/fh // 0 at the bottom, 1 at the top
		for x := range W {
			u := float64(x) / fh // square units, so blobs keep their shape at any width
			// Domain-warped noise scrolling upward: q bends the field, n is the flame body.
			q := fbm(u*2.2, v*1.6-c*0.9)
			n := fbm(u*3.2+q*1.1, v*2.4-c*1.9)
			heat := clamp01((1-v/tops[x])*0.95 + (n-0.5)*1.6 + e.flare*0.22*(1-v))
			f.Px[y*W+x] = heatmap.At(math.Pow(heat, 1.3) * 0.9) // white only in the hottest cores
		}
	}
	// Embers: a burst on each beat, and a trickle on the treble.
	spawn := int(a.Treble*a.Treble*8*dt*30 + rand.Float64()*0.5)
	if beat {
		spawn += 6 + int(a.Bass*10)
	}
	for range spawn {
		x := rand.Float64() * float64(W)
		e.embers = append(e.embers, ember{x, fh * (1 - tops[min(W-1, int(x))]*0.7), rand.Float64()*6 - 3, -(fh*0.35 + rand.Float64()*fh*0.4), 1})
	}
	spark := th.Color("yellow")
	live := e.embers[:0]
	for _, m := range e.embers {
		m.x += (m.vx + math.Sin(t*3+m.y*0.2)*4) * dt
		m.y += m.vy * dt
		m.life -= dt * 0.9
		if m.life <= 0 || m.y < 0 {
			continue
		}
		if ix, iy := int(m.x), int(m.y); ix >= 0 && ix < W && iy >= 0 && iy < H {
			f.Px[iy*W+ix] = lerp(f.Px[iy*W+ix], th.Glow(spark, m.life*0.5), m.life)
		}
		live = append(live, m)
	}
	e.embers = live
}

// fbm is three octaves of smooth value noise, about 0..1.
func fbm(x, y float64) float64 {
	return 0.55*vnoise(x, y) + 0.3*vnoise(x*2.03+17.1, y*2.03-4.7) + 0.15*vnoise(x*4.1-9.3, y*4.1+11.9)
}

// vnoise is value noise: hashed lattice values, smoothly interpolated.
func vnoise(x, y float64) float64 {
	ix, iy := math.Floor(x), math.Floor(y)
	fx, fy := x-ix, y-iy
	fx, fy = fx*fx*(3-2*fx), fy*fy*(3-2*fy)
	i, j := int64(ix), int64(iy)
	a, b := hash2(i, j), hash2(i+1, j)
	c, d := hash2(i, j+1), hash2(i+1, j+1)
	return a + (b-a)*fx + (c-a)*fy + (a-b-c+d)*fx*fy
}

func hash2(x, y int64) float64 {
	h := uint64(x)*0x9E3779B97F4A7C15 ^ uint64(y)*0xC2B2AE3D27D4EB4F
	h ^= h >> 31
	h *= 0xBF58476D1CE4E5B9
	h ^= h >> 29
	return float64(h>>11) / float64(1<<53)
}

// metaballHues are the theme colours the metaballs cycle through.
var metaballHues = []string{"red", "orange", "yellow", "green", "cyan", "blue", "magenta"}

// metaballLap is how many seconds the colours take to go round the hues once.
const metaballLap = 40.0

// Metaballs: five blobs whose sizes follow their own band groups.
// Size ← bass/mid/treble per ball, glow ← beat.
type Metaballs struct{}

func (*Metaballs) Name() string { return "metaballs" }

func (*Metaballs) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	type ball struct{ x, y, r2 float64 }
	var balls [5]ball
	W, H := float64(f.W), float64(f.H)
	for i := range balls {
		g := [5]float64{a.Bass, a.Bass, a.Mid, a.Mid, a.Treble}[i]
		fi := float64(i)
		r := 2 + g*math.Min(W, H*2)*0.09
		balls[i] = ball{
			x:  W/2 + math.Cos(t*(0.4+fi*0.13)+fi*1.7)*W*0.3,
			y:  H/2 + math.Sin(t*(0.5+fi*0.11)+fi)*H*0.32,
			r2: r * r,
		}
	}
	// Each ball wears its own theme colour, and all of them drift round the
	// theme's hues (one lap per metaballLap seconds), so over time every
	// colour in the theme comes through. Where balls merge, colours blend by
	// how much each contributes.
	hues := th.Gradient(append(metaballHues, metaballHues[0])...)
	var cols [5]RGB
	for i := range cols {
		p := t/metaballLap + float64(i)/float64(len(balls))
		cols[i] = hues.At(p - math.Floor(p))
	}
	for y := range f.H {
		for x := range f.W {
			field, r, g, b := 0.0, 0.0, 0.0, 0.0
			for i, bl := range balls {
				dx, dy := float64(x)-bl.x, float64(y)-bl.y
				w := bl.r2 / (dx*dx + dy*dy + 1)
				field += w
				r += w * float64(cols[i].R)
				g += w * float64(cols[i].G)
				b += w * float64(cols[i].B)
			}
			mix := RGB{uint8(r / field), uint8(g / field), uint8(b / field)}
			// One continuous ramp from the background into each ball, so the
			// edge is a fade rather than a rim; cores then warm toward white.
			body := smoothstep(0.06, 1.5, field)
			c := lerp(th.Background, mix, body)
			f.Px[y*f.W+x] = th.Glow(c, math.Min(0.45, math.Max(0, field-1.5)*0.08+a.Beat*0.2*body))
		}
	}
}

// smoothstep eases 0→1 as x goes from lo to hi, flat at both ends.
func smoothstep(lo, hi, x float64) float64 {
	t := clamp01((x - lo) / (hi - lo))
	return t * t * (3 - 2*t)
}
