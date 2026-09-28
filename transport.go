package main

import "os"

// glyphSet is how the transport controls are drawn.
type glyphSet struct{ prev, play, pause, stop, next string }

var (
	// Material Design icons as patched into Nerd Fonts (nf-md-skip_previous,
	// play, pause, stop, skip_next). Omarchy's default terminal font is a
	// Nerd Font; anywhere else they would draw as empty boxes.
	nerdGlyphs  = glyphSet{"\U000F04AE", "\U000F040A", "\U000F03E4", "\U000F04DB", "\U000F04AD"}
	plainGlyphs = glyphSet{"«", "▶", "❚❚", "■", "»"}
	glyphs      = plainGlyphs
)

// pickGlyphs applies the -icons flag: "nerd", "plain", or "auto", which takes
// an Omarchy theme file as the sign of a Nerd Font.
func pickGlyphs(mode, themePath string) {
	glyphs = plainGlyphs
	switch mode {
	case "nerd":
		glyphs = nerdGlyphs
	case "auto":
		if _, err := os.Stat(themePath); err == nil {
			glyphs = nerdGlyphs
		}
	}
}

// button is one transport control. draw and click both come from transport,
// so what is drawn is exactly what can be clicked.
type button struct {
	at     rect // where the glyph is drawn
	glyph  string
	op     string // the cliamp operation a click sends
	label  string // noted on the status line after the click
	colour cls
}

// hit is the clickable area: the glyph plus a column either side.
func (b button) hit() rect { return rect{b.at.x - 1, b.at.y, b.at.w + 2, 1} }

// transport lays out the controls on the player's bottom row. Only the
// control matching the playback state is lit: play green, pause yellow, stop
// red. A stream has nothing to skip through, and a narrow player keeps only
// play, pause and stop.
func (m model) transport(player rect) []button {
	ix, iw, iy, ih := player.x+2, player.w-4, player.y+1, player.h-2
	if iw < 4 || ih < 4 {
		return nil
	}
	st := m.snap.State
	lit := func(on bool, c cls) cls {
		if on {
			return c
		}
		return cDim
	}
	bs := []button{
		{glyph: glyphs.play, op: "play", colour: lit(st == "playing", cGreen)},
		{glyph: glyphs.pause, op: "pause", colour: lit(st == "paused", cYellow)},
		{glyph: glyphs.stop, op: "stop", label: "stopped", colour: lit(st != "playing" && st != "paused", cRed)},
	}
	if !m.live() && iw >= 30 {
		bs = append([]button{{glyph: glyphs.prev, op: "prev", label: "previous", colour: cWhite}}, bs...)
		bs = append(bs, button{glyph: glyphs.next, op: "next", label: "next", colour: cWhite})
	}
	x := ix
	for i := range bs {
		w := len([]rune(bs[i].glyph))
		bs[i].at = rect{x, iy + 3, w, 1}
		x += w + 2
	}
	return bs
}
