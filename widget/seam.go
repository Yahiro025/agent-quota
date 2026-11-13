package widget

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"agent-quota/internal/raster"
	"agent-quota/internal/text"
	"agent-quota/quota"
)

const (
	seamHeader = 24
	seamCells  = 72
	seamFooter = 36
	seamWing   = 8
	seamRadius = 14
)

func seamSelected(v View) (Item, bool) {
	for _, it := range v.Items {
		if it.ID == v.Selected {
			return it, true
		}
	}
	if len(v.Items) > 0 {
		return v.Items[0], true
	}
	return Item{}, false
}

func seamFooterHeight(v View) int {
	return seamFooter
}

func seamGroupHeight(v View) int {
	it, _ := seamSelected(v)
	if v.GroupsOpen && len(it.Details) == 0 && len(it.Groups) > 0 && (it.State == quota.OK || it.State == quota.Stale) {
		return 20 + len(it.Groups)*22
	}
	return 0
}

func seamLayout(v View) geometry {
	scale := v.Scale
	if scale <= 0 || math.IsNaN(float64(scale)) || math.IsInf(float64(scale), 0) {
		scale = 1
	}
	pw, ph := viewPillSize(v)
	g := geometry{w: pw, h: ph, pw: pw, ph: ph, scale: scale}
	if v.Open {
		fullView := v
		fullView.Compact = false
		full, _ := viewPillSize(fullView)
		g.w = max(280, full)
		if len(v.Items) > 0 {
			g.w = max(386, full)
		}
		g.h = seamHeader + seamCells + selectedDetailLayout(v, g.w).height + seamFooterHeight(v) + seamGroupHeight(v)
		g.panelH = g.h
	}
	if v.Above || v.Floating {
		g.topPad = shadowPad
	}
	return g
}

func seamPillBounds(v View) image.Rectangle {
	g := layout(v)
	y := 0
	if v.Open && v.Above {
		y = g.h - g.ph
	}
	return g.rect((g.w-g.pw)/2, y, g.pw, g.ph)
}

func seamPanelBounds(v View) image.Rectangle {
	if !v.Open {
		return image.Rectangle{}
	}
	g := layout(v)
	return g.rect(seamWing, 0, g.w-2*seamWing, g.h)
}

func seamHeaderControl(v View, close bool) image.Rectangle {
	if !v.Open {
		return image.Rectangle{}
	}
	g := layout(v)
	x := g.w - 66
	if close {
		x = g.w - 38
	}
	return g.rect(x, 1, 24, 22)
}

func seamRefreshBounds(v View) image.Rectangle {
	if !v.Open {
		return image.Rectangle{}
	}
	g := layout(v)
	return g.rect(g.w-126, g.h-30, 106, 24)
}

func seamItemBounds(v View, id string) image.Rectangle {
	g := layout(v)
	if v.Open {
		for i, it := range v.Items {
			if it.ID == id {
				left := 16 + (g.w-32)*i/max(1, len(v.Items))
				right := 16 + (g.w-32)*(i+1)/max(1, len(v.Items))
				return g.rect(left, seamHeader, right-left, seamCells-4)
			}
		}
		return image.Rectangle{}
	}
	if v.Compact {
		return image.Rectangle{}
	}
	r := PillBounds(v)
	x := pad
	for _, it := range v.Items {
		width := cell + pctGap + PctWidth + ringExtra(it)
		if it.ID == id {
			return image.Rect(r.Min.X+g.px(x), r.Min.Y+g.px(4), r.Min.X+g.px(x+width), r.Max.Y-g.px(4))
		}
		x += width + gap
	}
	return image.Rectangle{}
}

func seamDetailTarget(v View, id string, kind Control) image.Rectangle {
	if !v.Open {
		return image.Rectangle{}
	}
	it, ok := seamSelected(v)
	if !ok || it.ID != id {
		return image.Rectangle{}
	}
	g := layout(v)
	d := selectedDetailLayout(v, g.w)
	var r image.Rectangle
	switch kind {
	case RecoveryControl:
		r = d.recovery
	case GroupsControl:
		r = d.groups
	case ResetControl:
		r = d.reset
	default:
		return image.Rectangle{}
	}
	if r.Empty() {
		return image.Rectangle{}
	}
	return g.rect(r.Min.X, g.h-seamFooterHeight(v)+r.Min.Y, r.Dx(), r.Dy())
}

func seamContains(v View, pt image.Point) bool {
	g := layout(v)
	r := PillBounds(v)
	if v.Open {
		r = g.rect(0, 0, g.w, g.h)
	}
	if v.Floating {
		return raster.RoundContains(r, g.px(seamContourRadius(v)), pt)
	}
	return raster.NotchContains(r, g.px(seamWing), g.px(seamRadius), v.Above, pt)
}

func seamContourRadius(v View) int {
	if !v.Open {
		return layout(v).ph / 2
	}
	return seamRadius
}

