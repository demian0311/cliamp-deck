package fx

import "math"

// Landscapes drawn like the voxel games of the early 90s: each column is
// marched from the camera outward, and a slice of the world is painted only
// where it rises above everything nearer, so near ground hides far ground.

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

// Terrain, after Comanche (1992): flying high over mountains toward a low
// sun, looking down on the ridges ahead. The land under each column follows
// one part of the spectrum, bass on the left, so the range rises and falls
// with the music; bass sets the air speed, and a beat flares the sky and
// lights the ridge lines.
type Terrain struct {
	z   float64
	top []int
}

func (*Terrain) Name() string { return "terrain" }

const (
	terrainFar  = 40.0 // world units to the far edge of the land
	terrainCam  = 3.2  // camera height; ridges peak near 2.6, so the view looks down on them
	terrainPeak = 2.2  // world height of a ridge at full level
)

func (e *Terrain) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	if len(e.top) != f.W {
		e.top = make([]int, f.W)
	}
	W, H := float64(f.W), float64(f.H)
	e.z += dt * (1.2 + 3.5*a.Bass)
	hor := H * 0.2
	haze := sceneSky(f, th, hor, a.Beat*0.4)

	sun := th.Gradient("yellow", "orange", "red")
	sx, sy, sr := W*0.7, hor, H*0.13
	for y := max(0, int(sy-sr)); y < int(sy) && y < f.H; y++ {
		for x := max(0, int(sx-sr)); x < int(sx+sr) && x < f.W; x++ {
			dy := float64(y) - sy + sr
			if math.Hypot(float64(x)-sx, float64(y)-sy) < sr && !(dy > sr*0.5 && y%3 == 0) { // the lower half is slatted
				f.Px[y*f.W+x] = sun.At(dy / sr)
			}
		}
	}

	land := th.Heat("blue", "cyan", "green")
	grid := th.Color("magenta")
	far := haze
	if th.Light {
		far = th.Stage // on paper, blue land fogged toward pink haze turns the grey of the text
	}
	for x := range e.top {
		e.top[x] = f.H
	}
	for d := 1.0; d < terrainFar; d += 0.12 + d*0.025 {
		fog := d / terrainFar
		wz := e.z + d
		row := frac(wz) < 0.1 && d < 16 // lines across the ground scroll toward you
		for x := range f.W {
			u := float64(x) / W
			wx := (float64(x) - W/2) / H * d * 0.9
			hg := math.Pow(fbm(wx*0.3, wz*0.3), 1.5) * (0.4 + 1.6*bandAt(a, u))
			y := max(0, int(hor+(terrainCam-hg*terrainPeak)/d*H*0.45))
			if y >= e.top[x] {
				continue
			}
			c := land.At(math.Min(0.6, 0.15+hg*0.35)) // short of the top stop, which is ink on paper
			if row {
				c = lerp(c, grid, 0.3*(1-fog))
			}
			c = lerp(c, far, fog*0.85)
			for yy := y; yy < e.top[x]; yy++ {
				f.Px[yy*f.W+x] = c
			}
			if k := (0.25 + 0.6*a.Beat) * (1 - fog); th.Light {
				f.Px[y*f.W+x] = lerp(c, th.Stage, k*0.7) // a pale edge: deepened ridges on paper read as soot
			} else {
				f.Px[y*f.W+x] = th.Glow(c, k)
			}
			e.top[x] = y
		}
	}
}

// Aurora: curtains of light over three mountain ranges. The ranges drift
// past at different speeds, the far one slowest, so the land has depth; the
// curtains hang in the sky behind them. Each stretch of curtain burns as
// bright as its band, bass on the left; bass lifts the curtains and quickens
// the drift, mids set their sway, and a beat flares the whole sky and catches
// the snow on the peaks.
type Aurora struct{ pan float64 }

func (*Aurora) Name() string { return "aurora" }

// auroraDrift is how fast the nearest range slides past, in pixels a second
// at rest; the others move at auroraDepth of it.
const auroraDrift = 3.0

var auroraDepth = [3]float64{0.2, 0.5, 1}

