package raster

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/vector"
)

// ShadowRound keeps elevation inside the same premultiplied CPU surface as the
// widget. A padded window can therefore stay layered and never needs a GPU.
func (c *Canvas) ShadowRound(rect image.Rectangle, radius, blur, offset int) {
	b := rect.Add(image.Pt(0, offset)).Inset(-blur).Intersect(c.Bounds())
	mask := image.NewAlpha(b)
	cx, cy := float64(rect.Min.X+rect.Max.X)/2, float64(rect.Min.Y+rect.Max.Y)/2+float64(offset)
	hw, hh := float64(rect.Dx())/2, float64(rect.Dy())/2
	r, sigma := float64(radius), float64(max(blur, 1))/3
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			qx := math.Abs(float64(x)+.5-cx) - hw + r
			qy := math.Abs(float64(y)+.5-cy) - hh + r
			d := max(math.Hypot(max(qx, 0), max(qy, 0))+min(max(qx, qy), 0)-r, 0)
			mask.SetAlpha(x, y, color.Alpha{A: uint8(math.Round(255 * math.Exp(-d*d/(2*sigma*sigma))))})
		}
	}
	c.fillMask(mask, color.RGBA{A: 65})
}

// Line uses round caps so small control icons remain legible at fractional DPI.
func (c *Canvas) Line(x1, y1, x2, y2, width float64, ink color.RGBA) {
	half := width / 2
	dx, dy := x2-x1, y2-y1
	length := math.Hypot(dx, dy)
	if length == 0 || width <= 0 {
		return
	}
	nx, ny := -dy/length*half, dx/length*half
	b := rasterRect(min(x1, x2)-half, min(y1, y2)-half, math.Abs(dx)+width, math.Abs(dy)+width, c.Bounds())
	mask := image.NewAlpha(b)
	drawShape(mask, b, func(ras *vector.Rasterizer, ox, oy float32) {
		ras.MoveTo(float32(x1+nx)+ox, float32(y1+ny)+oy)
		ras.LineTo(float32(x2+nx)+ox, float32(y2+ny)+oy)
		ras.LineTo(float32(x2-nx)+ox, float32(y2-ny)+oy)
		ras.LineTo(float32(x1-nx)+ox, float32(y1-ny)+oy)
		ras.ClosePath()
		addDisc(ras, float32(x1)+ox, float32(y1)+oy, float32(half))
		addDisc(ras, float32(x2)+ox, float32(y2)+oy, float32(half))
	})
	c.fillMask(mask, ink)
}
