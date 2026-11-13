package widget

import (
	"image"
	"math"
	"strings"
	"testing"

	"agent-quota/internal/marks"
	"agent-quota/quota"
)

func focusFixture() View {
	v := View{Focus: true, Open: true, Selected: quota.ItemCodex, Scale: 1}
	for _, id := range quota.ItemAll {
		v.Items = append(v.Items, Item{ID: id, State: quota.OK, HasPct: true, Used: 42, Windows: []quota.Window{{Label: "5h", Used: 42}, {Label: "week", Used: 78}}, Reset: "5h resets in 1h 24m", PillReset: "1h24m"})
	}
	return v
}

func TestFocusPoolUsageBars(t *testing.T) {
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		v := View{Focus: true, Open: true, Scale: scale, Selected: quota.ItemClaude, Items: []Item{{ID: quota.ItemClaude, State: quota.OK, Windows: []quota.Window{{Label: "5h", Used: 25}, {Label: "week", Used: 95}}}}}
		at := func(column, offset int) image.Point {
			return PanelBounds(v).Min.Add(image.Pt(int(math.Round(float64(82+177*column+offset)*float64(scale))), int(math.Round(150*float64(scale)))))
		}
		img := Render(v)
		if p := at(0, 8); img.RGBAAt(p.X, p.Y) != marks.Green {
			t.Fatalf("five-hour bar missing or uses weekly color at scale %v", scale)
		}
		if p := at(1, 100); img.RGBAAt(p.X, p.Y) != marks.Red {
			t.Fatalf("weekly bar does not reflect its own usage at scale %v", scale)
		}
		if p := at(0, 100); img.RGBAAt(p.X, p.Y) == marks.Green || img.RGBAAt(p.X, p.Y).R == 0 {
			t.Fatal("unused portion must retain a visible empty track")
		}
		v.Items[0].Windows = []quota.Window{{Label: "5h", Used: 0}, {Label: "week", Used: 16}}
		img = Render(v)
		if p := at(0, 8); img.RGBAAt(p.X, p.Y) == marks.Green {
			t.Fatal("zero usage must not gain a fabricated fill")
		}
		if p := at(1, 8); img.RGBAAt(p.X, p.Y) != marks.Green {
			t.Fatal("small weekly usage lost its green fill")
		}
		v.Items[0].State = quota.Stale
		stale := Render(v)
		p := at(1, 8)
		if c := stale.RGBAAt(p.X, p.Y); c.G <= c.R || c.G >= marks.Green.G {
			t.Fatal("stale bar should retain a dimmed last-good indication")
		}
		v.Items[0].State = quota.OK
		v.Items[0].Windows = []quota.Window{{Label: "5h", Used: math.NaN()}}
		img = Render(v)
		if p := at(0, 8); img.RGBAAt(p.X, p.Y) == marks.Green {
			t.Fatal("unknown usage must not gain a fill")
		}
		if p := at(1, 8); img.RGBAAt(p.X, p.Y).R != 0 {
			t.Fatal("missing weekly pool gained a bar")
		}
	}
}

func TestFocusSelectionAndControls(t *testing.T) {
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		v := focusFixture()
		v.Scale = scale
		for _, id := range quota.ItemAll {
			v.Selected = id
			img := Render(v)
			panel := PanelBounds(v)
			if !panel.In(img.Bounds()) || panel.Dx() != int(468*scale) {
				t.Fatalf("panel geometry at %v: %v", scale, panel)
			}
			for _, candidate := range quota.ItemAll {
				r := PillItemBounds(v, candidate)
				p := image.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
				if r.Empty() || !r.In(panel) || Hit(v, p) != (Target{Control: AgentControl, ID: candidate}) {
					t.Fatalf("agent target %s at %v: %v %v", candidate, scale, r, Hit(v, p))
				}
				for _, c := range []image.Rectangle{CloseBounds(v), PinBounds(v), RefreshBounds(v), ResetRowBounds(v, id)} {
					if !r.Intersect(c).Empty() {
						t.Fatalf("rail overlaps control: %s", candidate)
					}
				}
			}
			for _, r := range []image.Rectangle{CloseBounds(v), PinBounds(v), RefreshBounds(v), ResetRowBounds(v, id)} {
				if r.Empty() || !r.In(panel) {
					t.Fatalf("control outside panel: %v", r)
				}
			}
		}
	}
}

