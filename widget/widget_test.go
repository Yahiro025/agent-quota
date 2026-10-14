package widget

import (
	"image"
	"math"
	"testing"

	"agent-quota/quota"
)

func TestRenderStates(t *testing.T) {
	v := View{Items: []Item{
		{ID: "claude", State: quota.OK, Used: 42, Detail: "Pro · 5h 42%"},
		{ID: "cursor", State: quota.Failed, Err: "Sign in first."},
		{ID: "devin", State: quota.Stale, Used: 62, Err: "unreachable", Stale: "3 min"},
		{ID: quota.ItemAntigravity, State: quota.OK, Used: 10, Detail: "Gemini Models · 90% remaining"},
	}}
	img := Render(v)
	if img.Bounds().Dx() <= 0 || img.Bounds().Dy() <= 0 {
		t.Fatal("empty image")
	}
	open := Render(View{Items: v.Items, Open: true, Checked: "Checked just now"})
	if open.Bounds().Dy() <= img.Bounds().Dy() {
		t.Fatal("panel did not grow the image")
	}
	if w, _ := PillSize(7, 34); w != 541 {
		t.Fatalf("seven-agent pill width = %d, want 541", w)
	}
}

func TestRenderUsesPhysicalDPIAndPremultipliedAlpha(t *testing.T) {
	v := View{Open: true, Items: []Item{
		{ID: "claude", State: quota.OK, Used: 42},
		{ID: "kiro", State: quota.OK, Used: 75},
		{ID: "codex", State: quota.OK, Used: 95},
		{ID: "cursor", State: quota.Failed, Err: "Sign in first."},
		{ID: "copilot", State: quota.OK, Used: 13},
		{ID: "devin", State: quota.Stale, Used: 62, Err: "unreachable", Stale: "3 min"},
		{ID: quota.ItemAntigravity, State: quota.OK, Used: 10},
	}}
	base := Render(v).Bounds()
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		v.Scale = scale
		img := Render(v)
		if img.Bounds().Dx() != int(math.Round(float64(float32(base.Dx())*scale))) || img.Bounds().Dy() != int(math.Round(float64(float32(base.Dy())*scale))) {
			t.Fatalf("scale %v did not render at physical resolution", scale)
		}
		for i := 0; i < len(img.Pix); i += 4 {
			if img.Pix[i] > img.Pix[i+3] || img.Pix[i+1] > img.Pix[i+3] || img.Pix[i+2] > img.Pix[i+3] {
				t.Fatalf("scale %v has unassociated-alpha pixels at %d", scale, i)
			}
		}
		if !RefreshBounds(v).In(img.Bounds()) || RefreshBounds(v).Empty() {
			t.Fatalf("scale %v refresh target is outside image", scale)
		}
	}
}

func TestInspectorKeepsPillAnchorAndOpaqueTextSurface(t *testing.T) {
	v := View{Open: true, Items: []Item{{ID: "claude", State: quota.Waiting}}}
	for _, above := range []bool{false, true} {
		v.Above = above
		img := Render(v)
		pill := PillBounds(v)
		if pill.Max.X != img.Bounds().Max.X || !pill.In(img.Bounds()) {
			t.Fatal("inspector does not keep the pill at its right edge")
		}
		panelY := pillH + panelGap
		if above {
			panelY = 0
			if pill.Max.Y != img.Bounds().Max.Y {
				t.Fatal("above inspector misplaced pill")
			}
		}
		pixel := img.RGBAAt(100, panelY+20)
		if pixel.A != 255 {
			t.Fatal("inspector lets desktop text show through its reading surface")
		}
		if image.Pt(1, 1).In(RefreshBounds(v)) {
			t.Fatal("refresh hit area includes the pill")
		}
	}
}
