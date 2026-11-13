package widget

import (
	"image"
	"math"
	"testing"

	"agent-quota/quota"
)

func TestFiveHourHeadlineDoesNotFollowWeeklyMaximum(t *testing.T) {
	it := Item{ID: "claude", State: quota.OK, Used: 95, Windows: []quota.Window{{Label: "5h", Used: 20}, {Label: "week", Used: 95}}}
	if s, ok := usage(it); !ok || s != "20" {
		t.Fatalf("headline = %q, %v; want 20%% five-hour usage", s, ok)
	}
	if seamBasis(it) != "5h" || valueInk(it) != whiteInk {
		t.Fatal("headline basis or color belongs to the weekly pool")
	}
	it.Windows[0].Used = 0
	if s, _ := usage(it); s != "0" {
		t.Fatalf("reported zero was replaced by weekly usage: %q", s)
	}
}

func TestDualPoolRingsAreIndependentAtPhysicalDPI(t *testing.T) {
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		for _, compact := range []bool{false, true} {
			it := Item{ID: "claude", State: quota.OK, Used: 95, Windows: []quota.Window{{Label: "5h", Used: 20}, {Label: "week", Used: 95}}}
			v := View{Seam: true, Scale: scale, Compact: compact, Items: []Item{it}}
			img := Render(v)
			b := PillItemBounds(v, it.ID)
			if compact {
				b = PillBounds(v)
				b.Min.X += int(math.Round(10 * float64(scale)))
			}
			cx := float64(b.Min.X) + 17*float64(scale)
			cy := float64(PillBounds(v).Min.Y) + 22*float64(scale)
			pixel := func(radius, angle float64) (r, g uint8) {
				x := int(math.Round(cx + radius*float64(scale)*math.Cos(angle)))
				y := int(math.Round(cy + radius*float64(scale)*math.Sin(angle)))
				c := img.RGBAAt(x, y)
				return c.R, c.G
			}
			r, g := pixel(14.9, 0)
			if r < 140 || g > 110 {
				t.Fatalf("scale %v compact %v: weekly outer arc is not red: %d,%d", scale, compact, r, g)
			}
			r, g = pixel(11.9, -math.Pi/3)
			if g < 100 || r > 100 {
				t.Fatalf("scale %v compact %v: five-hour inner arc is not green: %d,%d", scale, compact, r, g)
			}
			it.State = quota.Stale
			v.Items[0] = it
			stale := Render(v)
			p := image.Pt(int(math.Round(cx+14.9*float64(scale))), int(math.Round(cy)))
			if stale.RGBAAt(p.X, p.Y).R >= img.RGBAAt(p.X, p.Y).R {
				t.Fatal("stale weekly ring did not dim")
			}
		}
	}
}

func TestOnlyDualPoolSlotsGrow(t *testing.T) {
	v := View{Seam: true, Items: []Item{{ID: "claude", State: quota.OK, Used: 10}, {ID: "kiro", State: quota.OK, Used: 20}}}
	before := PillBounds(v).Dx()
	single := PillItemBounds(v, "kiro").Dx()
	v.Items[0].Windows = []quota.Window{{Label: "5h", Used: 10}, {Label: "week", Used: 80}}
	if got := PillBounds(v).Dx(); got != before+6 {
		t.Fatalf("rail width = %d, want %d", got, before+6)
	}
	if got := PillItemBounds(v, "kiro").Dx(); got != single {
		t.Fatalf("single-pool slot changed from %d to %d", single, got)
	}
	for _, it := range v.Items {
		r := PillItemBounds(v, it.ID)
		p := image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
		if got := Hit(v, p); got != (Target{Control: AgentControl, ID: it.ID}) {
			t.Fatalf("widened rail hit = %v", got)
		}
	}
}

func TestGroupedPoolsUseReportedWindowMaximumWithoutInventingMissingPools(t *testing.T) {
	it := Item{ID: "antigravity", State: quota.OK, Used: 95, Groups: []quota.Group{
		{Name: "Gemini", Windows: []quota.Window{{Label: "5h", Used: 10}, {Label: "week", Used: 95}}},
		{Name: "Claude", Windows: []quota.Window{{Label: "5h", Used: 40}, {Label: "week", Used: 60}}},
	}}
	if s, _ := usage(it); s != "40" {
		t.Fatalf("grouped five-hour headline = %q, want 40", s)
	}
	if w := poolWindow(it, "week"); w == nil || w.Used != 95 {
		t.Fatalf("weekly maximum = %v, want 95", w)
	}
	it.Groups[0].Windows = it.Groups[0].Windows[:1]
	it.Groups[1].Windows = it.Groups[1].Windows[:1]
	if ringExtra(it) != 0 || poolWindow(it, "week") != nil {
		t.Fatal("a missing weekly pool was invented")
	}
	it.Windows = []quota.Window{{Label: "week", Used: 0}}
	if ringExtra(it) != 6 {
		t.Fatal("a reported zero weekly pool was treated as missing")
	}
	if len(it.Groups[0].Windows) != 1 || len(it.Groups[1].Windows) != 1 {
		t.Fatal("pool selection mutated source groups")
	}
}
