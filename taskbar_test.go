package main

import (
	"image"
	"testing"
)

func TestFocusPopupStaysAboveTaskbar(t *testing.T) {
	work := image.Rect(0, 0, 1920, 1150)
	for _, readout := range []image.Rectangle{image.Rect(1397, 1151, 1545, 1199), image.Rect(0, 1151, 148, 1199)} {
		at := focusPopupPosition(readout, work, 625, 460)
		popup := image.Rectangle{Min: at, Max: at.Add(image.Pt(625, 460))}
		if !popup.In(work) || popup.Max.Y > readout.Min.Y-8 {
			t.Fatalf("popup %v overlaps taskbar %v", popup, readout)
		}
	}
}

func TestDockPreferencePreservesFloatingAnchor(t *testing.T) {
	t.Setenv("AGENTQUOTA_DATA_DIR", t.TempDir())
	s := settings{X: 300, Y: 200, HasPos: true, Docked: true, Selected: "codex", Visible: map[string]bool{"codex": true}, LayoutVersion: 2}
	saveSettings(s)
	loaded := loadSettings()
	if !loaded.Docked || loaded.X != 300 || loaded.Y != 200 || loaded.Selected != "codex" {
		t.Fatalf("lost dock intent or floating anchor: %+v", loaded)
	}
	s.Docked = false
	saveSettings(s)
	if loadSettings().Docked {
		t.Fatal("floating preference did not persist")
	}
}
