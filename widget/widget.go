package widget

import (
	"image"
	"image/color"
	"math"
	"strings"

	"agent-quota/internal/text"
	"agent-quota/quota"
)

type Item struct {
	ID                 string
	State              quota.State
	Used               float64
	HasPct             bool
	Detail, Err, Stale string
	// Both labels are formatted from Reading.Reset5h upstream; rendering has no clock.
	Reset, PillReset          string
	Plan, Summary, ResetLabel string
	Windows                   []quota.Window
	Groups                    []quota.Group
	Help                      string
	Details                   []Detail
	Observed                  string
}

type Detail struct {
	Label, Value, Reset string
}

type Control uint8

const (
	NoControl Control = iota
	RefreshControl
	PinControl
	CloseControl
	ResetControl
	GroupsControl
	RecoveryControl
	AgentControl
	OpenControl
)

type Target struct {
	Control Control
	ID      string
}

// View is display state only. Geometry, hit targets and pixels share one layout.
type View struct {
	Items                        []Item
	Open, Above, Pinned, Compact bool
	Checked                      string
	Scale                        float32
	Selected, RecoveryOpen       string
	GroupsOpen                   bool
	Busy                         bool
	Seam                         bool
	Floating                     bool
	Focus, DockPreview           bool
	FocusScroll, FocusMaxHeight  int
	Hovered                      Target
}

const (
	pad       = 14
	gap       = 6
	cell      = 28
	pctGap    = 3
	PctWidth  = 31
	pillH     = 44
	panelW    = 386
	panelGap  = 10
	headerH   = 49
	rowH      = 56
	footerH   = 49
	shadowPad = 16
	openW     = 30
)

var (
	pillInk     = color.RGBA{8, 8, 10, 255}
	panelInk    = color.RGBA{25, 23, 29, 255}
	edgeInk     = color.RGBA{255, 255, 255, 0x14}
	whiteInk    = color.NRGBA{244, 242, 246, 255}
	dimInk      = color.NRGBA{185, 179, 193, 255}
	mutedInk    = color.NRGBA{161, 154, 169, 255}
	errorInk    = color.NRGBA{244, 178, 171, 255}
	staleInk    = color.NRGBA{237, 194, 139, 255}
	numberFont  = text.Style{Size: 11.5, Weight: 600}
	percentFont = text.Style{Size: 9, Weight: 500}
	titleFont   = text.Style{Size: 12.5, Weight: 500}
	detailFont  = text.Style{Size: 11, Weight: 400}
	resetFont   = text.Style{Size: 9, Weight: 500}
	captionFont = text.Style{Size: 10, Weight: 400}
)

func PillSize(n, pctW int) (int, int) {
	if n == 0 {
		return 152, pillH
	}
	return 2*pad + n*(cell+pctGap+pctW) + (n-1)*gap + openW, pillH
}

type geometry struct {
	w, h, pw, ph, panelY, panelH int
	scale                        float32
	topPad                       int
}

func itemHeight(v View, it Item) int {
	h := rowH
	if it.State == quota.Stale || it.State == quota.Failed {
		h += 14
	}
	if v.GroupsOpen && len(it.Groups) > 0 && (it.State == quota.OK || it.State == quota.Stale) {
		h += 18 + len(it.Groups)*22
	}
	return h
}

func layout(v View) geometry {
	if v.Focus {
		return focusLayout(v).geometry
	}
	if v.Seam {
		return seamLayout(v)
	}
	scale := v.Scale
	if scale <= 0 || math.IsNaN(float64(scale)) || math.IsInf(float64(scale), 0) {
		scale = 1
	}
	pw, ph := viewPillSize(v)
	g := geometry{w: pw, h: ph, pw: pw, ph: ph, scale: scale, topPad: shadowPad}
	if v.Open {
		g.panelH = headerH + footerH
		if len(v.Items) == 0 {
			g.panelH += 180
		} else {
			for _, it := range v.Items {
				g.panelH += itemHeight(v, it)
			}
		}
		g.w = max(pw, panelW)
		g.panelY = ph + panelGap
		g.h += panelGap + g.panelH
		if v.Above {
			g.panelY = 0
		}
	}
	return g
}

