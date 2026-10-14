package marks

import (
	"bytes"
	"embed"
	"image"
	"image/color"
	"image/png"
	"math"
	"sync"

	"agent-quota/internal/raster"
	xdraw "golang.org/x/image/draw"
)

// Logo assets are kept beside this package so screenshots and the widget use the same
// source artwork at every raster scale.
//
//go:embed assets/*.svg assets/*.png
var logoFiles embed.FS

var antigravityPNG struct {
	sync.Once
	image image.Image
}

var (
	Green = color.RGBA{0x32, 0xD7, 0x4B, 0xFF}
	Amber = color.RGBA{0xFF, 0xB3, 0x40, 0xFF}
	Red   = color.RGBA{0xFF, 0x45, 0x3A, 0xFF}
	Grey  = color.RGBA{0x6B, 0x72, 0x80, 0xFF}
	Track = color.RGBA{0xFF, 0xFF, 0xFF, 0x33}
	Panel = color.RGBA{0x18, 0x1B, 0x20, 0xF2}
	Pill  = color.RGBA{0x18, 0x1B, 0x20, 0xE6}
	White = color.RGBA{0xF2, 0xF4, 0xF7, 0xFF}
	Dim   = color.RGBA{0x9A, 0xA0, 0xB2, 0xFF}
)

type logo struct {
	pad   float64
	box   [4]float32
	tile  color.RGBA
	edge  bool
	asset string
}

var logos = map[string]logo{
	"claude":      {pad: .19, box: [4]float32{0, 0, 24, 24}, tile: color.RGBA{0xD9, 0x77, 0x57, 0xFF}},
	"kiro":        {pad: .17, box: [4]float32{1.999, 0, 19.733, 24}, tile: color.RGBA{0x90, 0x46, 0xFF, 0xFF}, asset: "kiro.svg"},
	"codex":       {pad: .10, box: [4]float32{3, 3, 18, 18}, tile: color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}},
	"cursor":      {pad: .20, box: [4]float32{1.474, .001, 21.053, 23.998}, tile: color.RGBA{0x18, 0x18, 0x1C, 0xFF}, edge: true},
	"copilot":     {pad: .16, box: [4]float32{0, 0, 24, 24}, tile: color.RGBA{0x1F, 0x23, 0x28, 0xFF}, asset: "copilot-24.svg"},
	"devin":       {pad: .16, box: [4]float32{70, 50.12, 286.54, 326.04}, tile: color.RGBA{0x00, 0x14, 0x23, 0xFF}, asset: "devin.svg"},
	"antigravity": {pad: .08, box: [4]float32{0, 0, 32, 32}, tile: color.RGBA{0x1F, 0x23, 0x28, 0xFF}},
}

// ForUsed picks the quota ring colour using the original Hover palette.
func ForUsed(used float64) color.RGBA {
	switch {
	case used >= 90:
		return Red
	case used >= 70:
		return Amber
	default:
		return Green
	}
}

