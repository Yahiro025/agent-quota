package widget

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"

	"agent-quota/internal/marks"
	"agent-quota/internal/raster"
	"agent-quota/internal/text"
	"agent-quota/quota"
)

const focusRail = 58

// RenderTaskbar leaves the shell-owned material transparent. The popup and
// floating pill still paint their own surfaces through Render.
func RenderTaskbar(v View) *image.RGBA {
	v = focusCompact(v)
	f := seamLayout(v)
	w, h := Size(v)
	p := painter{c: raster.New(w, h), g: f}
	r := PillBounds(v)
	p.drawSeamRail(v, r)
	img := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(img, img.Bounds(), p.c.Img, r.Min, draw.Src)
	return img
}

type focusGeometry struct {
	geometry
	details                         detailLayout
	contentY, bodyHeight, scrollMax int
}

func focusCompact(v View) View {
	v.Focus = false
	v.Seam = true
	v.Compact = true
	v.Floating = true
	v.Open = false
	v.Above = false
	return v
}

func focusLayout(v View) focusGeometry {
	g := seamLayout(focusCompact(v))
	f := focusGeometry{geometry: g}
	if !v.Open {
		return f
	}
	f.w = 468
	f.details = selectedDetailLayout(focusDetailsView(v), f.w-focusRail)
	f.contentY = 94
	it, ok := seamSelected(v)
	if ok && (poolWindow(it, "5h") != nil || poolWindow(it, "week") != nil) {
		f.contentY = 174
	}
	f.panelH = max(20+42*len(v.Items), f.contentY+f.details.height+48)
	f.panelH = max(240, f.panelH)
	if v.FocusMaxHeight > 0 {
		f.panelH = min(f.panelH, max(240, v.FocusMaxHeight))
	}
	f.bodyHeight = max(1, f.panelH-f.contentY-44)
	f.scrollMax = max(0, f.details.height-f.bodyHeight)
	f.h = f.panelH + 12 + f.ph
	return f
}

// FocusPopupBounds excludes the separately hosted readout, retaining its shadow.
func FocusPopupBounds(v View) image.Rectangle {
	if !v.Focus || !v.Open {
		return image.Rectangle{}
	}
	f := focusLayout(v)
	return image.Rect(0, 0, f.px(f.w+2*shadowPad), f.px(f.panelH+2*shadowPad))
}

func FocusScrollLimit(v View) int {
	if !v.Focus || !v.Open {
		return 0
	}
	return focusLayout(v).scrollMax
}

func focusPillBounds(v View) image.Rectangle {
	f := focusLayout(v)
	y := 0
	if v.Open {
		y = f.panelH + 12
	}
	return f.rect((f.w-f.pw)/2, y, f.pw, f.ph)
}

func focusAgentBounds(v View, id string) image.Rectangle {
	if !v.Open {
		return image.Rectangle{}
	}
	f := focusLayout(v)
	step := min(42, max(24, (f.panelH-20)/max(1, len(v.Items))))
	for i, it := range v.Items {
		if it.ID == id {
			return f.rect(7, 10+i*step, 44, step-2)
		}
	}
	return image.Rectangle{}
}

func focusControl(v View, kind Control) image.Rectangle {
	if !v.Open {
		return image.Rectangle{}
	}
	f := focusLayout(v)
	switch kind {
	case CloseControl:
		return f.rect(f.w-33, 8, 24, 24)
	case PinControl:
		return f.rect(f.w-61, 8, 24, 24)
	case RefreshControl:
		return f.rect(f.w-122, f.panelH-35, 106, 26)
	}
	return image.Rectangle{}
}

func focusDetailTarget(v View, id string, kind Control) image.Rectangle {
	if !v.Open || id != v.Selected {
		return image.Rectangle{}
	}
	it, ok := seamSelected(v)
	if !ok {
		return image.Rectangle{}
	}
	f := focusLayout(v)
	left := focusRail + 18
	if kind == GroupsControl && len(it.Details) == 0 && len(it.Groups) > 0 {
		return f.rect(left, f.panelH-35, 76, 26)
	}
	if len(it.Details) == 0 && len(it.Groups) > 0 {
		left += 80
	}
	if kind == ResetControl && it.Reset != "" {
		return f.rect(left, f.panelH-35, 70, 26)
	}
	if kind == RecoveryControl && it.State == quota.Failed && it.Help != "" {
		return f.rect(left, f.panelH-35, 140, 26)
	}
	return image.Rectangle{}
}