func (g geometry) px(n int) int { return int(math.Round(float64(float32(n) * g.scale))) }
func (g geometry) rect(x, y, w, h int) image.Rectangle {
	return image.Rect(g.px(x+shadowPad), g.px(y+g.topPad), g.px(x+w+shadowPad), g.px(y+h+g.topPad))
}
func Size(v View) (int, int) {
	g := layout(v)
	bottom := shadowPad
	if v.Seam && v.Above && !v.Floating {
		bottom = 0
	}
	return g.px(g.w + 2*shadowPad), g.px(g.h + g.topPad + bottom)
}
func PillBounds(v View) image.Rectangle {
	if v.Focus {
		return focusPillBounds(v)
	}
	if v.Seam {
		return seamPillBounds(v)
	}
	g := layout(v)
	y := 0
	if v.Open && v.Above {
		y = g.h - g.ph
	}
	return g.rect(g.w-g.pw, y, g.pw, g.ph)
}
func PanelBounds(v View) image.Rectangle {
	if v.Focus {
		if !v.Open {
			return image.Rectangle{}
		}
		f := focusLayout(v)
		return f.rect(0, 0, f.w, f.panelH)
	}
	if v.Seam {
		return seamPanelBounds(v)
	}
	if !v.Open {
		return image.Rectangle{}
	}
	g := layout(v)
	return g.rect(g.w-panelW, g.panelY, panelW, g.panelH)
}
func RefreshBounds(v View) image.Rectangle {
	if v.Focus {
		return focusControl(v, RefreshControl)
	}
	if v.Seam {
		return seamRefreshBounds(v)
	}
	if !v.Open {
		return image.Rectangle{}
	}
	g := layout(v)
	return g.rect(g.w-95, g.panelY+g.panelH-footerH+11, 79, 27)
}
func PinBounds(v View) image.Rectangle {
	if v.Focus {
		return focusControl(v, PinControl)
	}
	if v.Seam {
		return seamHeaderControl(v, false)
	}
	if !v.Open {
		return image.Rectangle{}
	}
	g := layout(v)
	return g.rect(g.w-69, g.panelY+10, 28, 28)
}
func CloseBounds(v View) image.Rectangle {
	if v.Focus {
		return focusControl(v, CloseControl)
	}
	if v.Seam {
		return seamHeaderControl(v, true)
	}
	if !v.Open {
		return image.Rectangle{}
	}
	g := layout(v)
	return g.rect(g.w-40, g.panelY+10, 28, 28)
}

func rowBounds(v View, id string) image.Rectangle {
	if !v.Open {
		return image.Rectangle{}
	}
	g := layout(v)
	y := g.panelY + headerH
	for _, it := range v.Items {
		h := itemHeight(v, it)
		if it.ID == id {
			return g.rect(g.w-panelW, y, panelW, h)
		}
		y += h
	}
	return image.Rectangle{}
}

func ResetRowBounds(v View, id string) image.Rectangle {
	if v.Focus {
		return focusDetailTarget(v, id, ResetControl)
	}
	if v.Seam {
		return seamDetailTarget(v, id, ResetControl)
	}
	for _, it := range v.Items {
		if it.ID == id && it.Reset != "" {
			return rowBounds(v, id)
		}
	}
	return image.Rectangle{}
}
func GroupsBounds(v View, id string) image.Rectangle {
	if v.Focus {
		return focusDetailTarget(v, id, GroupsControl)
	}
	if v.Seam {
		return seamDetailTarget(v, id, GroupsControl)
	}
	for _, it := range v.Items {
		if it.ID == id && len(it.Groups) > 0 && (it.State == quota.OK || it.State == quota.Stale) {
			r := rowBounds(v, id)
			if r.Empty() {
				break
			}
			g := layout(v)
			return image.Rect(r.Min.X+g.px(54), r.Min.Y+g.px(25), r.Min.X+g.px(193), r.Min.Y+g.px(45))
		}
	}
	return image.Rectangle{}
}
func RecoveryBounds(v View, id string) image.Rectangle {
	if v.Focus {
		return focusDetailTarget(v, id, RecoveryControl)
	}
	if v.Seam {
		return seamDetailTarget(v, id, RecoveryControl)
	}
	for _, it := range v.Items {
		if it.ID == id && it.State == quota.Failed && it.Help != "" {
			r := rowBounds(v, id)
			if r.Empty() {
				break
			}
			g := layout(v)
			return image.Rect(r.Min.X+g.px(54), r.Min.Y+g.px(52), r.Min.X+g.px(180), r.Min.Y+g.px(68))
		}
	}
	return image.Rectangle{}
}
func PillItemBounds(v View, id string) image.Rectangle {
	if v.Focus {
		return focusAgentBounds(v, id)
	}
	if v.Seam {
		return seamItemBounds(v, id)
	}
	g := layout(v)
	r := PillBounds(v)
	if v.Compact {
		return image.Rectangle{}
	}
	x := pad
	for _, it := range v.Items {
		width := cell + pctGap + PctWidth + ringExtra(it)
		if it.ID == id {
			return image.Rect(r.Min.X+g.px(x), r.Min.Y+g.px(5), r.Min.X+g.px(x+width), r.Max.Y-g.px(5))
		}
		x += width + gap
	}
	return image.Rectangle{}
}

