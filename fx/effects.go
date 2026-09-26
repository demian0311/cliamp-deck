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
			c := th.Cycle.At(frac(v/8 + sp*0.05 + a.Bass*0.3))
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
			c := th.Tunnel.At(0.35 + frac(u*0.08)*0.6)
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
	cool := 40 / float64(H)
	for y := range H - 1 {
		for x := range W {
			src := (x + rand.IntN(3) - 1 + W) % W
			e.heat[y*W+x] = math.Max(0, e.heat[(y+1)*W+src]-(0.012+rand.Float64()*0.05)*cool)
		}
	}
	for i, h := range e.heat {
		f.Px[i] = th.Fire.At(h)
	}
}

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
	glow := th.Metaballs.At(0.33)
	for y := range f.H {
		for x := range f.W {
			field := 0.0
			for _, b := range balls {
				dx, dy := float64(x)-b.x, float64(y)-b.y
				field += b.r2 / (dx*dx + dy*dy + 1)
			}
			var c RGB
			if field > 1 {
				c = th.Metaballs.At(0.4 + math.Min(0.5, (field-1)*0.12) + a.Beat*0.25)
			} else {
				c = lerp(th.Background, glow, math.Min(0.45, field*0.35))
			}
			f.Px[y*f.W+x] = c
		}
	}
}
