package main

import "math"

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// meterRanges is how many frequency ranges the player's meter shows: one per
// braille dot row, over two text rows.
const meterRanges = 8

// meterRangeDB is how far below a range's recent peak its bar reaches empty.
const meterRangeDB = 6

// Peak dots hold at a range's recent high for peakHold spectrum frames, then
// slide back peakFall of the bar per frame until the bar catches them.
const (
	peakHold = 12
	peakFall = 0.03
)

// updateLevels splits the bands into meterRanges contiguous ranges, lows
// first, and takes each range's loudest band in dB against that range's own
// slowly decaying peak, over meterRangeDB. Levels jump up and fall back gently,
// like a meter needle, and each leaves a peak dot behind.
func (m *model) updateLevels(bands []float64) {
	n := len(bands)
	if n == 0 {
		return
	}
	for r := range meterRanges {
		lo, hi := r*n/meterRanges, max(r*n/meterRanges+1, (r+1)*n/meterRanges)
		top := 0.0
		for _, v := range bands[lo:min(n, hi)] {
			top = max(top, v)
		}
		m.levelGain[r] = max(top, m.levelGain[r]*0.999, 0.002)
		v := 0.0
		if top > 0 {
			v = clamp01(1 + 20*math.Log10(top/m.levelGain[r])/meterRangeDB)
		}
		m.levels[r] = max(v, m.levels[r]-0.04)
		switch {
		case m.levels[r] >= m.levelPeak[r]:
			m.levelPeak[r], m.peakWait[r] = m.levels[r], peakHold
		case m.peakWait[r] > 0:
			m.peakWait[r]--
		default:
			m.levelPeak[r] = max(m.levels[r], m.levelPeak[r]-peakFall)
		}
	}
}

// brailleBars renders one row of cells holding up to four horizontal bars, one
// per dot row, top to bottom; each fill is 0..1 across 2*w dot columns. A
// peak (0 for none) adds a lone dot where the bar last reached; peakOnly marks
// cells lit only by such dots, so they can be drawn in their own colour.
func brailleBars(fills, peaks []float64, w int) (cells []rune, peakOnly []bool) {
	rowBits := [4][2]rune{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}
	cells, peakOnly = make([]rune, w), make([]bool, w)
	var bar = make([]bool, w)
	for r, f := range fills {
		dots := int(math.Round(clamp01(f) * float64(w*2)))
		for d := range dots {
			cells[d/2] |= rowBits[r][d%2]
			bar[d/2] = true
		}
		if r < len(peaks) {
			if d := int(math.Round(clamp01(peaks[r])*float64(w*2))) - 1; d >= dots && d >= 0 {
				cells[d/2] |= rowBits[r][d%2]
			}
		}
	}
	for i := range cells {
		peakOnly[i] = cells[i] != 0 && !bar[i]
		cells[i] += 0x2800
	}
	return cells, peakOnly
}
