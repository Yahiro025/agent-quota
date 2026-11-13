package main

import (
	"image"
	"testing"

	"agent-quota/internal/win"
	"agent-quota/widget"
)

func TestSeamMigrationRunsOnceAndRetainsReaderPreferences(t *testing.T) {
	// LayoutVersion 0 predates Seam entirely: the old corner anchor and the
	// Compact default both belong to the previous layout.
	legacy := settings{X: 800, Y: 200, HasPos: true, Compact: true, Selected: "codex", Visible: map[string]bool{"codex": true, "claude": false}, Docked: true}
	s := migrateSeamSettings(legacy)
	if s.HasPos || s.Compact || s.LayoutVersion != 2 {
		t.Fatalf("migration kept the previous layout: %+v", s)
	}
	if s.Selected != "codex" || !s.Visible["codex"] || s.Visible["claude"] || !s.Docked {
		t.Fatalf("migration lost preferences: %+v", s)
	}
	// LayoutVersion 1 saved its top edge at work.Min.Y, which MyDockFinder
	// makes 30 px. Clearing only the anchor re-snaps Seam to the monitor's top
	// edge (y = 0) while Compact and the reader choices survive untouched.
	v1 := settings{X: 800, Y: 30, HasPos: true, Compact: true, Selected: "codex", Visible: map[string]bool{"codex": true, "claude": false}, Docked: true, LayoutVersion: 1}
	s = migrateSeamSettings(v1)
	if s.HasPos || s.LayoutVersion != 2 {
		t.Fatalf("version 1 migration kept the work-area anchor: %+v", s)
	}
	if !s.Compact || s.Selected != "codex" || !s.Visible["codex"] || s.Visible["claude"] || !s.Docked {
		t.Fatalf("version 1 migration lost preferences: %+v", s)
	}
	// Once at version 2 a real drag position is the user's choice for good.
	v2 := settings{X: 300, Y: 250, HasPos: true, Compact: true, Selected: "codex", Visible: map[string]bool{"codex": true}, LayoutVersion: 2}
	got := migrateSeamSettings(v2)
	if got.X != 300 || got.Y != 250 || !got.HasPos || !got.Compact || got.LayoutVersion != 2 || got.Selected != "codex" {
		t.Fatalf("later launch replaced a valid user drag/preference: %+v", got)
	}
}

// An appbar that reserves the screen's top edge must not keep the notch below
// it: Seam reaches y = 0 while the side and bottom work-area bounds stay.
func TestSeamWorkAreaReachesMonitorTopEdge(t *testing.T) {
	work := image.Rect(0, 30, 1920, 1200)
	monitor := image.Rect(0, 0, 1920, 1200)
	seam := win.SeamWorkAreaFor(work, monitor)
	if seam != image.Rect(0, 0, 1920, 1200) {
		t.Fatalf("seam area = %v, want the monitor top edge with work-area sides", seam)
	}
	if got := win.SeamWorkAreaFor(image.Rect(0, 0, 1920, 1040), monitor); got != image.Rect(0, 0, 1920, 1040) {
		t.Fatalf("bottom taskbar bound lost: %v", got)
	}
	if got := win.SeamWorkAreaFor(work, image.Rectangle{}); got != work {
		t.Fatalf("missing monitor bounds did not fall back to the work area: %v", got)
	}
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		v := widget.View{Seam: true, Scale: scale, Items: make([]widget.Item, 7)}
		x, y := initialPillPosition(settings{}, v, seam)
		r := widget.PillBounds(v)
		if y != 0 {
			t.Fatalf("scale=%v placed the notch at y=%d, want the monitor top edge", scale, y)
		}
		if x+r.Dx()/2 < 1920/2-1 || x+r.Dx()/2 > 1920/2+1 {
			t.Fatalf("scale=%v anchor=(%d,%d) bounds=%v", scale, x, y, r)
		}
	}
}

func TestSeamStartsFlushAtWorkAreaCentreAndPreservesValidDrag(t *testing.T) {
	work := image.Rect(60, 40, 1860, 1120)
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		v := widget.View{Seam: true, Scale: scale, Items: make([]widget.Item, 7)}
		x, y := initialPillPosition(settings{}, v, work)
		r := widget.PillBounds(v)
		if y != work.Min.Y || x+r.Dx()/2 < (work.Min.X+work.Max.X)/2-1 || x+r.Dx()/2 > (work.Min.X+work.Max.X)/2+1 {
			t.Fatalf("scale=%v anchor=(%d,%d) bounds=%v", scale, x, y, r)
		}
		x, y = initialPillPosition(settings{X: 300, Y: 400, HasPos: true}, v, work)
		if x != 300 || y != 400 {
			t.Fatal("valid Seam drag anchor moved on launch")
		}
	}
}
