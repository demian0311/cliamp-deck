package fx

import "math"

// Night scenes. Skyline is drawn like the voxel games of the early 90s:
// each column is marched from the camera outward, and a slice of the world
// is painted only where it rises above everything nearer.

// sceneSky fills the frame with a sky that fades from the stage at the top
// toward haze at the horizon row, and returns the haze.
func sceneSky(f *Frame, th *Theme, hor, flash float64) RGB {
	haze := th.Glow(lerp(th.Stage, th.Color("magenta"), 0.4), flash)
	for y := range f.H {
		k := clamp01(float64(y) / hor)
		c := lerp(th.Stage, haze, k*k)
		for x := range f.W {
			f.Px[y*f.W+x] = c
		}
	}
	return haze
}

// Aurora: bands of curtain light over three mountain ranges. One to four
// bands hang in the sky, more as the music gets louder; each stretch of a
// band burns as bright as its part of the spectrum, bass on the left. The
// ranges drift past at different speeds, the far one slowest, so the land
// has depth. Stars twinkle through dim and bright, each following one band
// and cycling faster with the treble. Bass lifts the curtains and quickens
// the drift, mids set their sway, and a beat flares the whole sky.
type Aurora struct {
	pan, bands, twinkle float64
	sway, level, swell  float64   // curtain phase; eased loudness and beat
	glow                []float64 // eased band level under each column
}

func (*Aurora) Name() string { return "aurora" }

// auroraDrift is how fast the nearest range slides past, in pixels a second
// at rest; the others move at auroraDepth of it.
const auroraDrift = 3.0

var auroraDepth = [3]float64{0.2, 0.5, 1}

const auroraMaxBands = 4

func (e *Aurora) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	W, H := float64(f.W), float64(f.H)
	energy := (a.Bass + a.Mid + a.Treble) / 3
	if e.bands == 0 {
		e.bands = 1
	}
	e.bands += (1 + (auroraMaxBands-1)*energy - e.bands) * math.Min(1, dt*1.2) // eased, so bands fade in and out
	e.pan += dt * auroraDrift * (1 + 1.5*a.Bass)
	e.twinkle += dt * (0.8 + 3*a.Treble)
	// The curtains ease toward the music rather than jumping with it: a
	// quarter second for the bands, a little longer for loudness, and a beat
	// swells and fades over about half a second. Sway is a phase that
	// accumulates, so a change in the mids changes its speed, not its place.
	ease := func(v *float64, to, rate float64) { *v += (to - *v) * math.Min(1, dt*rate) }
	if len(e.glow) != f.W {
		e.glow = make([]float64, f.W)
	}
	for x := range e.glow {
		ease(&e.glow[x], bandAt(a, float64(x)/W), 4)
	}
	ease(&e.level, energy, 3)
	e.swell = math.Max(e.swell*math.Exp(-dt*2), a.Beat)
	e.sway += dt * (0.3 + 0.6*a.Mid)
	low := lerp(th.Stage, th.Color("blue"), 0.15)
	for y := range f.H {
		c := lerp(th.Stage, low, float64(y)/H)
		for x := range f.W {
			f.Px[y*f.W+x] = c
			if !th.Light && hash2(int64(x), int64(y)) > 0.985 { // stars, on a dark sky only
				X, Y := int64(x), int64(y)
				cycle := 0.5 + 0.5*math.Sin(e.twinkle*(0.6+hash2(Y, X))+hash2(X+7, Y)*2*math.Pi)
				f.Px[y*f.W+x] = lerp(c, th.Bright, 0.05+0.85*cycle*(0.3+0.7*bandAt(a, hash2(X, Y+3))))
			}
		}
	}

	curtain := th.Gradient("green", "cyan", "magenta")
	for k := range auroraMaxBands {
		w := clamp01(e.bands - float64(k))
		if w == 0 {
			continue
		}
		kf := float64(k)
		for x := range f.W {
			fx := float64(x)
			yc := H*(0.2+0.11*kf) + math.Sin(fx*0.07*(1+0.3*kf)+e.sway*0.6+kf*1.7)*H*0.07 + math.Sin(fx*0.19-e.sway+kf)*H*0.03
			patch := 0.3 + 0.7*fbm(fx*0.03+kf*13+t*0.05, kf*5) // each band thins and thickens along its length
			in := w * patch * ((0.3+0.7*fbm(fx*0.04+t*0.08, t*0.05+kf*3))*(0.15+0.85*e.glow[x])*(0.5+0.5*e.level) + 0.3*e.swell)
			span := H * (0.07 + 0.1*in + 0.06*e.level) // short, so stacked bands stay apart
			for y := max(0, int(yc-span)); y < min(f.H, int(yc+3)); y++ {
				up := (yc - float64(y)) / span
				v := clamp01(1 - up)
				if float64(y) > yc {
					v = clamp01(1 - (float64(y)-yc)/3)
				}
				v *= in * (0.6 + 0.4*vnoise(fx*0.5, t*0.6+float64(y)*0.1))
				i := y*f.W + x
				f.Px[i] = lerp(f.Px[i], curtain.At(clamp01(up*0.6+kf*0.25)), clamp01(v*1.8))
			}
		}
	}

	blue := th.Color("blue")
	shade := [3]float64{0.3, 0.14, 0} // haze lightens the far ranges on a dark sky
	if th.Light {
		shade = [3]float64{0.2, 0.38, 0.55} // on paper the near range carries the most ink
	}
	for r, depth := range auroraDepth {
		c := lerp(th.Stage, blue, shade[r])
		base, rise := H*(0.8+0.07*float64(r)), H*(0.2-0.04*float64(r)) // low, so the sky has the frame
		for x := range f.W {
			wx := (float64(x) + e.pan*depth) * (0.018 + 0.01*float64(r))
			n := 1 - math.Abs(2*fbm(wx, float64(r)*7.3)-1) // ridged, so the ranges peak
			ridge := max(0, int(base-rise*n))
			for y := ridge; y < f.H; y++ {
				f.Px[y*f.W+x] = c
			}
		}
	}
}