func seamBasis(it Item) string {
	if it.State == quota.Waiting {
		return "Checking…"
	}
	if it.State == quota.Failed {
		return "Unavailable"
	}
	if it.State == quota.Stale {
		return "Stale"
	}
	if poolWindow(it, "5h") != nil {
		return "5h"
	}
	if strings.EqualFold(it.Summary, "Unlimited") {
		return "Unlimited"
	}
	if len(it.Groups) > 0 {
		return "Models"
	}
	if len(it.Windows) > 0 {
		w := it.Windows[0]
		for _, candidate := range it.Windows[1:] {
			if candidate.Used > w.Used {
				w = candidate
			}
		}
		return windowName(w.Label)
	}
	switch it.ID {
	case quota.ItemKiro:
		return "Credits"
	case quota.ItemCursor:
		return "Plan"
	case quota.ItemCopilot:
		return "Premium"
	case quota.ItemGrokBot:
		return "Week"
	case quota.ItemQoder:
		return "Credits"
	}
	return "Usage"
}

func windowName(label string) string {
	switch label {
	case "week":
		return "Week"
	case "day":
		return "Day"
	}
	return label
}

func seamDetail(it Item) string {
	if it.State == quota.Waiting {
		return "Checking usage…"
	}
	if len(it.Groups) > 0 {
		s := fmt.Sprintf("%d model groups", len(it.Groups))
		if it.Reset != "" {
			s += "  ·  " + it.Reset
		}
		return s
	}
	if it.Reset != "" {
		if len(it.Windows) > 0 {
			// The reset belongs to 5h; retain the other reported windows too.
			return metrics(it) + "  ·  " + it.Reset
		}
		return it.Reset
	}
	s := metrics(it)
	if it.ResetLabel != "" {
		s += "  ·  resets " + it.ResetLabel
	}
	return s
}

func renderSeam(v View) *image.RGBA {
	g := layout(v)
	w, h := Size(v)
	p := painter{c: raster.New(w, h), g: g}
	outline := PillBounds(v)
	if v.Open {
		outline = g.rect(0, 0, g.w, g.h)
	}
	if v.Floating {
		radius := p.step(seamContourRadius(v))
		p.c.ShadowRound(outline, radius, p.step(10), p.step(3))
		p.c.FillRound(outline.Min.X, outline.Min.Y, outline.Dx(), outline.Dy(), radius, color.RGBA{A: 255})
	} else {
		body := outline
		body.Min.X += p.step(seamWing)
		body.Max.X -= p.step(seamWing)
		p.c.ShadowRound(body, p.step(seamRadius), p.step(10), p.step(3))
		p.c.FillNotch(outline, p.step(seamWing), p.step(seamRadius), v.Above, color.RGBA{A: 255})
	}
	if !v.Open {
		p.drawSeamRail(v, PillBounds(v))
		return p.c.Img
	}
	p.draw(g.rect(24, 8, 0, 0).Min.X, g.rect(24, 8, 0, 0).Min.Y, "Used", detailFont, dimInk)
	p.seamButton(PinBounds(v), "pin", v.Hovered.Control == PinControl, v.Pinned)
	p.seamButton(CloseBounds(v), "close", v.Hovered.Control == CloseControl, false)
	if len(v.Items) == 0 {
		r := PanelBounds(v)
		p.center((r.Min.X+r.Max.X)/2, r.Min.Y+p.step(48), "No agents enabled", titleFont, whiteInk)
		p.center((r.Min.X+r.Max.X)/2, r.Min.Y+p.step(70), "Right-click to choose agents.", detailFont, dimInk)
	} else {
		selected, _ := seamSelected(v)
		for _, it := range v.Items {
			r := PillItemBounds(v, it.ID)
			if it.ID == selected.ID || v.Hovered == (Target{Control: AgentControl, ID: it.ID}) {
				p.c.FillRound(r.Min.X+p.step(2), r.Min.Y, r.Dx()-p.step(4), r.Dy(), p.step(8), color.RGBA{R: 16, G: 16, B: 16, A: 255})
			}
			extra := ringExtra(it)
			x := r.Min.X + (r.Dx()-p.step(53+extra))/2
			p.ring(it, x, r.Min.Y+p.step(6), p.step(26))
			s, pct := usage(it)
			if pct {
				s += "%"
			}
			p.draw(x+p.step(29+extra), r.Min.Y+p.step(12), s, numberFont, valueInk(it))
			mid := (r.Min.X + r.Max.X) / 2
			p.center(mid, r.Min.Y+p.step(39), quota.ItemShort(it.ID), text.Style{Size: 10.5, Weight: 500}, whiteInk)
			p.center(mid, r.Min.Y+p.step(54), text.Elide(seamBasis(it), captionFont, float32(r.Dx()-p.step(4))/g.scale), captionFont, mutedInk)
		}
	}
	p.drawSelectedDetails(v)
	if seamGroupHeight(v) > 0 {
		it, _ := seamSelected(v)
		at := g.rect(24, seamHeader+seamCells+selectedDetailLayout(v, g.w).height+5, 0, 0).Min
		p.drawGroups(it, at.X, at.Y, g.rect(g.w-24, 0, 0, 0).Min.X)
	}
	p.drawSeamFooter(v)
	return p.c.Img
}

