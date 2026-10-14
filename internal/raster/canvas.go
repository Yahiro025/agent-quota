package raster

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/vector"
)

// Canvas is a small premultiplied RGBA image and its CPU drawing surface.
type Canvas struct {
	Img *image.RGBA
}

func New(w, h int) *Canvas {
	return &Canvas{Img: image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))}
}

func (c *Canvas) Bounds() image.Rectangle { return c.Img.Bounds() }

// Clear replaces the canvas with transparent pixels.
func (c *Canvas) Clear() { clear(c.Img.Pix) }

func palette(col color.RGBA) color.NRGBA {
	// Callers provide ordinary, unassociated palette values. NRGBA tells the
	// standard draw pipeline to premultiply them when composing into Canvas.Img.
	return color.NRGBA{R: col.R, G: col.G, B: col.B, A: col.A}
}

func (c *Canvas) fillMask(mask *image.Alpha, col color.RGBA) {
	if mask == nil || col.A == 0 {
		return
	}
	r := mask.Bounds().Intersect(c.Img.Bounds())
	if r.Empty() {
		return
	}
	draw.DrawMask(c.Img, r, image.NewUniform(palette(col)), image.Point{}, mask, r.Min, draw.Over)
}

func rasterRect(x, y, w, h float64, b image.Rectangle) image.Rectangle {
	return image.Rect(int(math.Floor(x))-1, int(math.Floor(y))-1,
		int(math.Ceil(x+w))+1, int(math.Ceil(y+h))+1).Intersect(b)
}

func drawShape(dst *image.Alpha, bounds image.Rectangle, build func(*vector.Rasterizer, float32, float32)) {
	if bounds.Empty() {
		return
	}
	var ras vector.Rasterizer
	ras.Reset(bounds.Dx(), bounds.Dy())
	build(&ras, -float32(bounds.Min.X), -float32(bounds.Min.Y))
	ras.DrawOp = draw.Src
	ras.Draw(dst, bounds, image.Opaque, image.Point{})
}

// Rect fills an axis-aligned rectangle with antialiased edges.
func (c *Canvas) Rect(x, y, w, h int, col color.RGBA) {
	c.FillRound(x, y, w, h, 0, col)
}

// FillRound fills a rounded rectangle. Radius is clamped to half the short side.
func (c *Canvas) FillRound(x, y, w, h, radius int, col color.RGBA) {
	if w <= 0 || h <= 0 || col.A == 0 {
		return
	}
	r := float32(max(0, min(radius, min(w, h)/2)))
	b := rasterRect(float64(x), float64(y), float64(w), float64(h), c.Bounds())
	mask := image.NewAlpha(b)
	drawShape(mask, b, func(ras *vector.Rasterizer, ox, oy float32) {
		rounded(ras, ox, oy, float32(x), float32(y), float32(w), float32(h), r, false)
	})
	c.fillMask(mask, col)
}

// StrokeRound draws a rounded-rectangle outline inside its bounds.
func (c *Canvas) StrokeRound(x, y, w, h, radius, stroke int, col color.RGBA) {
	if w <= 0 || h <= 0 || stroke <= 0 || col.A == 0 {
		return
	}
	if w <= 2*stroke || h <= 2*stroke {
		c.FillRound(x, y, w, h, radius, col)
		return
	}
	outerR := float32(max(0, min(radius, min(w, h)/2)))
	innerR := max(outerR-float32(stroke), 0)
	b := rasterRect(float64(x), float64(y), float64(w), float64(h), c.Bounds())
	mask := image.NewAlpha(b)
	drawShape(mask, b, func(ras *vector.Rasterizer, ox, oy float32) {
		rounded(ras, ox, oy, float32(x), float32(y), float32(w), float32(h), outerR, false)
		rounded(ras, ox, oy, float32(x+stroke), float32(y+stroke), float32(w-2*stroke), float32(h-2*stroke), innerR, true)
	})
	c.fillMask(mask, col)
}