func focusContains(v View, p image.Point) bool {
	if p.In(PillBounds(v)) {
		return true
	}
	return v.Open && raster.RoundContains(PanelBounds(v), layout(v).px(14), p)
}

func focusHit(v View, p image.Point) Target {
	if !focusContains(v, p) {
		return Target{}
	}
	for _, kind := range []Control{CloseControl, PinControl, RefreshControl} {
		if kind == RefreshControl && v.Busy {
			continue
		}
		if p.In(focusControl(v, kind)) {
			return Target{Control: kind}
		}
	}
	for _, it := range v.Items {
		if p.In(focusAgentBounds(v, it.ID)) {
			return Target{Control: AgentControl, ID: it.ID}
		}
		for _, kind := range []Control{ResetControl, GroupsControl, RecoveryControl} {
			if p.In(focusDetailTarget(v, it.ID, kind)) {
				return Target{Control: kind, ID: it.ID}
			}
		}
	}
	if p.In(PillBounds(v)) {
		return Target{Control: OpenControl}
	}
	return Target{}
}

func renderFocus(v View) *image.RGBA {
	f := focusLayout(v)
	w, h := Size(v)
	p := painter{c: raster.New(w, h), g: f.geometry}
	pill := PillBounds(v)
	p.c.FillRound(pill.Min.X, pill.Min.Y, pill.Dx(), pill.Dy(), p.step(10), color.RGBA{A: 255})
	p.drawSeamRail(focusCompact(v), pill)
	if !v.Open {
		return p.c.Img
	}
	panel := PanelBounds(v)
	p.c.ShadowRound(panel, p.step(14), p.step(10), p.step(3))
	p.c.FillRound(panel.Min.X, panel.Min.Y, panel.Dx(), panel.Dy(), p.step(14), color.RGBA{A: 255})
	for _, it := range v.Items {
		r := focusAgentBounds(v, it.ID)
		if it.ID == v.Selected || v.Hovered == (Target{Control: AgentControl, ID: it.ID}) {
			p.c.FillRound(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), p.step(8), color.RGBA{R: 20, G: 20, B: 20, A: 255})
		}
		size := p.step(26)
		p.mark(it, r.Min.X+(r.Dx()-size)/2, r.Min.Y+(r.Dy()-size)/2, size)
	}
	line := f.rect(focusRail, 12, 1, f.panelH-24)
	p.c.Rect(line.Min.X, line.Min.Y, max(1, line.Dx()), line.Dy(), color.RGBA{R: 24, G: 24, B: 24, A: 255})
	if it, ok := seamSelected(v); ok {
		at := f.rect(focusRail+22, 22, 0, 0).Min
		p.ring(it, at.X, at.Y, p.step(44))
		title := f.rect(focusRail+82, 25, 0, 0).Min
		titleStyle := text.Style{Size: 17, Weight: 600}
		p.draw(title.X, title.Y, text.Elide(quota.ItemShort(it.ID), titleStyle, 230), titleStyle, whiteInk)
		caption := it.Plan
		if caption == "" {
			caption = seamBasis(it)
		}
		p.draw(title.X, title.Y+p.step(27), text.Elide(caption, captionFont, 230), captionFont, mutedInk)
		col := 0
		for _, label := range []string{"5h", "week"} {
			pool := poolWindow(it, label)
			if pool == nil {
				continue
			}
			at := f.rect(focusRail+24+col*177, 94, 0, 0).Min
			name := "5-hour usage"
			if label == "week" {
				name = "Weekly usage"
			}
			p.draw(at.X, at.Y, name, captionFont, mutedInk)
			value := fmt.Sprintf("%.0f%%", pool.Used)
			if math.IsNaN(pool.Used) || math.IsInf(pool.Used, 0) {
				value = "—"
			}
			p.draw(at.X, at.Y+p.step(20), value, text.Style{Size: 27, Weight: 600}, whiteInk)
			p.focusPoolBar(it, pool.Used, f.rect(focusRail+24+col*177, 148, 149, 4))
			col++
		}
		// Paint facts into their own viewport so scrolling cannot cover the rail or footer.
		body := raster.New(f.px(f.w-focusRail), f.px(max(f.bodyHeight, f.details.height)))
		scroll := max(0, min(v.FocusScroll, f.scrollMax))
		for i, l := range f.details.lines {
			if i == 0 {
				continue
			}
			p.paintDetailLine(body, f.px(24), f.px(l.y-12), body.Img.Bounds().Dx()-f.px(24), l, it.State == quota.Stale)
		}
		target := f.rect(focusRail, f.contentY, f.w-focusRail, f.bodyHeight)
		draw.Draw(p.c.Img, target, body.Img, image.Pt(0, f.px(scroll)), draw.Over)
		if f.scrollMax > 0 {
			track := f.rect(f.w-8, f.contentY, 2, f.bodyHeight)
			thumb := max(p.step(16), track.Dy()*f.bodyHeight/max(1, f.details.height))
			y := track.Min.Y + (track.Dy()-thumb)*scroll/max(1, f.scrollMax)
			p.c.FillRound(track.Min.X, y, track.Dx(), thumb, max(1, track.Dx()/2), color.RGBA{R: 64, G: 64, B: 64, A: 255})
		}
		for _, control := range []struct {
			kind  Control
			label string
		}{{ResetControl, "5h timer"}, {GroupsControl, "Groups"}, {RecoveryControl, "How to recover"}} {
			r := focusDetailTarget(v, it.ID, control.kind)
			if r.Empty() {
				continue
			}
			p.draw(r.Min.X+p.step(3), r.Min.Y+p.step(7), control.label, captionFont, dimInk)
		}
	} else {
		at := f.rect(focusRail+24, 40, 0, 0).Min
		p.draw(at.X, at.Y, "No agents enabled", titleFont, whiteInk)
	}
	for _, control := range []struct {
		kind Control
		icon string
	}{{CloseControl, "close"}, {PinControl, "pin"}} {
		p.button(focusControl(v, control.kind), control.icon, v.Hovered.Control == control.kind, control.kind == PinControl && v.Pinned)
	}
	footer := f.rect(focusRail+18, f.panelH-44, f.w-focusRail-36, 1)
	p.c.Rect(footer.Min.X, footer.Min.Y, footer.Dx(), max(1, footer.Dy()), color.RGBA{R: 24, G: 24, B: 24, A: 255})
	r := RefreshBounds(v)
	icon := image.Rect(r.Min.X, r.Min.Y, r.Min.X+p.step(24), r.Max.Y)
	p.button(icon, "refresh", v.Hovered.Control == RefreshControl, false)
	status := strings.TrimPrefix(v.Checked, "Checked ")
	if v.Busy {
		status = "Checking…"
	}
	if status == "" || text.Measure(status, captionFont) > 73 {
		status = "Refresh"
	}
	p.draw(r.Min.X+p.step(27), r.Min.Y+p.step(7), text.Elide(status, captionFont, 73), captionFont, mutedInk)
	return p.c.Img
}

