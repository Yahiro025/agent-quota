package widget

import (
	"bytes"
	"image"
	"strings"
	"testing"

	"agent-quota/internal/text"
	"agent-quota/quota"
)

func TestSelectedDetailsWrapWithoutLosingSourceFacts(t *testing.T) {
	it := Item{ID: quota.ItemAntigravity, State: quota.Stale, Used: 60, Plan: "Pro", Stale: "8 min ago", Err: "The latest request failed while connecting to the quota service. Try Refresh after checking the connection.", Observed: "16 Oct 08:00", Details: []Detail{{Label: "Gemini Models / Weekly Limit Remaining (gemini-pro-weekly) / weekly", Value: "60% used", Reset: "resets at 23 Oct 08:00"}, {Label: "Gemini Models / fast / 5h", Value: "20% used", Reset: "resets in 1h 0m"}}}
	v := View{Seam: true, Open: true, Selected: it.ID, Items: []Item{it}}
	d := selectedDetailLayout(v, 386)
	var lines []string
	for _, line := range d.lines {
		if text.Measure(line.value, line.style) > 338.01 {
			t.Fatalf("line exceeds full width: %q", line.value)
		}
		lines = append(lines, line.value)
	}
	joined := strings.Join(lines, " ")
	for _, line := range d.lines {
		joined += " " + line.amount + " " + line.reset
	}
	for _, want := range []string{"Antigravity", "Pro", "gemini-pro-weekly", "60%", "23 Oct 08:00", "20%", "As of 16 Oct 08:00", "Last good 8 min ago", it.Err} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q from %q", want, joined)
		}
	}
	if d.height <= 36 {
		t.Fatalf("complete detail squeezed into %dpx", d.height)
	}
}

func TestEverySelectedDetailChangesRenderedPixelsAtEveryDPI(t *testing.T) {
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		for _, id := range quota.ItemAll {
			for _, stale := range []bool{false, true} {
				it := Item{ID: id, State: quota.OK, Used: 70, Details: []Detail{{Label: "5h", Value: "19% used", Reset: "resets in 12m"}, {Label: "Week", Value: "70% used", Reset: "resets at 23 Oct 08:00"}}}
				if stale {
					it.State, it.Stale, it.Err = quota.Stale, "8 min ago", "Connection failed"
				}
				v := View{Seam: true, Open: true, Scale: scale, Selected: id, Items: []Item{it}}
				before := Render(v)
				v.Items[0].Details[1].Value = "71% used"
				if after := Render(v); bytes.Equal(before.Pix, after.Pix) {
					t.Fatalf("scale=%v id=%s stale=%v hides selected fact", scale, id, stale)
				}
			}
		}
	}
}

func TestExpandedDetailsAndFooterControlsStaySeparate(t *testing.T) {
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		v := seamFixture()
		v.Scale = scale
		v.Selected = quota.ItemAntigravity
		v.Items[2].Details = []Detail{{Label: "Gemini / pro / week", Value: "70% used", Reset: "resets at 23 Oct 08:00"}, {Label: "Gemini / fast / week", Value: "10% used", Reset: "resets at 24 Oct 08:00"}}
		for _, groups := range []bool{false, true} {
			v.GroupsOpen = groups
			g := layout(v)
			d := selectedDetailLayout(v, g.w)
			detail := g.rect(24, seamHeader+seamCells, g.w-48, d.height)
			if !GroupsBounds(v, v.Selected).Empty() || seamGroupHeight(v) != 0 {
				t.Fatal("complete bucket rows also display the legacy group table")
			}
			bounds := []image.Rectangle{ResetRowBounds(v, v.Selected), RefreshBounds(v)}
			for i, r := range bounds {
				if r.Empty() || !r.In(PanelBounds(v)) || r.Overlaps(detail) {
					t.Fatalf("scale=%v control outside footer: %v", scale, r)
				}
				for _, other := range bounds[i+1:] {
					if r.Overlaps(other) {
						t.Fatalf("footer controls overlap %v %v", r, other)
					}
				}
			}
			img := Render(v)
			last := d.lines[len(d.lines)-1]
			lineRect := g.rect(24, seamHeader+seamCells+last.y, g.w-48, 14)
			ink := false
			for y := lineRect.Min.Y; y < lineRect.Max.Y; y++ {
				for x := lineRect.Min.X; x < lineRect.Max.X; x++ {
					c := img.RGBAAt(x, y)
					ink = ink || c.R > 0 || c.G > 0 || c.B > 0
				}
			}
			if !ink {
				t.Fatalf("scale=%v final detail row has no painted text", scale)
			}
		}
	}
}