// Draw paints the donor-style rounded tile and the agent's vector mark into the box.
// Unknown IDs get a neutral question glyph only when no caller has a known agent ID.
func Draw(c *raster.Canvas, id string, x, y, size int) {
	if c == nil || size <= 0 {
		return
	}
	m, ok := logos[id]
	if !ok {
		m = logo{pad: .2, box: [4]float32{0, 0, 24, 24}, tile: color.RGBA{0x35, 0x39, 0x42, 0xFF}}
	}
	radius := int(math.Round(float64(size) * .3))
	c.FillRound(x, y, size, size, radius, m.tile)
	if m.edge {
		c.StrokeRound(x, y, size, size, radius, 1, color.RGBA{0xFF, 0xFF, 0xFF, 0x2E})
	}
	box := float64(size) * (1 - 2*m.pad)
	k := min(box/float64(m.box[2]), box/float64(m.box[3]))
	glyphW := max(1, int(math.Ceil(float64(m.box[2])*k)))
	glyphH := max(1, int(math.Ceil(float64(m.box[3])*k)))
	gx := x + int(math.Round(float64(size)*m.pad+(box-float64(m.box[2])*k)/2))
	gy := y + int(math.Round(float64(size)*m.pad+(box-float64(m.box[3])*k)/2))

	switch id {
	case "claude":
		c.FillPathRect(markClaude, m.box, gx, gy, glyphW, glyphH, color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}, false)
	case "codex":
		stops := []raster.GradientStop{
			{At: 0, Color: color.RGBA{0xB1, 0xA7, 0xFF, 0xFF}},
			{At: .5, Color: color.RGBA{0x7A, 0x9D, 0xFF, 0xFF}},
			{At: 1, Color: color.RGBA{0x39, 0x41, 0xFF, 0xFF}},
		}
		c.FillPathGradientRect(markCodex, m.box, gx, gy, glyphW, glyphH, stops, false)
		c.FillPathRect(markCodexGlyph, m.box, gx, gy, glyphW, glyphH, color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}, false)
	case "cursor":
		c.FillPathRect(markCursor, m.box, gx, gy, glyphW, glyphH, color.RGBA{0xEC, 0xEC, 0xF0, 0xFF}, true)
	case "kiro":
		// Kiro's path occupies only part of its 24-unit SVG viewBox; retain the
		// original square viewBox and offset it by the donor path bounds.
		k := box / 24
		side := max(1, int(math.Ceil(24*k)))
		kx := x + int(math.Round(float64(size)*m.pad+(box-float64(m.box[2])*k)/2-float64(m.box[0])*k))
		ky := y + int(math.Round(float64(size)*m.pad+(box-float64(m.box[3])*k)/2-float64(m.box[1])*k))
		c.FillSVG(asset("kiro.svg"), kx, ky, side, color.RGBA{0xFF, 0xFF, 0xFF, 0xFF})
	case "copilot":
		c.FillSVGRect(asset("copilot-24.svg"), gx, gy, glyphW, glyphH, color.RGBA{0xFF, 0xFF, 0xFF, 0xFF})
	case "devin":
		c.FillSVGRect(asset("devin.svg"), gx, gy, glyphW, glyphH, color.RGBA{0xFF, 0xFF, 0xFF, 0xFF})
	case "antigravity":
		target := image.Rect(gx, gy, gx+glyphW, gy+glyphH)
		mark := antigravityLogo()
		xdraw.CatmullRom.Scale(c.Img, target, mark, mark.Bounds(), xdraw.Over, nil)
	default:
		// Keep unknown IDs legible without adding text dependencies to the painter.
		c.FillPath("M 5 4 L 19 4 L 19 20 L 5 20 Z", [4]float32{0, 0, 24, 24}, gx, gy, max(glyphW, glyphH), color.RGBA{0xD4, 0xD8, 0xE0, 0xFF}, false)
	}
}

func antigravityLogo() image.Image {
	antigravityPNG.Do(func() {
		b, err := logoFiles.ReadFile("assets/antigravity.png")
		if err != nil {
			panic(err)
		}
		antigravityPNG.image, err = png.Decode(bytes.NewReader(b))
		if err != nil {
			panic(err)
		}
	})
	return antigravityPNG.image
}

func asset(name string) string {
	b, err := logoFiles.ReadFile("assets/" + name)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// Accent is the signature colour used when an agent has no usage percentage.
func Accent(id string) color.RGBA {
	switch id {
	case "claude":
		return color.RGBA{0xD9, 0x77, 0x57, 0xFF}
	case "kiro":
		return color.RGBA{0xB4, 0x8C, 0xFF, 0xFF}
	case "codex":
		return color.RGBA{0x7A, 0x9D, 0xFF, 0xFF}
	case "cursor":
		return color.RGBA{0xEC, 0xEC, 0xF0, 0xFF}
	case "copilot":
		return color.RGBA{0x8C, 0xD8, 0xFF, 0xFF}
	case "devin":
		return color.RGBA{0x7E, 0xC8, 0xB0, 0xFF}
	case "antigravity":
		return color.RGBA{0x55, 0xAA, 0xFA, 0xFF}
	default:
		return Grey
	}
}