func (e *Aurora) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	W, H := float64(f.W), float64(f.H)
	e.pan += dt * auroraDrift * (1 + 1.5*a.Bass)
	low := lerp(th.Stage, th.Color("blue"), 0.15)
	for y := range f.H {
		c := lerp(th.Stage, low, float64(y)/H)
		for x := range f.W {
			f.Px[y*f.W+x] = c
			if !th.Light && hash2(int64(x), int64(y)) > 0.985 { // stars, on a dark sky only
				f.Px[y*f.W+x] = lerp(c, th.Bright, 0.3+0.4*math.Abs(math.Sin(t*1.5+float64(x))))
			}
		}
	}

	curtain := th.Gradient("green", "cyan", "magenta")
	glow := make([]float64, f.W) // how lit each column's curtain is, for the snow
	for x := range f.W {
		fx := float64(x)
		yc := H*0.42 + math.Sin(fx*0.07+t*0.5)*H*0.08 + math.Sin(fx*0.19-t*(0.8+a.Mid))*H*0.04
		in := (0.2+0.8*fbm(fx*0.04+t*0.25, t*0.15))*(0.25+0.75*bandAt(a, fx/W)) + 0.35*a.Beat
		glow[x] = clamp01(in)
		span := H * (0.18 + 0.22*in + 0.15*a.Bass)
		for y := max(0, int(yc-span)); y < min(f.H, int(yc+3)); y++ {
			up := (yc - float64(y)) / span
			k := clamp01(1 - up)
			if float64(y) > yc {
				k = clamp01(1 - (float64(y)-yc)/3)
			}
			k *= in * (0.6 + 0.4*vnoise(fx*0.5, t*3+float64(y)*0.1))
			i := y*f.W + x
			f.Px[i] = lerp(f.Px[i], curtain.At(up), clamp01(k*1.3))
		}
	}

	blue := th.Color("blue")
	shade := [3]float64{0.3, 0.14, 0} // haze lightens the far ranges on a dark sky
	if th.Light {
		shade = [3]float64{0.2, 0.38, 0.55} // on paper the near range carries the most ink
	}
	snow := th.Color("green")
	for r, depth := range auroraDepth {
		c := lerp(th.Stage, blue, shade[r])
		base, rise := H*(0.62+0.1*float64(r)), H*(0.3-0.06*float64(r))
		for x := range f.W {
			wx := (float64(x) + e.pan*depth) * (0.018 + 0.01*float64(r))
			n := 1 - math.Abs(2*fbm(wx, float64(r)*7.3)-1) // ridged, so the ranges peak
			ridge := max(0, int(base-rise*n))
			for y := ridge; y < f.H; y++ {
				f.Px[y*f.W+x] = c
			}
			if ridge < f.H {
				f.Px[ridge*f.W+x] = lerp(c, snow, glow[x]*0.6)
			}
		}
	}
}

// Skyline: flying low over a city at night. Every building listens to one
// band, picked at random so the whole city answers the music at once: the
// louder its band, the more of its windows are lit, so the lights sweep
// through the blocks as the song moves. Bass sets the air speed, and a beat
// flares every lit window.
type Skyline struct {
	z     float64
	top   []int
	lastH []float64
}

func (*Skyline) Name() string { return "skyline" }

const (
	skylineFar    = 28.0 // world units to the last block drawn
	skylineCam    = 2.6  // camera height; the tallest towers reach 2.5
	skylineStreet = 0.22 // share of each block that is street
	skylineFloors = 7.0  // floors per world unit of height
	skylineDetail = 14.0 // windows are drawn one by one nearer than this
)

// skylineHeight is the building standing at world (wx, wz), with the block's
// cell for hashing, or 0 in the street.
func skylineHeight(wx, wz float64) (h float64, cx, cz int64) {
	cx, cz = int64(math.Floor(wx)), int64(math.Floor(wz))
	if frac(wx) < skylineStreet || frac(wz) < skylineStreet {
		return 0, cx, cz
	}
	r := hash2(cx, cz)
	return 0.3 + 2.2*r*r, cx, cz
}

func (e *Skyline) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
	if len(e.top) != f.W {
		e.top, e.lastH = make([]int, f.W), make([]float64, f.W)
	}
	W, H := float64(f.W), float64(f.H)
	e.z += dt * (1.5 + 3*a.Bass)
	hor := H * 0.18
	scale := H * 0.6
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
	for d := 0.8; d < skylineFar; d += 0.06 + d*0.02 {
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
