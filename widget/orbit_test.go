package widget

import (
	"image"
	"testing"

	"agent-quota/quota"
)

func TestOrbitInspectorKeepsItsOwnWidth(t *testing.T) {
	v := View{Open: true, Items: make([]Item, 7)}
	img := Render(v)
	y := PillBounds(v).Max.Y + panelGap + 20
	opaque := 0
	for x := 0; x < img.Bounds().Dx(); x++ {
		if img.RGBAAt(x, y).A == 255 {
			opaque++
		}
	}
	if opaque != 386 {
		t.Fatalf("inspector reading surface is %d px wide, want 386 independent of the rail", opaque)
	}
}

func TestOrbitControlsStayDistinctAtPhysicalDPI(t *testing.T) {
	v := View{Open: true, Items: []Item{
		{ID: "claude", State: quota.OK, Reset: "5h resets in 12m"},
		{ID: "antigravity", State: quota.OK, Reset: "5h resets in 4h 59m", Groups: []quota.Group{{Name: "Gemini"}, {Name: "Claude / GPT"}}},
		{ID: "devin", State: quota.Failed, Help: "Open Devin and sign in, then refresh."},
	}, GroupsOpen: true}
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		for _, above := range []bool{false, true} {
			v.Scale, v.Above = scale, above
			for _, tc := range []struct {
				r    image.Rectangle
				want Target
			}{
				{RefreshBounds(v), Target{Control: RefreshControl}},
				{PinBounds(v), Target{Control: PinControl}},
				{CloseBounds(v), Target{Control: CloseControl}},
				{GroupsBounds(v, "antigravity"), Target{Control: GroupsControl, ID: "antigravity"}},
				{RecoveryBounds(v, "devin"), Target{Control: RecoveryControl, ID: "devin"}},
				{ResetRowBounds(v, "claude"), Target{Control: ResetControl, ID: "claude"}},
			} {
				if tc.r.Empty() {
					t.Fatalf("%v at scale %v is empty", tc.want, scale)
				}
				point := image.Pt((tc.r.Min.X+tc.r.Max.X)/2, (tc.r.Min.Y+tc.r.Max.Y)/2)
				if got := Hit(v, point); got != tc.want {
					t.Fatalf("scale %v above=%v click %v = %v, want %v", scale, above, point, got, tc.want)
				}
				if CanDrag(v, point) {
					t.Fatalf("control %v starts a window drag at scale %v", tc.want, scale)
				}
			}
		}
	}
}

func TestOrbitShadowAndBusyButtonDoNotClaimActions(t *testing.T) {
	v := View{Open: true, Busy: true, Items: []Item{{ID: "claude", State: quota.OK}}}
	r := RefreshBounds(v)
	if got := Hit(v, image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)); got.Control != NoControl {
		t.Fatalf("busy Refresh is still clickable: %v", got)
	}
	if Contains(v, image.Pt(1, 1)) {
		t.Fatal("transparent shadow margin claims pointer input")
	}
	pill, panel := PillBounds(v), PanelBounds(v)
	if !Contains(v, image.Pt((pill.Min.X+pill.Max.X)/2, (pill.Max.Y+panel.Min.Y)/2)) {
		t.Fatal("crossing from rail to inspector leaves the widget")
	}
	v.Compact = true
	if w := PillBounds(v).Dx(); w != 112 {
		t.Fatalf("compact tab width = %d, want 112", w)
	}
	if w := PanelBounds(v).Dx(); w != 386 {
		t.Fatalf("compact inspector width = %d, want 386", w)
	}
}
