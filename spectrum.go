package main

import "math"

// cliamp's spectrum.get serves DefaultSpectrumBands (10) log-spaced bands.
// A braille cell is 2 dots wide, so a 60-column panel wants 120 values:
// resample interpolates between band centres with Catmull-Rom so the curve
// stays smooth instead of stepping in ten flat plateaus.
func resample(bands []float64, n int) []float64 {
	out := make([]float64, n)
	if len(bands) == 0 || n <= 0 {
		return out
	}
	if len(bands) == 1 {
		for i := range out {
			out[i] = clamp01(bands[0])
		}
		return out
	}
	at := func(i int) float64 { return bands[max(0, min(len(bands)-1, i))] }
	for i := range out {
		pos := 0.0
		if n > 1 {
			pos = float64(i) / float64(n-1) * float64(len(bands)-1)
		}
		k := int(math.Floor(pos))
		t := pos - float64(k)
		p0, p1, p2, p3 := at(k-1), at(k), at(k+1), at(k+2)
		v := 0.5 * (2*p1 + (-p0+p2)*t + (2*p0-5*p1+4*p2-p3)*t*t + (-p0+3*p1-3*p2+p3)*t*t*t)
		out[i] = clamp01(v)
	}
	return out
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// Braille dot bits, bottom row first, for the left and right dot columns.
var (
	brailleLeft  = [4]rune{0x40, 0x04, 0x02, 0x01}
	brailleRight = [4]rune{0x80, 0x20, 0x10, 0x08}
)

// brailleCell returns the glyph for one cell of a filled column graph.
// a and b are the heights in dots of the left and right columns measured
// from the bottom of the whole graph; row is this cell's index from the
// bottom. pa and pb are peak heights in dots (0 = none), drawn as one dot.
func brailleCell(a, b, pa, pb, row int) rune {
	var bits rune
	fa, fb := max(0, min(4, a-row*4)), max(0, min(4, b-row*4))
	for i := range fa {
		bits |= brailleLeft[i]
	}
	for i := range fb {
		bits |= brailleRight[i]
	}
	if d := pa - row*4 - 1; pa > a && d >= 0 && d < 4 {
		bits |= brailleLeft[d]
	}
	if d := pb - row*4 - 1; pb > b && d >= 0 && d < 4 {
		bits |= brailleRight[d]
	}
	return 0x2800 + bits
}

// peaks tracks falling peak caps per dot column, Winamp-style.
type peaks struct{ v []float64 }

func (p *peaks) update(vals []float64) []float64 {
	if len(p.v) != len(vals) {
		p.v = make([]float64, len(vals))
	}
	for i, v := range vals {
		p.v[i] = math.Max(p.v[i]-0.012, v)
	}
	return p.v
}
