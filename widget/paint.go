package widget

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"agent-quota/internal/marks"
	"agent-quota/internal/raster"
	"agent-quota/internal/text"
	"agent-quota/quota"
)

type painter struct {
	c *raster.Canvas
	g geometry
}

func (p painter) style(st text.Style) text.Style { st.Size *= p.g.scale; return st }
func (p painter) draw(x, y int, s string, st text.Style, ink color.NRGBA) {
	text.Draw(p.c.Img, float32(x), float32(y), s, p.style(st), ink)
}
func (p painter) right(x, y int, s string, st text.Style, ink color.NRGBA) {
	text.Draw(p.c.Img, float32(x)-text.Measure(s, p.style(st)), float32(y), s, p.style(st), ink)
}
func (p painter) step(n int) int      { return p.g.px(n) }
func rgba(ink color.NRGBA) color.RGBA { return color.RGBA{R: ink.R, G: ink.G, B: ink.B, A: ink.A} }
func valueInk(it Item) color.NRGBA {
	if it.State != quota.OK {
		return dimInk
	}
	if used := headlineUsed(it); used >= 70 {
		v := marks.ForUsed(used)
		return color.NRGBA{R: v.R, G: v.G, B: v.B, A: 255}
	}
	return whiteInk
}

func (p painter) mark(it Item, x, y, size int) {
	if it.State == quota.OK || it.State == quota.Stale {
		marks.Draw(p.c, it.ID, x, y, size)
		return
	}
	tile := raster.New(size, size)
	marks.Draw(tile, it.ID, 0, 0, size)
	for i := range tile.Img.Pix {
		tile.Img.Pix[i] = uint8(uint16(tile.Img.Pix[i]) * 120 / 255)
	}
	p.c.Blit(x, y, tile.Img)
}

func (p painter) ring(it Item, x, y, size int) {
	extra := p.step(ringExtra(it))
	cx, cy := float64(x)+float64(size+extra)/2, float64(y)+float64(size)/2
	radius := float64(size) * .425
	arc := func(used, r float64) {
		ink := marks.ForUsed(used)
		fraction := used / 100
		if it.State != quota.OK && it.State != quota.Stale {
			fraction = 0
		}
		if it.State == quota.Stale {
			ink.A = 128
		}
		p.c.RingF(cx, cy, r, 1.65*float64(p.g.scale), fraction, ink, color.RGBA{R: 255, G: 255, B: 255, A: 38})
	}
	if extra > 0 {
		arc(poolWindow(it, "week").Used, radius+3*float64(p.g.scale))
	}
	arc(headlineUsed(it), radius)
	markSize := p.step(18)
	p.mark(it, x+(size+extra-markSize)/2, y+(size-markSize)/2, markSize)
}

func (p painter) icon(name string, rect image.Rectangle, ink color.NRGBA) {
	side := min(rect.Dx(), rect.Dy())
	x, y := rect.Min.X+(rect.Dx()-side)/2, rect.Min.Y+(rect.Dy()-side)/2
	line := func(x1, y1, x2, y2 float64) {
		p.c.Line(float64(x)+x1*float64(side)/16, float64(y)+y1*float64(side)/16, float64(x)+x2*float64(side)/16, float64(y)+y2*float64(side)/16, 1.35*float64(p.g.scale), rgba(ink))
	}
	switch name {
	case "close":
		line(5, 5, 11, 11)
		line(11, 5, 5, 11)
	case "down":
		line(4.5, 6.5, 8, 10)
		line(8, 10, 11.5, 6.5)
	case "right":
		line(6.5, 4.5, 10, 8)
		line(10, 8, 6.5, 11.5)
	case "pin":
		for _, s := range [][4]float64{{9, 2, 14, 7}, {14, 7, 11, 8}, {11, 8, 9, 12}, {9, 12, 3, 6}, {3, 6, 7, 5}, {7, 5, 9, 2}, {7, 10, 3, 14}} {
			line(s[0], s[1], s[2], s[3])
		}
	case "clock":
		p.c.RingF(float64(x+side/2), float64(y+side/2), float64(side)*.34, float64(p.g.scale), 1, rgba(ink), color.RGBA{})
		line(8, 4.5, 8, 8)
		line(8, 8, 10.5, 9.5)
	case "refresh":
		p.c.RingF(float64(x+side/2), float64(y+side/2), float64(side)*.32, 1.2*float64(p.g.scale), .78, rgba(ink), color.RGBA{})
		line(12.5, 2, 12.5, 6)
		line(12.5, 6, 9, 6)
	case "empty":
		p.c.StrokeRound(x+p.step(2), y+p.step(3), side-p.step(4), side-p.step(6), p.step(2), max(1, p.step(1)), rgba(ink))
		line(6, 8, 10, 8)
		line(8, 6, 8, 10)
	}
}