func (p painter) drawSeamRail(v View, r image.Rectangle) {
	if len(v.Items) == 0 {
		p.draw(r.Min.X+p.step(16), r.Min.Y+p.step(15), "No agents enabled", detailFont, dimInk)
		return
	}
	items := v.Items
	if v.Compact {
		it, _ := seamSelected(v)
		items = []Item{it}
	}
	for _, it := range items {
		b := PillItemBounds(v, it.ID)
		if v.Compact {
			b = image.Rect(r.Min.X+p.step(10), r.Min.Y, r.Max.X-p.step(22), r.Max.Y)
		}
		p.ring(it, b.Min.X, r.Min.Y+p.step(8), p.step(28))
		x, y := b.Min.X+p.step(31+ringExtra(it)), r.Min.Y+p.step(15)
		if it.PillReset != "" {
			y = r.Min.Y + p.step(7)
		}
		s, has := usage(it)
		if has {
			s += "%"
		}
		p.draw(x, y, s, numberFont, valueInk(it))
		if it.PillReset != "" {
			// This is explicitly the five-hour timer, even when the headline
			// percentage belongs to the week window.
			st := text.Style{Size: 8, Weight: 500}
			p.draw(x, r.Min.Y+p.step(25), text.Elide(seamPillReset(it.PillReset), st, 37), st, mutedInk)
		}
	}
	p.icon("down", image.Rect(r.Max.X-p.step(27), r.Min.Y+p.step(16), r.Max.X-p.step(15), r.Min.Y+p.step(28)), mutedInk)
}

func (p painter) drawSeamFooter(v View) {
	g := p.g
	y := g.h - seamFooterHeight(v)
	left := g.rect(24, y, 0, 0).Min
	p.c.Rect(left.X, left.Y, g.px(g.w-48), max(1, g.px(1)), color.RGBA{R: 24, G: 24, B: 24, A: 255})
	if it, ok := seamSelected(v); ok {
		for _, control := range []struct {
			kind  Control
			label string
		}{
			{GroupsControl, fmt.Sprintf("%d groups", len(it.Groups))},
			{ResetControl, "5h timer"},
			{RecoveryControl, "How to recover ›"},
		} {
			r := seamDetailTarget(v, it.ID, control.kind)
			if r.Empty() {
				continue
			}
			label := control.label
			if control.kind == ResetControl {
				label = resetToggleLabel(it.Reset)
			}
			if control.kind == RecoveryControl && v.RecoveryOpen == it.ID {
				label = "Back to error ‹"
			}
			if v.Hovered == (Target{Control: control.kind, ID: it.ID}) {
				p.c.FillRound(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), p.step(5), color.RGBA{R: 16, G: 16, B: 16, A: 255})
			}
			p.draw(r.Min.X+p.step(3), r.Min.Y+p.step(6), label, captionFont, dimInk)
			if control.kind == GroupsControl {
				p.icon("down", image.Rect(r.Max.X-p.step(15), r.Min.Y+p.step(7), r.Max.X-p.step(5), r.Min.Y+p.step(17)), mutedInk)
			}
		}
	}
	r := RefreshBounds(v)
	if v.Hovered.Control == RefreshControl {
		p.c.FillRound(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), p.step(5), color.RGBA{R: 16, G: 16, B: 16, A: 255})
	}
	p.icon("refresh", image.Rect(r.Min.X+p.step(3), r.Min.Y+p.step(7), r.Min.X+p.step(15), r.Min.Y+p.step(19)), mutedInk)
	status := strings.TrimPrefix(v.Checked, "Checked ")
	if status == "just now" {
		status = "Just checked"
	}
	if status == "" {
		status = "Not checked"
	}
	if v.Busy {
		status = "Checking…"
	}
	p.draw(r.Min.X+p.step(22), r.Min.Y+p.step(7), text.Elide(status, captionFont, 80), captionFont, mutedInk)
}
func (p painter) seamButton(r image.Rectangle, name string, hover, selected bool) {
	if hover || selected {
		p.c.FillRound(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), p.step(5), color.RGBA{R: 20, G: 20, B: 20, A: 255})
	}
	ink := dimInk
	if hover || selected {
		ink = whiteInk
	}
	p.icon(name, r.Inset(p.step(3)), ink)
}

func resetToggleLabel(reset string) string {
	if strings.Contains(reset, " in ") || strings.HasSuffix(reset, " now") {
		return "Countdown"
	}
	return "Clock"
}

func seamPillReset(s string) string {
	// HhMM keeps every minute digit in the narrow rail slot; clock times and
	// sub-hour countdowns retain their original representation.
	if at := strings.IndexByte(s, 'h'); at >= 0 && strings.HasSuffix(s, "m") {
		minutes := strings.TrimSuffix(s[at+1:], "m")
		if len(minutes) == 1 {
			minutes = "0" + minutes
		}
		s = s[:at+1] + minutes
	}
	return "5h " + s
}
