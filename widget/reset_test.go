package widget

import (
	"bytes"
	"testing"

	"agent-quota/quota"
)

// Reset and PillReset arrive preformatted so Render stays deterministic;
// identical Views must rasterize identical pixels.
func TestResetRendersDeterministic(t *testing.T) {
	v := View{Open: true, Items: []Item{
		{ID: "codex", State: quota.OK, Used: 42, Detail: "5h 42%", Reset: "5h resets in 3h 12m", PillReset: "3h12m"},
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

// The panel draws whichever toggled Reset string upstream handed over, so a
// different string paints different pixels and both forms give hit bounds.
func TestResetTextChangesPixels(t *testing.T) {
	base := Item{ID: "codex", State: quota.OK, Used: 42, Detail: "5h 42%", Reset: "5h resets in 3h 12m", PillReset: "3h12m"}
	countdown := View{Open: true, Items: []Item{base}}
	flipped := base
	flipped.Reset = "5h resets 18:42"
	flipped.PillReset = "18:42"
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
		t.Fatal("toggled reset string did not change rendered pixels")
	}
}

func TestResetRowBoundsEmptyCases(t *testing.T) {
	withReset := Item{ID: "codex", State: quota.OK, Used: 42, Detail: "5h 42%", Reset: "5h resets in 3h 12m", PillReset: "3h12m"}
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
		{ID: "codex", State: quota.OK, Used: 42, Detail: "5h 42%", Reset: "5h resets in 3h 12m"},
		{ID: "claude", State: quota.OK, Used: 10, Detail: "5h 10%", Reset: "5h resets in 1h 02m"},
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
