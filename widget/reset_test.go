package widget

import (
	"bytes"
	"testing"

	"agent-quota/internal/text"
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

// The pill's second line shares the percent's fixed slot, so the widest
// strings resetLabels emits — a floored countdown and a wall clock — must fit
// PctWidth at resetFont. If this fails the slot is too narrow; PctWidth is the
// only allowed fix because it feeds PillSize uniformly for every agent.
func TestPillResetFitsSlot(t *testing.T) {
	for _, s := range []string{"4h59m", "23:59"} {
		w := text.Measure(s, resetFont)
		t.Logf("%q measures %.2f at resetFont (PctWidth=%d)", s, w, PctWidth)
		if w > float32(PctWidth) {
			t.Fatalf("%q measures %.2f at resetFont, over PctWidth=%d", s, w, PctWidth)
		}
	}
}

// A PillReset moves the percent up and adds a second line inside the same
// pill, so pixels must differ in the reset band while the pill never resizes.
func TestPillResetAddsSecondLine(t *testing.T) {
	base := Item{ID: "codex", State: quota.OK, Used: 42, Detail: "5h 42%"}
	with := base
	with.PillReset = "3h12m"
	a := Render(View{Items: []Item{base}})
	b := Render(View{Items: []Item{with}})
	if a.Bounds() != b.Bounds() {
		t.Fatalf("PillReset resized the pill: %v vs %v", a.Bounds(), b.Bounds())
	}
	if bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("PillReset did not change rendered pixels")
	}
	// The scan starts at y=24 on purpose: the raised percent inks rows ~7-16
	// and its unraised twin ~14-23, so only the reset line (drawn at py+21,
	// glyph ink ~22-29) can differ at y >= 24. Scanning from the midline would
	// pass on moved percent ink alone even if no second line were drawn.
	lower := false
	for y := 24; y < a.Bounds().Dy(); y++ {
		for x := 0; x < a.Bounds().Dx(); x++ {
			i := a.PixOffset(x, y)
			if !bytes.Equal(a.Pix[i:i+4], b.Pix[i:i+4]) {
				lower = true
			}
		}
	}
	if !lower {
		t.Fatal("PillReset changed no pixels in the reset band; second line missing")
	}
}

// A runaway reset string is elided inside the percent slot: the pill width is
// fixed, so overflow would paint over the next agent's ring.
func TestPillResetStaysInSlot(t *testing.T) {
	items := []Item{
		{ID: "codex", State: quota.OK, Used: 42, Detail: "5h 42%"},
		{ID: "claude", State: quota.OK, Used: 10, Detail: "5h 10%"},
	}
	with := []Item{items[0], items[1]}
	with[0].PillReset = "9999h99m far too long to show"
	a := Render(View{Items: items})
	b := Render(View{Items: with})
	slotEnd := pad + cell + pctGap + PctWidth
	for y := 0; y < a.Bounds().Dy(); y++ {
		for x := slotEnd; x < a.Bounds().Dx(); x++ {
			i := a.PixOffset(x, y)
			if !bytes.Equal(a.Pix[i:i+4], b.Pix[i:i+4]) {
				t.Fatalf("PillReset painted past its slot at (%d,%d)", x, y)
			}
		}
	}
}
