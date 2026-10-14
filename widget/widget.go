package widget

import (
	"image"
	"image/color"
	"math"
	"strings"

	"agent-quota/internal/marks"
	"agent-quota/internal/raster"
	"agent-quota/internal/text"
	"agent-quota/quota"
)

type Item struct {
	ID     string
	State  quota.State
	Used   float64
	HasPct bool
	Detail string
	Err    string
	Stale  string
	// Reset (the panel detail line, e.g. "5h resets in 2h 14m") and PillReset
	// (the pill's second line, "2h14m") are preformatted upstream — already
	// toggled countdown/clock — so Render stays deterministic and never calls
	// time.Now; empty strings mean this row has no 5h-window reset to show.
	Reset string
	// PillReset draws as the pill's second line under the percent; empty keeps
	// the single centred line.
	PillReset string
}

// View contains only display state; rendering and hit geometry use no window.
type View struct {
	Items          []Item
	Open           bool
	Checked        string
	Scale          float32
	Above          bool
	RefreshHovered bool
}

const (
	pad      = 14
	gap      = 12
	cell     = 24
	pctGap   = 5
	PctWidth = 31
	pillH    = 40
	panelW   = 392
	panelGap = 8
	headerH  = 40
	rowH     = 56
	footerH  = 44
)

var (
	pillInk     = color.RGBA{0, 0, 0, 255}
	panelInk    = color.RGBA{0x13, 0x11, 0x16, 255}
	edgeInk     = color.RGBA{255, 255, 255, 0x14}
	whiteInk    = color.NRGBA{255, 255, 255, 255}
	dimInk      = color.NRGBA{255, 255, 255, 0xA6}
	mutedInk    = color.NRGBA{255, 255, 255, 0x73}
	numberFont  = text.Style{Size: 11.5, Weight: 600}
	percentFont = text.Style{Size: 10.5, Weight: 600}
	titleFont   = text.Style{Size: 12.5, Weight: 600}
	detailFont  = text.Style{Size: 11.5, Weight: 400}
	// The pill's second line reads as a footnote under the percent: smaller
	// and medium weight so it stays legible at 9 px without competing.
	resetFont = text.Style{Size: 9, Weight: 500}
)

func PillSize(n, pctW int) (int, int) {
	if n == 0 {
		return 152, pillH
	}
	return 2*pad + n*(cell+pctGap+pctW) + (n-1)*gap, pillH
}

type geometry struct {
	w, h, pw, ph, panelY int
	scale                float32
}

func layout(v View) geometry {
	scale := v.Scale
	if scale <= 0 || math.IsNaN(float64(scale)) || math.IsInf(float64(scale), 0) {
		scale = 1
	}
	pw, ph := PillSize(len(v.Items), PctWidth)
	g := geometry{w: pw, h: ph, pw: pw, ph: ph, scale: scale}
	if v.Open {
		g.w = max(pw, panelW)
		g.panelY = ph + panelGap
		g.h += panelGap + headerH + max(len(v.Items), 1)*rowH + footerH
		if v.Above {
			g.panelY = 0
		}
	}
	return g
}

func (g geometry) px(n int) int { return int(math.Round(float64(float32(n) * g.scale))) }

func Size(v View) (int, int) {
	g := layout(v)
	return g.px(g.w), g.px(g.h)
}

// Keep the pill anchored while its inspector grows leftwards or above it.
func PillBounds(v View) image.Rectangle {
	g := layout(v)
	y := 0
	if v.Open && v.Above {
		y = g.h - g.ph
	}
	return image.Rect(g.px(g.w-g.pw), g.px(y), g.px(g.w), g.px(y+g.ph))
}

func RefreshBounds(v View) image.Rectangle {
	if !v.Open {
		return image.Rectangle{}
	}
	g := layout(v)
	y := g.panelY + headerH + max(len(v.Items), 1)*rowH + 9
	return image.Rect(g.px(g.w-94), g.px(y), g.px(g.w-16), g.px(y+26))
}

