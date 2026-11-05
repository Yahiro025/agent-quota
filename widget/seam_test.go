package widget

import (
	"bytes"
	"image"
	"image/color"
	"strings"
	"testing"

	"agent-quota/internal/text"
	"agent-quota/quota"
)

func seamFixture() View {
	return View{Seam: true, Open: true, Selected: quota.ItemClaude, Items: []Item{
		{ID: quota.ItemClaude, State: quota.OK, Used: 70, Windows: []quota.Window{{Label: "5h", Used: 19}, {Label: "week", Used: 70}}, Reset: "5h resets in 12m", PillReset: "12m"},
		{ID: quota.ItemDevin, State: quota.Failed, Err: "Sign-in expired", Help: "Open Devin and sign in, then refresh."},
		{ID: quota.ItemAntigravity, State: quota.OK, Used: 10, Reset: "5h resets in 4h 59m", Groups: []quota.Group{{Name: "Gemini", Windows: []quota.Window{{Label: "week", Used: 2}}}, {Name: "Claude / GPT", Windows: []quota.Window{{Label: "5h", Used: 0}, {Label: "week", Used: 10}}}}},
	}}
}

func TestSeamMatteSurfaceAndShoulderHitRegion(t *testing.T) {
	v := seamFixture()
	g := layout(v)
	img := Render(v)
	r := PanelBounds(v)
	p := image.Pt((r.Min.X+r.Max.X)/2, r.Min.Y+g.px(22))
	if got := img.RGBAAt(p.X, p.Y); got != (color.RGBA{A: 255}) {
		t.Fatalf("matte reading surface=%v, want opaque pure black", got)
	}
	outside := image.Pt(g.rect(0, 0, 0, 0).Min.X+1, g.px(24))
	if Contains(v, outside) || Hit(v, outside).Control != NoControl {
		t.Fatal("inset side/shadow intercepts underlying desktop")
	}
	if !Contains(v, p) {
		t.Fatal("solid body rejects pointer input")
	}
	if PillBounds(v).Min.Y != 0 {
		t.Fatal("top-attached surface has a transparent gap above it")
	}
}

func TestSeamIndependentControlsAtFractionalDPI(t *testing.T) {
	center := func(r image.Rectangle) image.Point { return image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2) }
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		for _, above := range []bool{false, true} {
			for _, id := range []string{quota.ItemClaude, quota.ItemDevin, quota.ItemAntigravity} {
				v := seamFixture()
				v.Scale = scale
				v.Above = above
				v.Selected = id
				v.GroupsOpen = true
				targets := []struct {
					r    image.Rectangle
					want Target
				}{
					{RefreshBounds(v), Target{Control: RefreshControl}},
					{PinBounds(v), Target{Control: PinControl}},
					{CloseBounds(v), Target{Control: CloseControl}},
					{PillItemBounds(v, id), Target{Control: AgentControl, ID: id}},
				}
				if id == quota.ItemClaude {
					targets = append(targets, struct {
						r    image.Rectangle
						want Target
					}{ResetRowBounds(v, id), Target{Control: ResetControl, ID: id}})
				}
				if id == quota.ItemDevin {
					targets = append(targets, struct {
						r    image.Rectangle
						want Target
					}{RecoveryBounds(v, id), Target{Control: RecoveryControl, ID: id}})
				}
				if id == quota.ItemAntigravity {
					targets = append(targets, struct {
						r    image.Rectangle
						want Target
					}{GroupsBounds(v, id), Target{Control: GroupsControl, ID: id}}, struct {
						r    image.Rectangle
						want Target
					}{ResetRowBounds(v, id), Target{Control: ResetControl, ID: id}})
				}
				for _, tc := range targets {
					if tc.r.Empty() {
						t.Fatalf("empty %v", tc.want)
					}
					p := center(tc.r)
					if got := Hit(v, p); got != tc.want {
						t.Fatalf("scale=%v above=%v selected=%s got %v want %v", scale, above, id, got, tc.want)
					}
					if CanDrag(v, p) {
						t.Fatalf("control %v starts drag", tc.want)
					}
				}
			}
		}
	}
}