func (p painter) focusPoolBar(it Item, used float64, r image.Rectangle) {
	p.c.FillRound(r.Min.X, r.Min.Y, r.Dx(), r.Dy(), p.step(2), color.RGBA{R: 40, G: 43, B: 48, A: 255})
	if (it.State != quota.OK && it.State != quota.Stale) || math.IsNaN(used) || math.IsInf(used, 0) {
		return
	}
	w := int(math.Round(float64(r.Dx()) * max(0, min(100, used)) / 100))
	if w == 0 {
		return
	}
	ink := marks.ForUsed(used)
	if it.State == quota.Stale {
		ink.A = 128
	}
	p.c.FillRound(r.Min.X, r.Min.Y, w, r.Dy(), p.step(2), ink)
}

func focusDetailsView(v View) View {
	if !v.GroupsOpen {
		return v
	}
	v.Items = append([]Item(nil), v.Items...)
	for i, it := range v.Items {
		if it.ID != v.Selected || len(it.Details) > 0 {
			continue
		}
		for _, group := range it.Groups {
			for _, window := range group.Windows {
				v.Items[i].Details = append(v.Items[i].Details, Detail{Label: group.Name + " ? " + window.Label, Value: fmt.Sprintf("%.0f%% used", window.Used)})
			}
		}
	}
	return v
}
