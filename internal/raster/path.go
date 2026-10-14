package raster

import (
	"encoding/xml"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/image/vector"
)

// SVGPathPolylines parses an SVG path d attribute into flattened line segments.
// Curves and elliptical arcs use the same outline parser as Hover's CPU painter.
func SVGPathPolylines(d string) (lines [][][2]float32, closed []bool) {
	sub, _ := parsePath(d)
	for _, s := range sub {
		line := make([][2]float32, len(s.pts))
		for i, p := range s.pts {
			line[i] = [2]float32{p.x, p.y}
		}
		lines = append(lines, line)
		closed = append(closed, s.closed)
	}
	return lines, closed
}

// FillPath draws a path in the supplied view box, fitted into a square tile.
// evenOdd selects SVG's even-odd fill rule; the default is non-zero winding.
func (c *Canvas) FillPath(d string, viewBox [4]float32, x, y, size int, col color.RGBA, evenOdd bool) {
	if size <= 0 || col.A == 0 {
		return
	}
	c.FillPathRect(d, viewBox, x, y, size, size, col, evenOdd)
}

// FillPathRect draws a path fitted into a rectangular destination.
func (c *Canvas) FillPathRect(d string, viewBox [4]float32, x, y, w, h int, col color.RGBA, evenOdd bool) {
	if w <= 0 || h <= 0 || col.A == 0 {
		return
	}
	mask := c.pathMask(d, viewBox, x, y, w, h, evenOdd)
	c.fillMask(mask, col)
}

// GradientStop is one horizontal path-fill gradient stop.
type GradientStop struct {
	At    float64
	Color color.RGBA
}

// FillPathGradient draws a path filled by a horizontal gradient across its tile.
func (c *Canvas) FillPathGradient(d string, viewBox [4]float32, x, y, size int, stops []GradientStop, evenOdd bool) {
	if size <= 0 || len(stops) == 0 {
		return
	}
	c.FillPathGradientRect(d, viewBox, x, y, size, size, stops, evenOdd)
}

// FillPathGradientRect draws a path fitted into a rectangle with a horizontal gradient.
func (c *Canvas) FillPathGradientRect(d string, viewBox [4]float32, x, y, w, h int, stops []GradientStop, evenOdd bool) {
	if w <= 0 || h <= 0 || len(stops) == 0 {
		return
	}
	mask := c.pathMask(d, viewBox, x, y, w, h, evenOdd)
	if mask == nil {
		return
	}
	b := mask.Bounds()
	src := image.NewNRGBA(b)
	for py := b.Min.Y; py < b.Max.Y; py++ {
		for px := b.Min.X; px < b.Max.X; px++ {
			coverage := mask.AlphaAt(px, py).A
			if coverage == 0 {
				continue
			}
			col := sampleGradient(stops, float64(px-x)/float64(max(w-1, 1)))
			col.A = uint8((uint16(col.A)*uint16(coverage) + 127) / 255)
			src.SetNRGBA(px, py, color.NRGBA{R: col.R, G: col.G, B: col.B, A: col.A})
		}
	}
	draw.Draw(c.Img, b, src, b.Min, draw.Over)
}

func sampleGradient(stops []GradientStop, at float64) color.RGBA {
	if at <= stops[0].At {
		return stops[0].Color
	}
	for i := 1; i < len(stops); i++ {
		if at <= stops[i].At {
			a, b := stops[i-1], stops[i]
			f := (at - a.At) / max(b.At-a.At, 1e-9)
			mix := func(x, y uint8) uint8 { return uint8(math.Round(float64(x) + (float64(y)-float64(x))*f)) }
			return color.RGBA{mix(a.Color.R, b.Color.R), mix(a.Color.G, b.Color.G), mix(a.Color.B, b.Color.B), mix(a.Color.A, b.Color.A)}
		}
	}
	return stops[len(stops)-1].Color
}

func (c *Canvas) pathMask(d string, viewBox [4]float32, x, y, w, h int, evenOdd bool) *image.Alpha {
	paths, _ := parsePath(d)
	if len(paths) == 0 || viewBox[2] <= 0 || viewBox[3] <= 0 {
		return nil
	}
	b := image.Rect(x, y, x+w, y+h).Intersect(c.Bounds())
	if b.Empty() {
		return nil
	}
	mask := image.NewAlpha(b)
	scale := min(float32(w)/viewBox[2], float32(h)/viewBox[3])
	offX := float32(x) + (float32(w)-viewBox[2]*scale)/2 - viewBox[0]*scale
	offY := float32(y) + (float32(h)-viewBox[3]*scale)/2 - viewBox[1]*scale
	render := func(sub []subpath, dst *image.Alpha) {
		var ras vector.Rasterizer
		ras.Reset(b.Dx(), b.Dy())
		for _, s := range sub {
			if len(s.pts) < 2 {
				continue
			}
			x0, y0 := s.pts[0].x*scale+offX-float32(b.Min.X), s.pts[0].y*scale+offY-float32(b.Min.Y)
			ras.MoveTo(x0, y0)
			for _, p := range s.pts[1:] {
				ras.LineTo(p.x*scale+offX-float32(b.Min.X), p.y*scale+offY-float32(b.Min.Y))
			}
			// SVG fills implicitly close open subpaths too.
			ras.ClosePath()
		}
		ras.DrawOp = draw.Src
		ras.Draw(dst, b, image.Opaque, image.Point{})
	}
	if !evenOdd {
		render(paths, mask)
		return mask
	}
	tmp := image.NewAlpha(b)
	for _, p := range paths {
		clear(tmp.Pix)
		render([]subpath{p}, tmp)
		for i, v := range tmp.Pix {
			a := uint32(mask.Pix[i])
			mask.Pix[i] = uint8(a + uint32(v) - 2*a*uint32(v)/255)
		}
	}
	return mask
}

