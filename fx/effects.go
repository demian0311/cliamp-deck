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
			f.Px[y*f.W+x] = lerp(c, th.Bright, a.Beat*0.35)
		}
	}
}

// Tunnel: a checkered tube flown down its axis.
// Flight speed ← bass, twist ← mid, ring flash ← beat.
type Tunnel struct{ z float64 }

func (*Tunnel) Name() string { return "tunnel" }

func (e *Tunnel) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	grad := th.Gradient("darker_background", "blue", "accent", "cyan", "bright_foreground")
	e.z += dt * (0.6 + a.Bass*2.7)
	twist := math.Sin(t*0.3) * 0.6 * a.Mid
	for y := range f.H {
		for x := range f.W {
			dx := (float64(x) - float64(f.W)/2) / float64(f.W) * 2
			dy := (float64(y) - float64(f.H)/2) / float64(f.W) * 2 // pixels are square: one scale
			d := math.Hypot(dx, dy) + 1e-4
			u := 0.35/d + e.z
			v := math.Atan2(dy, dx)/math.Pi + twist*u*0.2 + t*0.05
			ring := int(math.Floor(u * 6))
			checker := (ring + int(math.Floor(v*8))) & 1
			c := grad.At(0.35 + frac(u*0.08)*0.6)
			if checker == 0 {
				c = lerp(th.Background, c, 0.35)
			}
			if ring%4 == 0 {
				c = lerp(c, th.Bright, a.Beat*0.6)
			}
			f.Px[y*f.W+x] = lerp(th.Background, c, math.Min(1, d*1.4)) // fade the vanishing point
		}
	}
}

// Fire: the classic heat-propagation buffer.
// Heat ← bass, sparks ← treble, flare ← beat.
type Fire struct{ heat []float64 }

func (*Fire) Name() string { return "fire" }

func (e *Fire) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	W, H := f.W, f.H
	if W < 1 || H < 2 {
		return
	}
	if len(e.heat) != W*H {
		e.heat = make([]float64, W*H)
	}
	for x := range W {
		e.heat[(H-1)*W+x] = math.Min(1, (0.55+a.Bass*0.6+a.Beat*0.4)*(0.6+rand.Float64()*0.4))
	}
	for range int(a.Treble * float64(W) * 0.2) {
		e.heat[(H-2)*W+rand.IntN(W)] = 1
	}
	heatmap := th.Gradient("darker_background", "red", "orange", "yellow", "bright_foreground")
	cool := 40 / float64(H)
	for y := range H - 1 {
		for x := range W {
			src := (x + rand.IntN(3) - 1 + W) % W
			e.heat[y*W+x] = math.Max(0, e.heat[(y+1)*W+src]-(0.012+rand.Float64()*0.05)*cool)
		}
	}
	for i, h := range e.heat {
		f.Px[i] = heatmap.At(h)
	}
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
			var c RGB
			if field > 1 {
				c = lerp(mix, th.Bright, math.Min(0.45, (field-1)*0.08)+a.Beat*0.2)
			} else {
				c = lerp(th.Background, mix, math.Min(0.4, field*0.3))
			}
			f.Px[y*f.W+x] = c
		}
	}
}