// ResetRowBounds is the hit/probe rect for a row's reset countdown. It follows
// the RefreshBounds pattern (closed panel guards, y from panel rows, every
// coordinate through g.px) and returns the full panel row so --shot stays
// pixel-testable without substring measuring.
func ResetRowBounds(v View, id string) image.Rectangle {
	if !v.Open {
		return image.Rectangle{}
	}
	g := layout(v)
	for i, it := range v.Items {
		if it.ID != id {
			continue
		}
		// An empty Reset means no reset for this row (Waiting/Failed, or
		// agents without a 5h window).
		if it.Reset == "" {
			return image.Rectangle{}
		}
		y := g.panelY + headerH + i*rowH
		return image.Rect(g.px(0), g.px(y), g.px(g.w), g.px(y+rowH))
	}
	return image.Rectangle{}
}

func usage(it Item) (string, bool) {
	if it.State == quota.OK || it.State == quota.Stale {
		return quota.Custom(it.Used, 0), true
	}
	return "—", false
}

// Measure glyphs instead of cutting byte offsets, so punctuation and error
// messages keep their meaning and never end in a broken UTF-8 character.
func detailLines(s string, maxW float32, st text.Style) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	line := words[0]
	for i := 1; i < len(words); i++ {
		if text.Measure(line+" "+words[i], st) > maxW {
			return []string{text.Elide(line, st, maxW), text.Elide(strings.Join(words[i:], " "), st, maxW)}
		}
		line += " " + words[i]
	}
	return []string{text.Elide(line, st, maxW)}
}

func detail(it Item) string {
	var base string
	switch it.State {
	case quota.Waiting:
		base = "Checking usage…"
	case quota.Failed:
		if it.Err != "" {
			base = it.Err
			break
		}
		base = it.Detail
	case quota.Stale:
		base = it.Err + " · last good " + it.Stale
	default:
		base = it.Detail
	}
	// The 5h-window reset rides in the same dim detail run, appended after
	// existing Detail so wrapping and style stay shared instead of adding a
	// second text style or clock call.
	if it.Reset != "" {
		if base != "" {
			base += " · " + it.Reset
		} else {
			base = it.Reset
		}
	}
	return base
}