// FillSVG draws the path elements from a simple logo SVG. It deliberately ignores CSS,
// filters, and text so mark rendering stays independent of the typography package.
func (c *Canvas) FillSVG(src string, x, y, size int, col color.RGBA) {
	c.FillSVGRect(src, x, y, size, size, col)
}

// FillSVGRect draws the path elements from a simple logo SVG into a rectangle.
func (c *Canvas) FillSVGRect(src string, x, y, w, h int, col color.RGBA) {
	dec := xml.NewDecoder(strings.NewReader(src))
	var vb [4]float32
	type pathSpec struct {
		d       string
		evenOdd bool
	}
	var paths []pathSpec
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		attrs := make(map[string]string, len(start.Attr))
		for _, a := range start.Attr {
			attrs[a.Name.Local] = a.Value
		}
		if start.Name.Local == "svg" {
			v := strings.Fields(attrs["viewBox"])
			if len(v) == 4 {
				for i := range vb {
					f, _ := strconv.ParseFloat(v[i], 32)
					vb[i] = float32(f)
				}
			}
		}
		if start.Name.Local == "path" && attrs["d"] != "" {
			paths = append(paths, pathSpec{d: attrs["d"], evenOdd: attrs["fill-rule"] == "evenodd"})
		}
	}
	if vb[2] <= 0 || vb[3] <= 0 {
		return
	}
	for _, p := range paths {
		c.FillPathRect(p.d, vb, x, y, w, h, col, p.evenOdd)
	}
}

type pt struct{ x, y float32 }

type subpath struct {
	pts    []pt
	closed bool
}

// parsePath follows the SVG path subset used by Hover's logos: M/L/H/V/C/S/Q/T/A/Z,
// including relative commands. Curves and arcs are flattened for x/image/vector.
func parsePath(d string) ([]subpath, bool) {
	var out []subpath
	var cur *subpath
	var x, y, sx, sy, cx, cy, qx, qy float32
	var prev byte
	i := 0
	skip := func() {
		for i < len(d) && (d[i] == ' ' || d[i] == ',' || d[i] == '\n' || d[i] == '\t' || d[i] == '\r') {
			i++
		}
	}
	number := func() (float32, bool) {
		skip()
		j := i
		if j < len(d) && (d[j] == '-' || d[j] == '+') {
			j++
		}
		dot := false
		for j < len(d) {
			if d[j] >= '0' && d[j] <= '9' {
				j++
			} else if d[j] == '.' && !dot {
				dot = true
				j++
			} else if (d[j] == 'e' || d[j] == 'E') && j > i && j+1 < len(d) && (d[j+1] == '-' || d[j+1] == '+' || (d[j+1] >= '0' && d[j+1] <= '9')) {
				j += 2
			} else {
				break
			}
		}
		if j == i {
			return 0, false
		}
		v, err := strconv.ParseFloat(d[i:j], 32)
		i = j
		return float32(v), err == nil
	}
	flag := func() float32 {
		skip()
		if i < len(d) && (d[i] == '0' || d[i] == '1') {
			i++
			return float32(d[i-1] - '0')
		}
		return 0
	}
	start := func(px, py float32) {
		out = append(out, subpath{pts: []pt{{px, py}}})
		cur = &out[len(out)-1]
	}
	add := func(px, py float32) {
		if cur == nil {
			start(x, y)
		}
		cur.pts = append(cur.pts, pt{px, py})
	}
	cubic := func(x1, y1, x2, y2, x3, y3 float32) {
		x0, y0 := x, y
		for s := 1; s <= 14; s++ {
			t := float32(s) / 14
			u := 1 - t
			add(u*u*u*x0+3*u*u*t*x1+3*u*t*t*x2+t*t*t*x3, u*u*u*y0+3*u*u*t*y1+3*u*t*t*y2+t*t*t*y3)
		}
		cx, cy = x2, y2
		x, y = x3, y3
	}
	var cmd byte
	for {
		skip()
		if i >= len(d) {
			break
		}
		if unicode.IsLetter(rune(d[i])) {
			cmd = d[i]
			i++
			if cmd == 'Z' || cmd == 'z' {
				if cur != nil {
					cur.closed = true
					cur = nil
				}
				x, y = sx, sy
				prev = cmd
				continue
			}
		} else if cmd == 0 || cmd == 'Z' || cmd == 'z' {
			break
		}
		rel := cmd >= 'a'
		o := func(v, base float32) float32 {
			if rel {
				return v + base
			}
			return v
		}
		ok := true
		get := func() float32 {
			v, k := number()
			ok = ok && k
			return v
		}
		switch cmd | 0x20 {
		case 'm':
			nx, ny := get(), get()
			x, y = o(nx, x), o(ny, y)
			sx, sy = x, y
			start(x, y)
			if rel {
				cmd = 'l'
			} else {
				cmd = 'L'
			}
		case 'l':
			nx, ny := get(), get()
			x, y = o(nx, x), o(ny, y)
			add(x, y)
		case 'h':
			x = o(get(), x)
			add(x, y)
		case 'v':
			y = o(get(), y)
			add(x, y)
		case 'c':
			a, b, c, e, f, g := get(), get(), get(), get(), get(), get()
			if ok {
				cubic(o(a, x), o(b, y), o(c, x), o(e, y), o(f, x), o(g, y))
			}
		case 's':
			c, e, f, g := get(), get(), get(), get()
			x1, y1 := x, y
			if strings.ContainsRune("csCS", rune(prev)) {
				x1, y1 = 2*x-cx, 2*y-cy
			}
			if ok {
				cubic(x1, y1, o(c, x), o(e, y), o(f, x), o(g, y))
			}
		case 'q', 't':
			var x1, y1 float32
			if cmd|0x20 == 'q' {
				a, b := get(), get()
				x1, y1 = o(a, x), o(b, y)
			} else {
				x1, y1 = x, y
				if strings.ContainsRune("qtQT", rune(prev)) {
					x1, y1 = 2*x-qx, 2*y-qy
				}
			}
			f, g := get(), get()
			if ok {
				ex, ey := o(f, x), o(g, y)
				cubic(x+2*(x1-x)/3, y+2*(y1-y)/3, ex+2*(x1-ex)/3, ey+2*(y1-ey)/3, ex, ey)
				qx, qy = x1, y1
			}
		case 'a':
			rx, ry, rot := get(), get(), get()
			la, sw := flag(), flag()
			nx, ny := get(), get()
			if ok {
				ex, ey := o(nx, x), o(ny, y)
				arcTo(x, y, rx, ry, rot, la != 0, sw != 0, ex, ey, add)
				x, y = ex, ey
			}
		default:
			ok = false
		}
		if !ok {
			break
		}
		prev = cmd
	}
	return out, true
}

