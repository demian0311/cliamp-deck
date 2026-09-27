// Package fx renders audio-reactive demo effects into a pixel frame. A
// terminal cell holds two pixels (a ▀ half-block with separate foreground and
// background colours), so the frame is W×2H for a W×H cell area. Effects take
// their colours from a Theme, which is read from the active Omarchy theme.
// The package imports only the standard library, so it can move into cliamp.
//
// # Adding an effect
//
// One file, one line. Write a type with Name and Render, then append it to
// Stock. The deck's tests render every Stock effect at every layout, and
// TestStockEffectsPaintTheWholeFrame checks it fills the frame, so a new
// effect is covered without new tests.
//
//	// Pulse: concentric rings that ride outward on the bass.
//	type Pulse struct{ r float64 }
//
//	func (*Pulse) Name() string { return "pulse" }
//
//	func (e *Pulse) Render(f *Frame, a Audio, t, dt float64, th *Theme) {
//		rings := th.Gradient("background", "accent", "bright_foreground")
//		e.r += dt * (4 + a.Bass*20)
//		for y := range f.H {
//			for x := range f.W {
//				d := math.Hypot(float64(x-f.W/2), float64(y-f.H/2))
//				f.Px[y*f.W+x] = rings.At(frac((d-e.r)/12) + a.Beat*0.3)
//			}
//		}
//	}
//
// Rules of thumb:
//   - Take every colour from th (Color, Gradient, Background, Bright); never a
//     literal, or the effect ignores the user's theme.
//   - Themes can be light. For empty space use th.Stage, not
//     darker_background; for "brighter" use th.Glow or th.Heat, not a lerp
//     toward Bright, which is near-black text on a light theme. Effects that
//     add light (trails, afterglow) subtract it on paper; see Vortex.
//     TestEffectsReadOnLightAndDarkThemes fails an effect that vanishes or
//     turns to soot there.
//   - Advance motion by dt, not per call: frames arrive at ~30 fps but not
//     evenly. Keep that state on the effect's own struct.
//   - Audio levels are already normalised to the recent range of the music,
//     so 0..1 is the full swing; Beat is 1 on an onset and decays.
//   - Size scratch buffers from f.W and f.H on each call; the stage resizes
//     with the terminal and fullscreen.
package fx

import "math"

type RGB struct{ R, G, B uint8 }

// Frame is a W×H pixel buffer, row-major.
type Frame struct {
	W, H int
	Px   []RGB
}

func (f *Frame) Resize(w, h int) {
	if f.W == w && f.H == h {
		return
	}
	f.W, f.H = w, h
	f.Px = make([]RGB, max(0, w*h))
}

// Audio is what every effect reacts to. Levels are 0..1, normalised to the
// music's recent range; Beat jumps to 1 on a detected onset and decays over
// ~300 ms. Bands holds cliamp's ten log-spaced bands (70 Hz..16 kHz), each
// normalised the same way, for effects that want more than three levels.
type Audio struct {
	Bass, Mid, Treble, Beat float64
	Bands                   []float64
}

// Effect draws one frame. dt is the seconds since the previous frame; t is a
// monotonic clock. Effects may keep state between frames.
type Effect interface {
	Name() string
	Render(f *Frame, a Audio, t, dt float64, th *Theme)
}

// Stock is the built-in set, in `v` cycle order.
func Stock() []Effect {
	return []Effect{&Plasma{}, &Tunnel{}, &Fire{}, &Metaballs{}, &Ridges{}, &Spectrum{}, &Timescope{}, &Vortex{}, &Water{}, &Synaesthesia{}, &Scope{}, &Fountain{}}
}

// Analyzer turns cliamp's ten log-spaced bands into the Audio signals.
//
// cliamp serves eased bands, so absolute levels drift slowly and sit in a
// narrow range that differs per station (measured 2026-09-26 on a radio
// stream: bass 0.18–0.66, moving ~0.01 a frame; treble never above 0.10).
// Each signal is therefore rescaled to its own recent range, and beats come
// from spectral flux measured against its own recent spread rather than a
// fixed jump in level.
type Analyzer struct {
	Audio
	bass, mid, treble follower
	bands             []follower
	prev              []float64
	fluxMean, fluxVar float64
	sinceBeat         float64
}

// follower tracks a signal's recent floor and ceiling. Both jump outward at
// once and relax inward with a time constant of rangeMemory seconds.
type follower struct {
	lo, hi float64
	init   bool
}

const (
	rangeMemory = 4.0  // seconds a loud or quiet extreme is remembered
	minSpan     = 0.05 // below this a signal is treated as steady, not stretched to full scale
	fluxMemory  = 1.5  // seconds of flux history the beat threshold adapts over
	fluxSigma   = 1.5  // a beat is flux this many deviations above its mean
	beatGap     = 0.18 // seconds; no two beats closer than this (~330 bpm)
	beatDecay   = 3.2  // Beat falls from 1 to 0 in 1/beatDecay seconds
)

func (f *follower) norm(v, dt float64) float64 {
	if !f.init {
		f.lo, f.hi, f.init = v, v, true
	}
	k := 1 - math.Exp(-dt/rangeMemory)
	if v < f.lo {
		f.lo = v
	} else {
		f.lo += (v - f.lo) * k
	}
	if v > f.hi {
		f.hi = v
	} else {
		f.hi += (v - f.hi) * k
	}
	return clamp01((v - f.lo) / math.Max(f.hi-f.lo, minSpan))
}

func (an *Analyzer) Update(bands []float64, dt float64) Audio {
	avg := func(lo, hi int) float64 {
		s, n := 0.0, 0
		for i := lo; i < hi && i < len(bands); i++ {
			s += bands[i]
			n++
		}
		if n == 0 {
			return 0
		}
		return s / float64(n)
	}
	an.Bass = an.bass.norm(avg(0, 3), dt)
	an.Mid = an.mid.norm(avg(3, 7), dt)
	an.Treble = an.treble.norm(avg(7, 10), dt)
	if len(an.bands) != len(bands) {
		an.bands = make([]follower, len(bands))
		an.Bands = make([]float64, len(bands))
	}
	for i, v := range bands {
		an.Bands[i] = an.bands[i].norm(v, dt)
	}

	flux := 0.0
	if len(an.prev) == len(bands) {
		for i, v := range bands {
			flux += math.Max(0, v-an.prev[i])
		}
	}
	an.prev = append(an.prev[:0], bands...)
	threshold := an.fluxMean + fluxSigma*math.Sqrt(an.fluxVar) + 0.005
	an.sinceBeat += dt
	if flux > threshold && an.sinceBeat >= beatGap {
		an.Beat, an.sinceBeat = 1, 0
	} else {
		an.Beat = math.Max(0, an.Beat-dt*beatDecay)
	}
	k := 1 - math.Exp(-dt/fluxMemory)
	d := flux - an.fluxMean
	an.fluxMean += d * k
	an.fluxVar += (d*d - an.fluxVar) * k
	return an.Audio
}

func lerp(a, b RGB, t float64) RGB {
	t = clamp01(t)
	return RGB{
		uint8(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		uint8(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		uint8(float64(a.B) + (float64(b.B)-float64(a.B))*t),
	}
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func frac(v float64) float64 { return v - math.Floor(v) }