func TestFocusReadoutMatchesSelection(t *testing.T) {
	v := focusFixture()
	v.Open = false
	v.Selected = quota.ItemCodex
	first := Render(v)
	v.Selected = quota.ItemClaude
	second := Render(v)
	if string(first.Pix) == string(second.Pix) {
		t.Fatal("selected authentic mark never changes")
	}
	r := PillBounds(v)
	if r.Dx() != 118 || r.Dy() != 38 {
		t.Fatalf("full compact readout changed size: %v", r)
	}
	if Hit(v, image.Pt(r.Min.X+10, r.Min.Y+10)).Control != OpenControl {
		t.Fatal("readout does not toggle Focus")
	}
}

func TestFocusLongDetailsScrollWithinBody(t *testing.T) {
	v := focusFixture()
	v.FocusMaxHeight = 340
	for i := range v.Items {
		if v.Items[i].ID == v.Selected {
			v.Items[i].State = quota.Stale
			v.Items[i].Err = strings.Repeat("An explanatory failure sentence. ", 90)
			v.Items[i].Stale = "8 min ago"
		}
	}
	if FocusScrollLimit(v) <= 0 {
		t.Fatal("long facts have no scroll range")
	}
	v.FocusScroll = FocusScrollLimit(v)
	img := Render(v)
	if PanelBounds(v).Dy() != 340 || !FocusPopupBounds(v).In(img.Bounds()) {
		t.Fatal("viewport height not respected")
	}
	footer := RefreshBounds(v)
	if Hit(v, image.Pt(footer.Min.X+1, footer.Min.Y+1)).Control != RefreshControl {
		t.Fatal("scroll hid footer")
	}
}

func TestFocusTruthfulPoolStates(t *testing.T) {
	for _, it := range []Item{
		{ID: quota.ItemCodex, State: quota.OK, Windows: []quota.Window{{Label: "5h", Used: 0}}},
		{ID: quota.ItemCopilot, State: quota.OK, Summary: "Unlimited"},
		{ID: quota.ItemDevin, State: quota.Failed, Err: "Unavailable", Help: "Open Devin to sign in."},
		{ID: quota.ItemClaude, State: quota.Waiting},
	} {
		v := View{Focus: true, Open: true, Items: []Item{it}, Selected: it.ID, Scale: 1}
		if Render(v).Bounds().Empty() {
			t.Fatal("missing data cannot render")
		}
		if ringExtra(it) != 0 {
			t.Fatal("invented second pool")
		}
	}
}

func TestTaskbarReadoutKeepsBackgroundTransparent(t *testing.T) {
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		v := focusFixture()
		v.Scale = scale
		img := RenderTaskbar(v)
		if img.Bounds().Size() != PillBounds(View{Focus: true, Items: v.Items, Scale: scale}).Size() {
			t.Fatal("docked readout lost its complete compact geometry")
		}
		for _, p := range []image.Point{image.Pt(0, 0), image.Pt(img.Bounds().Dx()/2, 0), image.Pt(img.Bounds().Dx()-1, img.Bounds().Dy()/2)} {
			if img.RGBAAt(p.X, p.Y).A != 0 {
				t.Fatalf("taskbar background painted at %v, scale %v", p, scale)
			}
		}
		nonzero := 0
		for i := 0; i < len(img.Pix); i += 4 {
			a := img.Pix[i+3]
			if a != 0 {
				nonzero++
			}
			if img.Pix[i] > a || img.Pix[i+1] > a || img.Pix[i+2] > a {
				t.Fatal("taskbar pixels are not premultiplied")
			}
		}
		if nonzero < 100 {
			t.Fatal("transparent render removed quota content")
		}
		panel := PanelBounds(v)
		if Render(v).RGBAAt(panel.Min.X+panel.Dx()/2, panel.Min.Y+4).A != 255 {
			t.Fatal("Focus popup lost its opaque surface")
		}
	}
}
