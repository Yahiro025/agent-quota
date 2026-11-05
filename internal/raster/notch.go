package raster

import (
	"image"
	"image/color"
	"math"

	"golang.org/x/image/vector"
)

// FillNotch joins the full top edge to inset sides with concave shoulders.
// Mirroring the outline lets a dragged dock reveal above without leaving work.
func (c *Canvas) FillNotch(rect image.Rectangle, wing, radius int, above bool, ink color.RGBA) {
	w, h := float32(rect.Dx()), float32(rect.Dy())
	if w <= 0 || h <= 0 {
		return
	}
	a := float32(min(wing, rect.Dx()/4, rect.Dy()/3))
	r := float32(min(radius, (rect.Dx()-2*int(a))/2, (rect.Dy()-int(a))/2))
	mask := image.NewAlpha(rect.Intersect(c.Bounds()))
	drawShape(mask, mask.Bounds(), func(v *vector.Rasterizer, ox, oy float32) {
		const k = .552285
		x := float32(rect.Min.X) + ox
		y := func(n float32) float32 {
			if above {
				n = h - n
			}
			return float32(rect.Min.Y) + oy + n
		}
		v.MoveTo(x, y(0))
		v.LineTo(x+w, y(0))
		v.CubeTo(x+w-a*k, y(0), x+w-a, y(a*(1-k)), x+w-a, y(a))
		v.LineTo(x+w-a, y(h-r))
		v.CubeTo(x+w-a, y(h-r+r*k), x+w-a-r+r*k, y(h), x+w-a-r, y(h))
		v.LineTo(x+a+r, y(h))
		v.CubeTo(x+a+r-r*k, y(h), x+a, y(h-r+r*k), x+a, y(h-r))
		v.LineTo(x+a, y(a))
		v.CubeTo(x+a, y(a*(1-k)), x+a*k, y(0), x, y(0))
		v.ClosePath()
	})
	c.fillMask(mask, ink)
}

// NotchContains excludes the same concave shoulders and rounded corners that
// painting excludes, so transparent pixels do not cover the underlying editor.
func NotchContains(rect image.Rectangle, wing, radius int, above bool, p image.Point) bool {
	if !p.In(rect) {
		return false
	}
	w, h := float64(rect.Dx()), float64(rect.Dy())
	a := float64(min(wing, rect.Dx()/4, rect.Dy()/3))
	r := float64(min(radius, (rect.Dx()-2*int(a))/2, (rect.Dy()-int(a))/2))
	x, y := float64(p.X-rect.Min.X)+.5, float64(p.Y-rect.Min.Y)+.5
	if above {
		y = h - y
	}
	left := a
	if y < a {
		left = math.Sqrt(max(0, a*a-(y-a)*(y-a)))
	}
	if y > h-r {
		left += r - math.Sqrt(max(0, r*r-(y-h+r)*(y-h+r)))
	}
	return x >= left && x < w-left
}
