package text

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// Style selects an Inter size and CSS-style weight (400 regular, 500 medium,
// 600 semibold). A zero size defaults to 12 px; a zero weight defaults to 400.
type Style struct {
	Size   float32
	Weight float32
}

type glyph struct {
	face *font.Face
	id   font.GID
	x, y float32
}

type line struct {
	glyphs []glyph
	width  float32
	asc    float32
}

type shapeKey struct {
	text   string
	size   uint32
	weight uint32
}

type rasterKey struct {
	face *font.Face
	id   font.GID
	size uint32
	subX uint8
	subY uint8
}

type glyphMask struct {
	image *image.Alpha
	left  int
	top   int
}

type engine struct {
	fonts   *fonts
	hb      shaping.HarfbuzzShaper
	segment shaping.Segmenter
	shapes  map[shapeKey][]line
	raster  vector.Rasterizer
	masks   map[rasterKey]*glyphMask
}

var shared = struct {
	sync.Mutex
	e *engine
}{}

const subpx = 32

func fx(v float32) fixed.Int26_6 { return fixed.Int26_6(math.Round(float64(v) * 64)) }
func ff(v fixed.Int26_6) float32 { return float32(v) / 64 }

func getEngine() *engine {
	if shared.e == nil {
		shared.e = &engine{
			fonts:  newFonts(),
			shapes: make(map[shapeKey][]line),
			masks:  make(map[rasterKey]*glyphMask),
		}
	}
	return shared.e
}

func normalize(st Style) Style {
	if st.Size <= 0 || math.IsNaN(float64(st.Size)) || math.IsInf(float64(st.Size), 0) {
		st.Size = 12
	}
	if st.Weight <= 0 || math.IsNaN(float64(st.Weight)) || math.IsInf(float64(st.Weight), 0) {
		st.Weight = 400
	}
	return st
}

// Measure returns the widest shaped line in logical pixels. Inter's bundled
// faces and HarfBuzz shaping keep the result stable across Windows machines.
func Measure(s string, style Style) float32 {
	shared.Lock()
	defer shared.Unlock()
	lines := getEngine().shape(s, normalize(style))
	var width float32
	for _, l := range lines {
		width = max(width, l.width)
	}
	return width
}

// Draw paints shaped text at its top-left point using premultiplied-alpha
// compositing into dst. Newlines advance by 1.2 times the selected font size.
func Draw(dst *image.RGBA, x, y float32, s string, style Style, col color.NRGBA) {
	if dst == nil || s == "" || col.A == 0 {
		return
	}
	shared.Lock()
	defer shared.Unlock()
	e := getEngine()
	st := normalize(style)
	lines := e.shape(s, st)
	lineY := y
	for _, l := range lines {
		base := lineY + l.asc
		for _, g := range l.glyphs {
			gx, gy := x+g.x, base+g.y
			px, py := int(math.Floor(float64(gx))), int(math.Floor(float64(gy)))
			subX := uint8(min(int((gx-float32(px))*4), 3))
			subY := uint8(min(int((gy-float32(py))*4), 3))
			m := e.mask(g.face, g.id, st.Size, subX, subY)
			if m == nil {
				continue
			}
			dr := image.Rect(px+m.left, py-m.top, px+m.left+m.image.Bounds().Dx(), py-m.top+m.image.Bounds().Dy())
			draw.DrawMask(dst, dr, image.NewUniform(col), image.Point{}, m.image, image.Point{}, draw.Over)
		}
		lineY += st.Size * 1.2
	}
}

