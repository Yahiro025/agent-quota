package text

import (
	"crypto/sha256"
	"image"
	"image/color"
	"math"
	"testing"
	"unicode/utf8"
)

func TestBundledFontShapesPunctuation(t *testing.T) {
	style := Style{Size: 18, Weight: 400}
	var hashes [3][32]byte
	for i, mark := range []string{"—", "·", "…"} {
		if w := Measure(mark, style); w <= 0 {
			t.Errorf("Measure(%q) = %v, want a positive advance", mark, w)
		}
		img := image.NewRGBA(image.Rect(0, 0, 48, 32))
		Draw(img, 1, 1, mark, style, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		ink := false
		for i := 3; i < len(img.Pix); i += 4 {
			if img.Pix[i] != 0 {
				ink = true
				break
			}
		}
		if !ink {
			t.Errorf("Draw(%q) produced no glyph pixels", mark)
		}
		hashes[i] = sha256.Sum256(img.Pix)
	}
	if hashes[0] == hashes[1] || hashes[0] == hashes[2] || hashes[1] == hashes[2] {
		t.Fatal("em dash, middle dot, and ellipsis should draw as distinct glyphs")
	}
}

func TestMeasureUsesProportionalAdvances(t *testing.T) {
	style := Style{Size: 14, Weight: 400}
	if narrow, wide := Measure("iii", style), Measure("WWW", style); narrow >= wide {
		t.Fatalf("Inter should measure narrow and wide letters proportionally: iii=%v, WWW=%v", narrow, wide)
	}
}

func TestGlyphInkStaysWithinLineAtFractionalBaselines(t *testing.T) {
	style := Style{Size: 16, Weight: 600}
	img := image.NewRGBA(image.Rect(0, 0, 112, 24))
	for i, y := range []float32{0, 0.25, 0.5, 0.75} {
		Draw(img, float32(i*27)+0.25, y, "H", style, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	}
	for i, y := range []float32{0, 0.25, 0.5, 0.75} {
		x0 := i * 27
		minY, maxY := img.Bounds().Max.Y, 0
		for py := 0; py < img.Bounds().Max.Y; py++ {
			for px := x0; px < x0+24; px++ {
				if img.RGBAAt(px, py).A != 0 {
					minY = min(minY, py)
					maxY = max(maxY, py+1)
				}
			}
		}
		lineTop := int(math.Floor(float64(y)))
		lineBottom := int(math.Ceil(float64(y + style.Size*1.2)))
		if minY == img.Bounds().Max.Y {
			t.Fatalf("H at y=%v produced no ink", y)
		}
		if minY < lineTop || maxY > lineBottom {
			t.Errorf("H at y=%v has ink bounds [%d,%d), want within line [%d,%d)", y, minY, maxY, lineTop, lineBottom)
		}
	}
}

func TestElideKeepsUTF8AndFits(t *testing.T) {
	style := Style{Size: 14, Weight: 500}
	source := "café résumé — quota"
	limit := Measure("café…", style)
	got := Elide(source, style, limit)
	if !utf8.ValidString(got) {
		t.Fatalf("Elide returned invalid UTF-8: %q", got)
	}
	if got == source || got == "" || got[len(got)-len("…"):] != "…" {
		t.Fatalf("Elide(%q) = %q, want a shortened string ending in an ellipsis", source, got)
	}
	if width := Measure(got, style); width > limit {
		t.Fatalf("Elide width = %v, limit = %v", width, limit)
	}
}