func (p painter) button(rect image.Rectangle, name string, hover, selected bool) {
	if hover || selected {
		alpha := uint8(14)
		if hover {
			alpha = 24
		}
		p.c.FillRound(rect.Min.X, rect.Min.Y, rect.Dx(), rect.Dy(), p.step(6), color.RGBA{R: 255, G: 255, B: 255, A: alpha})
	}
	inner := rect.Inset(p.step(6))
	ink := mutedInk
	if hover || selected {
		ink = whiteInk
	}
	p.icon(name, inner, ink)
}

func Render(v View) *image.RGBA {
	if v.Focus {
		return renderFocus(v)
	}
	if v.Seam {
		img := renderSeam(v)
		if v.DockPreview {
			c := &raster.Canvas{Img: img}
			r := PillBounds(v)
			g := layout(v)
			c.StrokeRound(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), g.px(12), max(1, g.px(1)), color.RGBA{R: 145, G: 170, B: 190, A: 255})
		}
		return img
	}
	g := layout(v)
	w, h := Size(v)
	p := painter{c: raster.New(w, h), g: g}
	c := p.c
	pill := PillBounds(v)
	if v.Open {
		panel := PanelBounds(v)
		bridge := image.Rect(max(pill.Min.X, panel.Min.X), min(pill.Max.Y, panel.Max.Y), min(pill.Max.X, panel.Max.X), max(pill.Min.Y, panel.Min.Y))
		c.Rect(bridge.Min.X, bridge.Min.Y, bridge.Dx(), bridge.Dy(), color.RGBA{A: 1})
	}
	c.ShadowRound(pill, p.step(22), p.step(12), p.step(3))
	c.FillRound(pill.Min.X, pill.Min.Y, pill.Dx(), pill.Dy(), p.step(22), pillInk)
	c.StrokeRound(pill.Min.X, pill.Min.Y, pill.Dx(), pill.Dy(), p.step(22), max(1, p.step(1)), edgeInk)
	p.drawRail(v, pill)
	if !v.Open {
		return c.Img
	}
	panel := PanelBounds(v)
	c.ShadowRound(panel, p.step(16), p.step(12), p.step(4))
	c.FillRound(panel.Min.X, panel.Min.Y, panel.Dx(), panel.Dy(), p.step(16), panelInk)
	c.StrokeRound(panel.Min.X, panel.Min.Y, panel.Dx(), panel.Dy(), p.step(16), max(1, p.step(1)), edgeInk)
	p.draw(panel.Min.X+p.step(17), panel.Min.Y+p.step(17), "Usage limits", text.Style{Size: 13, Weight: 600}, whiteInk)
	p.draw(panel.Min.X+p.step(103), panel.Min.Y+p.step(19), fmt.Sprintf("%d agents", len(v.Items)), captionFont, mutedInk)
	p.button(PinBounds(v), "pin", v.Hovered.Control == PinControl, v.Pinned)
	p.button(CloseBounds(v), "close", v.Hovered.Control == CloseControl, false)
	if len(v.Items) == 0 {
		mid := panel.Min.Y + p.step(headerH+58)
		p.icon("empty", image.Rect(panel.Min.X+p.step(179), mid, panel.Min.X+p.step(207), mid+p.step(28)), mutedInk)
		p.center(panel.Min.X+panel.Dx()/2, mid+p.step(43), "No agents enabled", titleFont, whiteInk)
		p.center(panel.Min.X+panel.Dx()/2, mid+p.step(67), "Right-click the widget to choose agents.", detailFont, dimInk)
	} else {
		for _, it := range v.Items {
			p.drawRow(v, it, rowBounds(v, it.ID))
		}
	}
	p.drawFooter(v, panel)
	return c.Img
}

func (p painter) center(x, y int, s string, st text.Style, ink color.NRGBA) {
	p.draw(x-int(text.Measure(s, p.style(st))/2), y, s, st, ink)
}

