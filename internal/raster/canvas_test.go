package raster

import (
	"image/color"
	"testing"
)

func TestRoundedFillIsAntialiasedAndPremultiplied(t *testing.T) {
	c := New(12, 12)
	c.FillRound(1, 1, 10, 10, 4, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	center := c.Img.RGBAAt(6, 6)
	if center != (color.RGBA{R: 200, G: 100, B: 50, A: 255}) {
		t.Fatalf("center pixel = %#v", center)
	}
	if got := c.Img.RGBAAt(1, 1).A; got != 0 {
		t.Fatalf("rounded corner alpha = %d, want transparent", got)
	}
	partial := false
	for i := 0; i < len(c.Img.Pix); i += 4 {
		r, g, b, a := c.Img.Pix[i], c.Img.Pix[i+1], c.Img.Pix[i+2], c.Img.Pix[i+3]
		if r > a || g > a || b > a {
			t.Fatalf("pixel is not premultiplied: RGBA(%d,%d,%d,%d)", r, g, b, a)
		}
		partial = partial || a > 0 && a < 255
	}
	if !partial {
		t.Fatal("rounded edge has no fractional coverage")
	}
}

func TestOffscreenRoundedFillClipsToCanvas(t *testing.T) {
	c := New(8, 8)
	c.FillRound(-3, -3, 10, 10, 3, color.RGBA{R: 240, G: 120, B: 30, A: 255})
	if got := c.Img.RGBAAt(2, 2).A; got == 0 {
		t.Fatal("visible part of offscreen shape was clipped away")
	}
	if got := c.Img.RGBAAt(7, 7).A; got != 0 {
		t.Fatalf("pixel outside shape alpha = %d", got)
	}
}

func TestEvenOddSVGPathLeavesItsHoleTransparent(t *testing.T) {
	c := New(24, 24)
	c.FillPath("M 0 0 H 24 V 24 H 0 Z M 8 8 H 16 V 16 H 8 Z", [4]float32{0, 0, 24, 24}, 0, 0, 24, color.RGBA{R: 255, A: 255}, true)
	if got := c.Img.RGBAAt(4, 4).A; got == 0 {
		t.Fatal("outer path was not filled")
	}
	if got := c.Img.RGBAAt(12, 12).A; got != 0 {
		t.Fatalf("hole alpha = %d, want 0", got)
	}
}

func TestRingFillsArcAndTrackWithSmoothEdges(t *testing.T) {
	c := New(24, 24)
	c.RingF(12, 12, 8, 2, .25, color.RGBA{R: 50, G: 200, B: 80, A: 255}, color.RGBA{R: 255, G: 255, B: 255, A: 51})
	if got := c.Img.RGBAAt(12, 4).G; got < 150 {
		t.Fatalf("start of usage arc green = %d", got)
	}
	if got := c.Img.RGBAAt(20, 12).A; got == 0 {
		t.Fatal("unused track was not drawn")
	}
	partial := false
	for i := 3; i < len(c.Img.Pix); i += 4 {
		partial = partial || c.Img.Pix[i] > 0 && c.Img.Pix[i] < 255
	}
	if !partial {
		t.Fatal("ring has no fractional coverage at its edges")
	}
}