// Elide shortens s at a UTF-8 rune boundary and appends a shaped ellipsis until
// the result fits maxW. It returns an empty string when even the ellipsis cannot fit.
func Elide(s string, style Style, maxW float32) string {
	if !utf8.ValidString(s) || maxW <= 0 {
		return ""
	}
	if Measure(s, style) <= maxW {
		return s
	}
	const ellipsis = "…"
	if Measure(ellipsis, style) > maxW {
		return ""
	}
	rs := []rune(s)
	lo, hi := 0, len(rs)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		candidate := string(rs[:mid]) + ellipsis
		if Measure(candidate, style) <= maxW {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return string(rs[:lo]) + ellipsis
}

func (e *engine) shape(s string, st Style) []line {
	k := shapeKey{s, math.Float32bits(st.Size), math.Float32bits(st.Weight)}
	if cached, ok := e.shapes[k]; ok {
		return cached
	}
	parts := strings.Split(s, "\n")
	lines := make([]line, 0, len(parts))
	for _, part := range parts {
		lines = append(lines, e.shapeLine(part, st))
	}
	if len(e.shapes) >= 512 {
		clear(e.shapes)
	}
	e.shapes[k] = lines
	return lines
}

type resolver struct {
	f      *fonts
	weight float32
}

func (r *resolver) ResolveFace(c rune) *font.Face { return r.f.face(c, r.weight) }

func (e *engine) shapeLine(s string, st Style) line {
	l := line{asc: st.Size * 0.9}
	if s == "" {
		return l
	}
	rs := []rune(s)
	in := shaping.Input{
		Text: rs, RunStart: 0, RunEnd: len(rs), Direction: di.DirectionLTR,
		Size: fx(st.Size), Language: language.NewLanguage("en"),
	}
	resolver := &resolver{f: e.fonts, weight: st.Weight}
	for _, segment := range e.segment.Split(in, resolver) {
		if segment.Face == nil {
			l.width += float32(segment.RunEnd-segment.RunStart) * st.Size * 0.5
			continue
		}
		o := shapeRun(&e.hb, segment)
		a := ff(o.LineBounds.Ascent)
		if a < 0 {
			a = -a
		}
		l.asc = max(l.asc, a)
		pen := float32(0)
		for _, g := range o.Glyphs {
			l.glyphs = append(l.glyphs, glyph{
				face: o.Face,
				id:   g.GlyphID,
				x:    l.width + pen + ff(g.XOffset),
				y:    -ff(g.YOffset),
			})
			pen += ff(g.XAdvance)
		}
		l.width += ff(o.Advance)
	}
	return l
}

func shapeRun(hb *shaping.HarfbuzzShaper, in shaping.Input) shaping.Output {
	size := in.Size
	in.Size *= subpx
	o := hb.Shape(in)
	o.Size = size
	for i := range o.Glyphs {
		g := &o.Glyphs[i]
		g.Width, g.Height = div(g.Width), div(g.Height)
		g.XBearing, g.YBearing = div(g.XBearing), div(g.YBearing)
		g.Advance, g.XAdvance, g.YAdvance = div(g.Advance), div(g.XAdvance), div(g.YAdvance)
		g.XOffset, g.YOffset = div(g.XOffset), div(g.YOffset)
	}
	o.LineBounds = shaping.Bounds{Ascent: div(o.LineBounds.Ascent), Descent: div(o.LineBounds.Descent), Gap: div(o.LineBounds.Gap)}
	o.GlyphBounds = shaping.Bounds{Ascent: div(o.GlyphBounds.Ascent), Descent: div(o.GlyphBounds.Descent), Gap: div(o.GlyphBounds.Gap)}
	o.RecomputeAdvance()
	return o
}

func div(v fixed.Int26_6) fixed.Int26_6 {
	if v < 0 {
		return -div(-v)
	}
	return (v + subpx/2) / subpx
}

func (e *engine) mask(face *font.Face, id font.GID, size float32, subX, subY uint8) *glyphMask {
	k := rasterKey{face: face, id: id, size: math.Float32bits(size), subX: subX, subY: subY}
	if cached, ok := e.masks[k]; ok {
		return cached
	}
	m := renderGlyph(face, id, size, float32(subX)/4, float32(subY)/4, &e.raster)
	if len(e.masks) >= 2048 {
		clear(e.masks)
	}
	e.masks[k] = m
	return m
}

func renderGlyph(face *font.Face, id font.GID, size, subX, subY float32, ras *vector.Rasterizer) *glyphMask {
	if face == nil {
		return nil
	}
	out, ok := face.GlyphDataOutline(id)
	if !ok || len(out.Segments) == 0 {
		return nil
	}
	scale := size / float32(face.Upem())
	at := func(p opentype.SegmentPoint) (float32, float32) {
		return p.X*scale + subX, -p.Y*scale + subY
	}
	x0, y0 := float32(math.MaxFloat32), float32(math.MaxFloat32)
	x1, y1 := float32(-math.MaxFloat32), float32(-math.MaxFloat32)
	for i := range out.Segments {
		for _, p := range out.Segments[i].ArgsSlice() {
			x, y := at(p)
			x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
		}
	}
	left, top := int(math.Floor(float64(x0)))-1, int(math.Floor(float64(y0)))-1
	w, h := int(math.Ceil(float64(x1)))+1-left, int(math.Ceil(float64(y1)))+1-top
	if w <= 0 || h <= 0 || w > 4096 || h > 4096 {
		return nil
	}
	ras.Reset(w, h)
	open := false
	pt := func(p opentype.SegmentPoint) (float32, float32) {
		x, y := at(p)
		return x - float32(left), y - float32(top)
	}
	for i := range out.Segments {
		sg := &out.Segments[i]
		a := sg.Args
		switch sg.Op {
		case opentype.SegmentOpMoveTo:
			if open {
				ras.ClosePath()
			}
			x, y := pt(a[0])
			ras.MoveTo(x, y)
			open = true
		case opentype.SegmentOpLineTo:
			x, y := pt(a[0])
			ras.LineTo(x, y)
		case opentype.SegmentOpQuadTo:
			bx, by := pt(a[0])
			cx, cy := pt(a[1])
			ras.QuadTo(bx, by, cx, cy)
		case opentype.SegmentOpCubeTo:
			bx, by := pt(a[0])
			cx, cy := pt(a[1])
			dx, dy := pt(a[2])
			ras.CubeTo(bx, by, cx, cy, dx, dy)
		}
	}
	if open {
		ras.ClosePath()
	}
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	ras.DrawOp = draw.Src
	ras.Draw(m, m.Bounds(), image.Opaque, image.Point{})
	return &glyphMask{image: m, left: left, top: -top}
}