func (p painter) drawRail(v View, r image.Rectangle) {
	if len(v.Items) == 0 {
		p.draw(r.Min.X+p.step(15), r.Min.Y+p.step(14), "No agents enabled", detailFont, dimInk)
		return
	}
	if v.Compact {
		it := v.Items[0]
		for _, candidate := range v.Items {
			if candidate.ID == v.Selected {
				it = candidate
				break
			}
		}
		p.ring(it, r.Min.X+p.step(5), r.Min.Y+p.step(5), p.step(28))
		s, hasPct := usage(it)
		if hasPct {
			s += "%"
		}
		p.draw(r.Min.X+p.step(41+ringExtra(it)), r.Min.Y+p.step(12), s, numberFont, valueInk(it))
	} else {
		for _, it := range v.Items {
			b := PillItemBounds(v, it.ID)
			if v.Hovered == (Target{Control: AgentControl, ID: it.ID}) {
				p.c.FillRound(b.Min.X-p.step(2), b.Min.Y, b.Dx()+p.step(4), b.Dy(), p.step(7), color.RGBA{R: 255, G: 255, B: 255, A: 10})
			}
			p.ring(it, b.Min.X, r.Min.Y+p.step(8), p.step(28))
			x := b.Min.X + p.step(cell+pctGap+ringExtra(it))
			y := r.Min.Y + p.step(15)
			if it.PillReset != "" {
				y = r.Min.Y + p.step(7)
			}
			s, hasPct := usage(it)
			p.draw(x, y, s, numberFont, valueInk(it))
			if hasPct {
				vx := float32(x) + text.Measure(s, p.style(numberFont))
				text.Draw(p.c.Img, vx, float32(y+p.step(2)), "%", p.style(percentFont), valueInk(it))
			}
			if it.PillReset != "" {
				p.draw(x, r.Min.Y+p.step(25), text.Elide(it.PillReset, resetFont, PctWidth), resetFont, mutedInk)
			}
		}
		x := r.Max.X - p.step(37)
		p.c.Rect(x, r.Min.Y+p.step(14), max(1, p.step(1)), p.step(16), edgeInk)
	}
	right := image.Rect(r.Max.X-p.step(31), r.Min.Y+p.step(8), r.Max.X-p.step(7), r.Max.Y-p.step(8))
	name := "down"
	if !v.Open {
		name = "right"
	}
	p.button(right, name, v.Hovered.Control == OpenControl, false)
}

func (p painter) drawRow(v View, it Item, r image.Rectangle) {
	x := r.Min.X + p.step(54)
	right := r.Max.X - p.step(16)
	p.mark(it, r.Min.X+p.step(17), r.Min.Y+p.step(12), p.step(27))
	p.draw(x, r.Min.Y+p.step(8), quota.ItemShort(it.ID), titleFont, whiteInk)
	nameWidth := text.Measure(quota.ItemShort(it.ID), p.style(titleFont))
	if it.Plan != "" {
		space := float32(right-x) - nameWidth - float32(p.step(56))
		p.draw(x+int(nameWidth)+p.step(7), r.Min.Y+p.step(10), text.Elide(it.Plan, p.style(captionFont), max(space, 0)), captionFont, mutedInk)
	}
	s, hasPct := usage(it)
	if hasPct {
		s += "%"
	}
	p.right(right, r.Min.Y+p.step(8), s, text.Style{Size: 13, Weight: 600}, valueInk(it))
	if it.State == quota.Waiting {
		p.c.FillRound(x, r.Min.Y+p.step(33), p.step(106), p.step(6), p.step(3), color.RGBA{R: 53, G: 48, B: 61, A: 255})
	} else if it.State == quota.Failed {
		message := it.Err
		if message == "" {
			message = it.Detail
		}
		ink := errorInk
		if v.RecoveryOpen == it.ID {
			message = it.Help
			ink = dimInk
		}
		for i, line := range detailLines(message, float32(panelW-70), detailFont) {
			p.draw(x, r.Min.Y+p.step(27+i*13), line, detailFont, ink)
		}
		if it.Help != "" {
			label := "How to recover"
			if v.RecoveryOpen == it.ID {
				label = "Back to error"
			}
			ink := mutedInk
			if v.Hovered == (Target{Control: RecoveryControl, ID: it.ID}) {
				ink = whiteInk
			}
			p.draw(x, r.Min.Y+p.step(54), label, captionFont, ink)
			p.icon("right", image.Rect(x+p.step(83), r.Min.Y+p.step(55), x+p.step(93), r.Min.Y+p.step(65)), ink)
		}
	} else {
		reset := it.ResetLabel
		if it.Reset != "" {
			reset = "5h · " + strings.TrimPrefix(it.Reset, "5h resets ")
		}
		maxWidth := float32(panelW - 70)
		if reset != "" {
			rw := text.Measure(reset, p.style(captionFont))
			rx := right - int(math.Ceil(float64(rw)))
			if v.Hovered == (Target{Control: ResetControl, ID: it.ID}) {
				p.c.FillRound(rx-p.step(17), r.Min.Y+p.step(25), right-rx+p.step(19), p.step(18), p.step(4), color.RGBA{R: 255, G: 255, B: 255, A: 15})
			}
			p.right(right, r.Min.Y+p.step(29), reset, captionFont, dimInk)
			p.icon("clock", image.Rect(rx-p.step(15), r.Min.Y+p.step(30), rx-p.step(5), r.Min.Y+p.step(40)), mutedInk)
			maxWidth = float32(rx-x-p.step(23)) / p.g.scale
		}
		if len(it.Groups) > 0 {
			ink := dimInk
			if v.Hovered == (Target{Control: GroupsControl, ID: it.ID}) {
				ink = whiteInk
			}
			label := fmt.Sprintf("%d model groups", len(it.Groups))
			p.draw(x, r.Min.Y+p.step(29), label, detailFont, ink)
			ox := x + int(text.Measure(label, p.style(detailFont))) + p.step(4)
			name := "right"
			if v.GroupsOpen {
				name = "down"
			}
			p.icon(name, image.Rect(ox, r.Min.Y+p.step(30), ox+p.step(11), r.Min.Y+p.step(41)), mutedInk)
			if v.GroupsOpen {
				y := 54
				if it.State == quota.Stale {
					y += 14
				}
				p.drawGroups(it, x, r.Min.Y+p.step(y), right)
			}
		} else {
			if it.Reset != "" || len(it.Windows) > 0 || it.Summary != "" {
				p.draw(x, r.Min.Y+p.step(28), text.Elide(metrics(it), detailFont, maxWidth), detailFont, dimInk)
			} else {
				for i, line := range detailLines(metrics(it), maxWidth, detailFont) {
					p.draw(x, r.Min.Y+p.step(28+i*13), line, detailFont, dimInk)
				}
			}
		}
		if it.State == quota.Stale {
			p.draw(x, r.Min.Y+p.step(48), text.Elide(it.Err+" · last good "+it.Stale, captionFont, panelW-70), captionFont, staleInk)
		}
	}
	if r.Max.Y < PanelBounds(v).Max.Y-p.step(footerH) {
		p.c.Rect(x, r.Max.Y-max(1, p.step(1)), right-x, max(1, p.step(1)), color.RGBA{R: 255, G: 255, B: 255, A: 13})
	}
}