// Skyline: flying low over a city at night. Every building listens to one
// band, picked at random so the whole city answers the music at once: the
// louder its band, the more of its windows are lit, so the lights sweep
// through the blocks as the song moves. Bass sets the air speed, and a beat
// flares every lit window. The city is drawn at skylineSS times the pane in
// each direction and averaged down, which softens the stair-stepped edges a
// column march leaves on rooftops and far blocks.
type Skyline struct {
	z     float64
	top   []int
	lastH []float64
	fine  Frame
}

func (*Skyline) Name() string { return "skyline" }

const (
	skylineFar    = 28.0 // world units to the last block drawn
	skylineCam    = 4.0  // camera height, well above the tallest towers (2.5), so the view looks down on the city
	skylineStreet = 0.22 // share of each block that is street
	skylineFloors = 7.0  // floors per world unit of height
	skylineDetail = 14.0 // windows are drawn one by one nearer than this
	skylineSS     = 2    // supersampling factor per axis
)

// skylineHeight is the building standing at world (wx, wz), with the block's
// cell for hashing, or 0 in the street.
func skylineHeight(wx, wz float64) (h float64, cx, cz int64) {
	cx, cz = int64(math.Floor(wx)), int64(math.Floor(wz))
	if frac(wx) < skylineStreet || frac(wz) < skylineStreet {
		return 0, cx, cz
	}
	r := hash2(cx, cz)
	return 0.3 + 2.2*r*r*r, cx, cz // cubed: mostly low blocks, a few towers
}

func (e *Skyline) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	e.fine.Resize(f.W*skylineSS, f.H*skylineSS)
	e.draw(&e.fine, a, dt, th)
	for y := range f.H {
		for x := range f.W {
			var r, g, b int
			for sy := range skylineSS {
				for sx := range skylineSS {
					p := e.fine.Px[(y*skylineSS+sy)*e.fine.W+x*skylineSS+sx]
					r, g, b = r+int(p.R), g+int(p.G), b+int(p.B)
				}
			}
			n := skylineSS * skylineSS
			f.Px[y*f.W+x] = RGB{uint8(r / n), uint8(g / n), uint8(b / n)}
		}
	}
}

func (e *Skyline) draw(f *Frame, a Audio, dt float64, th *Theme) {
	if len(e.top) != f.W {
		e.top, e.lastH = make([]int, f.W), make([]float64, f.W)
	}
	W, H := float64(f.W), float64(f.H)
	e.z += dt * (0.5 + 1.2*a.Bass)
	hor := H * 0.12
	scale := H * 0.45
	haze := sceneSky(f, th, hor, 0)

	blue := th.Color("blue")
	face, roof, street := lerp(th.Stage, blue, 0.18), lerp(th.Stage, blue, 0.08), lerp(th.Stage, blue, 0.04)
	if th.Light {
		face, roof, street = lerp(th.Stage, blue, 0.45), lerp(th.Stage, blue, 0.3), lerp(th.Stage, blue, 0.1)
	}
	dark := lerp(face, th.Stage, 0.5)
	warm := th.Gradient("yellow", "orange")
	lamp := th.Color("yellow")

	for x := range e.top {
		e.top[x], e.lastH[x] = f.H, 0
	}
	for d := 1.5; d < skylineFar; d += 0.06 + d*0.02 {
		fog := d / skylineFar
		wz := e.z + d
		for x := range f.W {
			wx := (float64(x) - W/2) / H * d * 0.9
			h, cx, cz := skylineHeight(wx, wz)
			facing := h > e.lastH[x] // the ray just met a wall, not a roof
			e.lastH[x] = h
			y := max(0, int(hor+(skylineCam-h)/d*scale))
			if y >= e.top[x] {
				continue
			}
			level := bandAt(a, hash2(cx, cz+7919))
			for yy := y; yy < e.top[x]; yy++ {
				var c RGB
				switch {
				case h == 0:
					c = street
					if frac(wz*2) < 0.12 && (frac(wx) < 0.03 || math.Abs(frac(wx)-skylineStreet) < 0.03) {
						c = lamp
					}
				case !facing:
					c = roof
				case d > skylineDetail:
					c = lerp(face, warm.At(0.3), 0.5*level) // too far for single windows: a glow
				default:
					wh := skylineCam - (float64(yy)-hor)*d/scale // world height of this pixel
					fl := wh * skylineFloors
					col := (wx + wz) * 5
					c = face
					if frac(fl) > 0.35 && frac(col) > 0.35 {
						id := hash2(cx*131+cz, int64(math.Floor(fl))*977+int64(math.Floor(col)))
						c = dark
						if id < 0.1+0.9*level {
							c = th.Glow(warm.At(id), a.Beat*0.6)
						}
					}
				}
				f.Px[yy*f.W+x] = lerp(c, haze, fog*0.8)
			}
			e.top[x] = y
		}
	}
}
