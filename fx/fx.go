// Package fx renders audio-reactive demo effects into a pixel frame. A
// terminal cell holds two pixels (a ▀ half-block with separate foreground and
// background colours), so the frame is W×2H for a W×H cell area. Effects take
// their colours from a Theme, which is read from the active Omarchy theme.
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

// Audio is what every effect reacts to. Levels are 0..1; Beat jumps to 1 on
// a detected onset and decays over ~300 ms.
type Audio struct {
	Bass, Mid, Treble, Beat float64
}

// Effect draws one frame. dt is the seconds since the previous frame; t is a
// monotonic clock. Effects may keep state between frames.
type Effect interface {
	Name() string
	Render(f *Frame, a Audio, t, dt float64, th *Theme)
}

// Stock is the built-in set, in `v` cycle order.
func Stock() []Effect {
	return []Effect{&Plasma{}, &Tunnel{}, &Fire{}, &Metaballs{}}
}

// Analyzer turns cliamp's ten log-spaced bands into the Audio signals.
type Analyzer struct {
	Audio
	avgBass float64
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
	bass := avg(0, 3)
	an.Mid, an.Treble = avg(3, 7), avg(7, 10)
	an.avgBass = an.avgBass*0.97 + bass*0.03
	if bass > an.avgBass*1.35 && bass-an.Bass > 0.04 && an.Beat < 0.4 {
		an.Beat = 1
	} else {
		an.Beat = math.Max(0, an.Beat-dt*3.2)
	}
	an.Bass = bass
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