// rounded appends a closed rounded rectangle using cubic quarter-circle curves.
func rounded(r *vector.Rasterizer, ox, oy, x, y, w, h, radius float32, reverse bool) {
	const k = 0.447715 // 1 - the cubic approximation for a quarter circle
	x, y = x+ox, y+oy
	radius = min(radius, min(w/2, h/2))
	if radius <= 0 {
		if !reverse {
			r.MoveTo(x, y)
			r.LineTo(x+w, y)
			r.LineTo(x+w, y+h)
			r.LineTo(x, y+h)
		} else {
			r.MoveTo(x, y)
			r.LineTo(x, y+h)
			r.LineTo(x+w, y+h)
			r.LineTo(x+w, y)
		}
		r.ClosePath()
		return
	}
	if !reverse {
		r.MoveTo(x+radius, y)
		r.LineTo(x+w-radius, y)
		r.CubeTo(x+w-radius*k, y, x+w, y+radius*k, x+w, y+radius)
		r.LineTo(x+w, y+h-radius)
		r.CubeTo(x+w, y+h-radius*k, x+w-radius*k, y+h, x+w-radius, y+h)
		r.LineTo(x+radius, y+h)
		r.CubeTo(x+radius*k, y+h, x, y+h-radius*k, x, y+h-radius)
		r.LineTo(x, y+radius)
		r.CubeTo(x, y+radius*k, x+radius*k, y, x+radius, y)
	} else {
		r.MoveTo(x+radius, y)
		r.CubeTo(x+radius*k, y, x, y+radius*k, x, y+radius)
		r.LineTo(x, y+h-radius)
		r.CubeTo(x, y+h-radius*k, x+radius*k, y+h, x+radius, y+h)
		r.LineTo(x+w-radius, y+h)
		r.CubeTo(x+w-radius*k, y+h, x+w, y+h-radius*k, x+w, y+h-radius)
		r.LineTo(x+w, y+radius)
		r.CubeTo(x+w, y+radius*k, x+w-radius*k, y, x+w-radius, y)
	}
	r.ClosePath()
}

// Ring draws a smooth quota track and its clockwise usage arc from 12 o'clock.
func (c *Canvas) Ring(cx, cy, radius, thickness int, frac float64, fg, track color.RGBA) {
	c.RingF(float64(cx), float64(cy), float64(radius), float64(thickness), frac, fg, track)
}

// RingF is Ring with fractional pixel geometry for scaled layouts.
func (c *Canvas) RingF(cx, cy, radius, thickness, frac float64, fg, track color.RGBA) {
	if radius <= 0 || thickness <= 0 {
		return
	}
	if track.A != 0 {
		c.ringShape(cx, cy, radius, thickness, 1, track)
	}
	if frac > 0 && fg.A != 0 {
		c.ringShape(cx, cy, radius, thickness, min(frac, 1), fg)
	}
}

func (c *Canvas) ringShape(cx, cy, radius, thickness, frac float64, col color.RGBA) {
	half := thickness / 2
	pad := half + 1
	b := rasterRect(cx-radius-pad, cy-radius-pad, 2*(radius+pad), 2*(radius+pad), c.Bounds())
	mask := image.NewAlpha(b)
	steps := max(24, int(math.Ceil(2*math.Pi*radius*2*frac)))
	drawShape(mask, b, func(ras *vector.Rasterizer, ox, oy float32) {
		cx0, cy0, r0, h0 := float32(cx)+ox, float32(cy)+oy, float32(radius), float32(half)
		point := func(r float32, i int, fromTop bool) (float32, float32) {
			f := frac * float64(i) / float64(steps)
			a := -math.Pi/2 + f*2*math.Pi
			if !fromTop {
				a = -math.Pi/2 + frac*2*math.Pi - f*2*math.Pi
			}
			return cx0 + r*float32(math.Cos(a)), cy0 + r*float32(math.Sin(a))
		}
		for i := 0; i <= steps; i++ {
			x, y := point(r0+h0, i, true)
			if i == 0 {
				ras.MoveTo(x, y)
			} else {
				ras.LineTo(x, y)
			}
		}
		for i := 0; i <= steps; i++ {
			x, y := point(r0-h0, i, false)
			ras.LineTo(x, y)
		}
		ras.ClosePath()
		if frac < 1 {
			end := -math.Pi/2 + frac*2*math.Pi
			addDisc(ras, cx0+r0*float32(math.Cos(end)), cy0+r0*float32(math.Sin(end)), h0)
			addDisc(ras, cx0, cy0-r0, h0)
		}
	})
	c.fillMask(mask, col)
}

func addDisc(r *vector.Rasterizer, cx, cy, radius float32) {
	n := max(12, int(radius*8))
	for i := 0; i < n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		x, y := cx+radius*float32(math.Cos(a)), cy+radius*float32(math.Sin(a))
		if i == 0 {
			r.MoveTo(x, y)
		} else {
			r.LineTo(x, y)
		}
	}
	r.ClosePath()
}

// Blit draws a premultiplied RGBA image over the canvas.
func (c *Canvas) Blit(x, y int, src *image.RGBA) {
	b := src.Bounds()
	dr := image.Rect(x, y, x+b.Dx(), y+b.Dy()).Intersect(c.Bounds())
	if dr.Empty() {
		return
	}
	draw.Draw(c.Img, dr, src, b.Min.Add(dr.Min.Sub(image.Pt(x, y))), draw.Over)
}