func arcTo(x0, y0, rx, ry, rotDeg float32, large, sweep bool, x1, y1 float32, add func(float32, float32)) {
	if rx == 0 || ry == 0 || x0 == x1 && y0 == y1 {
		add(x1, y1)
		return
	}
	rx, ry = float32(math.Abs(float64(rx))), float32(math.Abs(float64(ry)))
	phi := float64(rotDeg) * math.Pi / 180
	cp, sp := math.Cos(phi), math.Sin(phi)
	dx, dy := float64(x0-x1)/2, float64(y0-y1)/2
	xp, yp := cp*dx+sp*dy, -sp*dx+cp*dy
	rx2, ry2 := float64(rx)*float64(rx), float64(ry)*float64(ry)
	if l := xp*xp/rx2 + yp*yp/ry2; l > 1 {
		s := math.Sqrt(l)
		rx, ry = float32(float64(rx)*s), float32(float64(ry)*s)
		rx2, ry2 = float64(rx)*float64(rx), float64(ry)*float64(ry)
	}
	num, den := rx2*ry2-rx2*yp*yp-ry2*xp*xp, rx2*yp*yp+ry2*xp*xp
	co := 0.0
	if den != 0 && num > 0 {
		co = math.Sqrt(num / den)
	}
	if large == sweep {
		co = -co
	}
	cxp, cyp := co*float64(rx)*yp/float64(ry), -co*float64(ry)*xp/float64(rx)
	cx := cp*cxp - sp*cyp + float64(x0+x1)/2
	cy := sp*cxp + cp*cyp + float64(y0+y1)/2
	angle := func(ux, uy, vx, vy float64) float64 { return math.Atan2(ux*vy-uy*vx, ux*vx+uy*vy) }
	t1 := angle(1, 0, (xp-cxp)/float64(rx), (yp-cyp)/float64(ry))
	dt := angle((xp-cxp)/float64(rx), (yp-cyp)/float64(ry), (-xp-cxp)/float64(rx), (-yp-cyp)/float64(ry))
	if !sweep && dt > 0 {
		dt -= 2 * math.Pi
	} else if sweep && dt < 0 {
		dt += 2 * math.Pi
	}
	n := max(4, int(math.Abs(dt)/(math.Pi/2)*8))
	for i := 1; i <= n; i++ {
		t := t1 + dt*float64(i)/float64(n)
		ex, ey := float64(rx)*math.Cos(t), float64(ry)*math.Sin(t)
		add(float32(cp*ex-sp*ey+cx), float32(sp*ex+cp*ey+cy))
	}
	add(x1, y1)
}