func (p painter) drawGroups(it Item, x, y, right int) {
	var labels []string
	for _, group := range it.Groups {
		for _, w := range group.Windows {
			found := false
			for _, label := range labels {
				if label == w.Label {
					found = true
					break
				}
			}
			if !found {
				if w.Label == "5h" {
					labels = append([]string{w.Label}, labels...)
				} else {
					labels = append(labels, w.Label)
				}
			}
		}
	}
	width := p.step(48)
	for i, label := range labels {
		if label == "week" {
			label = "Week"
		}
		p.right(right-(len(labels)-1-i)*width, y, label, captionFont, mutedInk)
	}
	for row, group := range it.Groups {
		gy := y + p.step(18+row*22)
		p.draw(x, gy, text.Elide(group.Name, captionFont, float32(panelW-70-len(labels)*48-10)), captionFont, dimInk)
		for i, label := range labels {
			s := "—"
			for _, w := range group.Windows {
				if w.Label == label {
					s = quota.Custom(w.Used, 0) + "%"
					break
				}
			}
			p.right(right-(len(labels)-1-i)*width, gy, s, captionFont, dimInk)
		}
	}
}

func (p painter) drawFooter(v View, panel image.Rectangle) {
	y := panel.Max.Y - p.step(footerH)
	p.c.Rect(panel.Min.X+p.step(16), y, panel.Dx()-p.step(32), max(1, p.step(1)), edgeInk)
	s := v.Checked
	if v.Busy {
		s = "Checking usage…"
	}
	if s == "" {
		s = "Checking usage…"
	}
	p.c.FillRound(panel.Min.X+p.step(17), y+p.step(23), p.step(4), p.step(4), p.step(2), rgba(mutedInk))
	p.draw(panel.Min.X+p.step(26), y+p.step(18), text.Elide(s, captionFont, panelW-128), captionFont, mutedInk)
	r := RefreshBounds(v)
	alpha := uint8(12)
	if v.Hovered.Control == RefreshControl {
		alpha = 25
	}
	p.c.FillRound(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), p.step(6), color.RGBA{R: 255, G: 255, B: 255, A: alpha})
	if v.Busy {
		p.center((r.Min.X+r.Max.X)/2, r.Min.Y+p.step(7), "Checking…", captionFont, mutedInk)
	} else {
		p.icon("refresh", image.Rect(r.Min.X+p.step(9), r.Min.Y+p.step(8), r.Min.X+p.step(21), r.Min.Y+p.step(20)), dimInk)
		p.draw(r.Min.X+p.step(27), r.Min.Y+p.step(7), "Refresh", text.Style{Size: 10.5, Weight: 500}, whiteInk)
	}
}
