package widget

import (
	"bytes"
	"testing"

	"agent-quota/quota"
)

// Both preformatted strings ride on the Item so Render stays deterministic;
// identical Views must rasterize identical pixels.
func TestResetRendersDeterministic(t *testing.T) {
	v := View{Open: true, Items: []Item{
		{ID: "codex", State: quota.OK, Used: 42, Detail: "5h 42%", ResetCountdown: "3h 12m left", ResetClock: "resets 18:42"},
	}}
	a := Render(v)
	b := Render(v)
	if a.Bounds() != b.Bounds() {
		t.Fatalf("bounds differ: %v vs %v", a.Bounds(), b.Bounds())
	}
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("same reset View rendered different pixels")
	}
}

// ShowClock flips which preformatted string the detail area shows.
func TestShowClockFlipsResetText(t *testing.T) {
	base := Item{ID: "codex", State: quota.OK, Used: 42, Detail: "5h 42%", ResetCountdown: "3h 12m left", ResetClock: "resets 18:42"}
	countdown := View{Open: true, Items: []Item{base}}
	flipped := base
	flipped.ShowClock = true
	clock := View{Open: true, Items: []Item{flipped}}
	if ResetRowBounds(countdown, "codex").Empty() {
		t.Fatal("countdown row should have reset bounds")
	}
	if ResetRowBounds(clock, "codex").Empty() {
		t.Fatal("clock row should have reset bounds")
	}
	a := Render(countdown)
	b := Render(clock)
	if bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("ShowClock did not change rendered pixels")
	}
}

func TestResetRowBoundsEmptyCases(t *testing.T) {
	withReset := Item{ID: "codex", State: quota.OK, Used: 42, Detail: "5h 42%", ResetCountdown: "3h 12m left", ResetClock: "resets 18:42"}
	if r := ResetRowBounds(View{Items: []Item{withReset}}, "codex"); !r.Empty() {
		t.Fatalf("closed panel should have empty reset bounds, got %v", r)
	}
	open := View{Open: true, Items: []Item{withReset}}
	if r := ResetRowBounds(open, "nope"); !r.Empty() {
		t.Fatalf("unknown id should have empty reset bounds, got %v", r)
	}
	plain := Item{ID: "codex", State: quota.OK, Used: 42, Detail: "5h 42%"}
	if r := ResetRowBounds(View{Open: true, Items: []Item{plain}}, "codex"); !r.Empty() {
		t.Fatalf("row without reset should have empty reset bounds, got %v", r)
	}
}

func TestResetRowBoundsStride(t *testing.T) {
	v := View{Open: true, Items: []Item{
		{ID: "codex", State: quota.OK, Used: 42, Detail: "5h 42%", ResetCountdown: "3h 12m left"},
		{ID: "claude", State: quota.OK, Used: 10, Detail: "5h 10%", ResetCountdown: "1h 02m left"},
	}}
	r0 := ResetRowBounds(v, "codex")
	r1 := ResetRowBounds(v, "claude")
	if r0.Empty() || r1.Empty() {
		t.Fatalf("expected non-empty rows, got %v and %v", r0, r1)
	}
	if dy := r1.Min.Y - r0.Min.Y; dy != rowH {
		t.Fatalf("row 1 sits %d px below row 0, want exactly rowH=%d", dy, rowH)
	}
}