func TestSeamResetMeaningAndMissingGroupWindows(t *testing.T) {
	v := seamFixture()
	it := v.Items[0]
	if got := seamBasis(it); got != "5h" {
		t.Fatalf("primary basis=%q", got)
	}
	if got := seamDetail(it); !strings.Contains(got, "5h 19%") || !strings.Contains(got, "Week 70%") || !strings.Contains(got, "5h resets in 12m") {
		t.Fatalf("detail must retain both windows and qualify the five-hour reset: %q", got)
	}
	v.Selected = quota.ItemAntigravity
	v.GroupsOpen = true
	before := len(v.Items[2].Groups[0].Windows)
	Render(v)
	if len(v.Items[2].Groups[0].Windows) != before {
		t.Fatal("render invented a missing quota window")
	}
}

func TestSeamCodexWeeklyUsageAndResetRemainVisible(t *testing.T) {
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		for _, full := range []bool{false, true} {
			for _, change := range []string{"weekly", "countdown", "clock"} {
				it := Item{ID: quota.ItemCodex, State: quota.OK, Used: 68,
					Windows: []quota.Window{{Label: "5h", Used: 68}, {Label: "week", Used: 37}},
					Reset:   "5h resets in 33m"}
				if change == "clock" {
					it.Reset = "5h resets at 23:59"
				}
				v := View{Seam: true, Open: true, Scale: scale, Selected: quota.ItemCodex, Items: []Item{it}}
				if full {
					for _, id := range []string{quota.ItemClaude, quota.ItemKiro, quota.ItemCursor, quota.ItemCopilot, quota.ItemDevin, quota.ItemAntigravity} {
						v.Items = append(v.Items, Item{ID: id, State: quota.OK})
					}
				}
				before := Render(v)
				switch change {
				case "weekly":
					v.Items[0].Windows[1].Used = 38
				case "countdown":
					v.Items[0].Reset = "5h resets in 34m"
				case "clock":
					v.Items[0].Reset = "5h resets at 23:58"
				}
				// The headline remains 5h 68%; only the selected detail changes.
				if after := Render(v); bytes.Equal(before.Pix, after.Pix) {
					t.Errorf("scale=%v full=%v: %s value is hidden from the overview", scale, full, change)
				}
			}
		}
	}
}

func TestSeamBusyRefreshAndCompactAnchor(t *testing.T) {
	v := seamFixture()
	v.Busy = true
	r := RefreshBounds(v)
	if Hit(v, image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)).Control != NoControl {
		t.Fatal("busy refresh accepts duplicate action")
	}
	v.Compact = true
	open := PillBounds(v)
	v.Open = false
	closed := PillBounds(v)
	if open.Dx() != 118 || closed.Dx() != 118 {
		t.Fatal("compact anchor changes width on reveal")
	}
}

func TestSeamFiveHourCaptionFitsWithoutHidingMinutes(t *testing.T) {
	for _, input := range []string{"4h59m", "4h0m", "23:59", "12m"} {
		s := seamPillReset(input)
		if width := text.Measure(s, text.Style{Size: 8, Weight: 500}); width > 37 {
			t.Fatalf("%q needs %.2fpx; caption has37", s, width)
		}
	}
}

func TestSeamFloatingCompactCornersAreClickThrough(t *testing.T) {
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		v := seamFixture()
		v.Open, v.Compact, v.Floating = false, true, true
		v.Scale = scale
		g := layout(v)
		r := PillBounds(v)
		p := r.Min.Add(image.Pt(g.px(8), g.px(1)))
		if Contains(v, p) || Hit(v, p).Control != NoControl || CanDrag(v, p) {
			t.Fatalf("scale=%v transparent floating corner intercepts input", scale)
		}
		img := Render(v)
		if img.RGBAAt(p.X, p.Y).A >= 128 {
			t.Fatalf("scale=%v floating corner is opaque", scale)
		}
		middle := image.Pt((r.Min.X+r.Max.X)/2, r.Min.Y+g.px(3))
		if !Contains(v, middle) || img.RGBAAt(middle.X, middle.Y) != (color.RGBA{A: 255}) {
			t.Fatalf("scale=%v floating surface lost its opaque matte material", scale)
		}
		if r.Min.Y != g.px(16) {
			t.Fatalf("scale=%v floating shadow lacks space above the rail", scale)
		}
	}
}