func Render(v View) *image.RGBA {
	g := layout(v)
	c := raster.New(g.px(g.w), g.px(g.h))
	pill := PillBounds(v)
	c.FillRound(pill.Min.X, pill.Min.Y, pill.Dx(), pill.Dy(), g.px(20), pillInk)
	c.StrokeRound(pill.Min.X, pill.Min.Y, pill.Dx(), pill.Dy(), g.px(20), max(1, g.px(1)), edgeInk)
	styled := func(st text.Style) text.Style { st.Size *= g.scale; return st }
	draw := func(x, y int, s string, st text.Style, col color.NRGBA) {
		text.Draw(c.Img, float32(g.px(x)), float32(g.px(y)), s, styled(st), col)
	}
	py := 0
	if v.Open && v.Above {
		py = g.h - g.ph
	}
	x := g.w - g.pw + pad
	if len(v.Items) == 0 {
		draw(x, py+12, "No agents enabled", detailFont, dimInk)
	}
	for _, it := range v.Items {
		fg := marks.ForUsed(it.Used)
		frac := it.Used / 100
		if it.State != quota.OK && it.State != quota.Stale {
			frac = 0
		}
		if it.State == quota.Stale {
			fg.A = 0x88
		}
		c.RingF(float64(g.px(x+12)), float64(g.px(py+20)), 10.75*float64(g.scale), 2*float64(g.scale), frac, fg, color.RGBA{255, 255, 255, 0x33})
		marks.Draw(c, it.ID, g.px(x+5), g.px(py+13), g.px(14))
		value, hasPct := usage(it)
		col := whiteInk
		if it.State != quota.OK {
			col = dimInk
		}
		// A PillReset grows a second line: the percent rides ~7 px higher so
		// the pair stays centred inside the fixed 40 px pill, with the reset
		// underneath in the footnote style. No reset keeps the single centred
		// line byte-identical to before.
		numY, pctY := py+12, py+13
		if it.PillReset != "" {
			numY, pctY = py+5, py+6
		}
		draw(x+cell+pctGap, numY, value, numberFont, col)
		if hasPct {
			vx := float32(g.px(x+cell+pctGap)) + text.Measure(value, styled(numberFont))
			text.Draw(c.Img, vx, float32(g.px(pctY)), "%", styled(percentFont), mutedInk)
		}
		if it.PillReset != "" {
			// Elide inside the percent slot: PillSize fixes the pill width, so
			// a longer string would bleed over the next agent's ring.
			draw(x+cell+pctGap, py+21, text.Elide(it.PillReset, resetFont, float32(PctWidth)), resetFont, mutedInk)
		}
		x += cell + pctGap + PctWidth + gap
	}
	if !v.Open {
		return c.Img
	}
	panelH := headerH + max(len(v.Items), 1)*rowH + footerH
	c.FillRound(0, g.px(g.panelY), g.px(g.w), g.px(panelH), g.px(16), panelInk)
	c.StrokeRound(0, g.px(g.panelY), g.px(g.w), g.px(panelH), g.px(16), max(1, g.px(1)), edgeInk)
	draw(16, g.panelY+13, "Usage limits", titleFont, whiteInk)
	if len(v.Items) == 0 {
		draw(16, g.panelY+headerH+9, "Enable an agent in the right-click menu.", detailFont, dimInk)
	}
	for i, it := range v.Items {
		y := g.panelY + headerH + i*rowH
		marks.Draw(c, it.ID, g.px(16), g.px(y+8), g.px(26))
		draw(54, y+5, quota.ItemShort(it.ID), titleFont, whiteInk)
		value, hasPct := usage(it)
		if hasPct {
			value += "%"
		}
		valueW := text.Measure(value, styled(titleFont))
		col := whiteInk
		if it.State == quota.Failed || it.State == quota.Waiting {
			col = mutedInk
		}
		text.Draw(c.Img, float32(g.px(g.w-16))-valueW, float32(g.px(y+5)), value, styled(titleFont), col)
		for j, line := range detailLines(detail(it), float32(g.w-70), detailFont) {
			draw(54, y+23+j*14, line, detailFont, dimInk)
		}
		if i < len(v.Items)-1 {
			c.Rect(g.px(54), g.px(y+rowH-1), g.px(g.w-70), max(1, g.px(1)), color.RGBA{255, 255, 255, 0x0A})
		}
	}
	fy := g.panelY + headerH + max(len(v.Items), 1)*rowH
	c.Rect(g.px(16), g.px(fy), g.px(g.w-32), max(1, g.px(1)), edgeInk)
	checked := v.Checked
	if checked == "" {
		checked = "Checking usage…"
	}
	draw(16, fy+14, text.Elide(checked, detailFont, float32(g.w-126)), detailFont, mutedInk)
	r := RefreshBounds(v)
	buttonInk := color.RGBA{255, 255, 255, 0x12}
	if v.RefreshHovered {
		buttonInk.A = 0x26
	}
	c.FillRound(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), g.px(7), buttonInk)
	c.StrokeRound(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), g.px(7), max(1, g.px(1)), edgeInk)
	refreshStyle := styled(text.Style{Size: 11.5, Weight: 500})
	refreshWidth := text.Measure("Refresh", refreshStyle)
	text.Draw(c.Img, float32(r.Min.X)+(float32(r.Dx())-refreshWidth)/2, float32(r.Min.Y)+6*g.scale, "Refresh", refreshStyle, whiteInk)
	return c.Img
}