func Hit(v View, p image.Point) Target {
	if v.Focus {
		return focusHit(v, p)
	}
	if v.Seam && !seamContains(v, p) {
		return Target{}
	}
	for _, control := range []struct {
		bounds image.Rectangle
		kind   Control
	}{{RefreshBounds(v), RefreshControl}, {PinBounds(v), PinControl}, {CloseBounds(v), CloseControl}} {
		if control.kind == RefreshControl && v.Busy {
			continue
		}
		if p.In(control.bounds) {
			return Target{Control: control.kind}
		}
	}
	for _, it := range v.Items {
		for _, control := range []struct {
			bounds image.Rectangle
			kind   Control
		}{{GroupsBounds(v, it.ID), GroupsControl}, {RecoveryBounds(v, it.ID), RecoveryControl}, {ResetRowBounds(v, it.ID), ResetControl}, {PillItemBounds(v, it.ID), AgentControl}} {
			if p.In(control.bounds) {
				return Target{Control: control.kind, ID: it.ID}
			}
		}
	}
	if p.In(PillBounds(v)) && (!v.Seam || !v.Open || p.Y < layout(v).rect(0, 0, layout(v).w, seamHeader).Max.Y) {
		return Target{Control: OpenControl}
	}
	return Target{}
}

// The transparent bridge is interactive so crossing the ten-pixel gap does not
// arm leave/close. The shadow outside this bridge remains click-through.
func Contains(v View, p image.Point) bool {
	if v.Focus {
		return focusContains(v, p)
	}
	if v.Seam {
		return seamContains(v, p)
	}
	if p.In(PillBounds(v)) || p.In(PanelBounds(v)) {
		return true
	}
	if !v.Open {
		return false
	}
	a, b := PillBounds(v), PanelBounds(v)
	bridge := image.Rect(max(a.Min.X, b.Min.X), min(a.Max.Y, b.Max.Y), min(a.Max.X, b.Max.X), max(a.Min.Y, b.Min.Y))
	return p.In(bridge)
}

func CanDrag(v View, p image.Point) bool {
	if v.Focus {
		return p.In(PillBounds(v))
	}
	if v.Seam {
		if !seamContains(v, p) {
			return false
		}
		if !v.Open {
			return true
		}
		g := layout(v)
		control := Hit(v, p).Control
		return p.Y < g.rect(0, 0, g.w, seamHeader).Max.Y && (control == NoControl || control == OpenControl)
	}
	if p.In(PillBounds(v)) {
		return true
	}
	panel := PanelBounds(v)
	return p.In(panel) && p.Y < panel.Min.Y+layout(v).px(headerH) && Hit(v, p).Control == NoControl
}

func usage(it Item) (string, bool) {
	if it.State == quota.OK || it.State == quota.Stale {
		return quota.Custom(headlineUsed(it), 0), true
	}
	return "—", false
}
func detailLines(s string, maxW float32, style text.Style) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	line := words[0]
	for i := 1; i < len(words); i++ {
		if text.Measure(line+" "+words[i], style) > maxW {
			return []string{text.Elide(line, style, maxW), text.Elide(strings.Join(words[i:], " "), style, maxW)}
		}
		line += " " + words[i]
	}
	return []string{text.Elide(line, style, maxW)}
}
func metrics(it Item) string {
	if it.Summary != "" {
		return it.Summary
	}
	var parts []string
	for _, w := range it.Windows {
		label := w.Label
		if label == "week" {
			label = "Week"
		} else if label == "day" {
			label = "Day"
		}
		parts = append(parts, label+" "+quota.Custom(w.Used, 0)+"%")
	}
	if len(parts) > 0 {
		return strings.Join(parts, "   ")
	}
	return it.Detail
}
