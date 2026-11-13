package main

import (
	"testing"

	"agent-quota/quota"
	"agent-quota/widget"
)

func TestAgentSelectionPreservesPinState(t *testing.T) {
	t.Setenv("AGENTQUOTA_DATA_DIR", t.TempDir())
	a := &app{
		settings: settings{Visible: map[string]bool{}, Selected: quota.ItemClaude},
		open:     true,
		scale:    1,
	}
	for _, id := range quota.ItemAll {
		a.settings.Visible[id] = true
	}
	a.poller = quota.NewPoller(func(string) quota.Reading { return quota.Reading{} }, func(string) bool { return false }, func() {})

	for _, initiallyPinned := range []bool{false, true} {
		pinLabel := "unpinned"
		if initiallyPinned {
			pinLabel = "pinned"
		}
		for _, id := range quota.ItemAll {
			for _, same := range []bool{true, false} {
				start := quota.ItemAll[0]
				if same {
					start = id
				} else if start == id {
					start = quota.ItemAll[1]
				}
				caseLabel := "different"
				if same {
					caseLabel = "same"
				}
				t.Run(pinLabel+"/"+id+"/"+caseLabel, func(t *testing.T) {
					a.settings.Selected = start
					a.open, a.pinned = true, initiallyPinned
					a.groupsOpen, a.recoveryOpen = true, start
					a.view = a.buildView()
					bounds := widget.PillItemBounds(a.view, id)
					a.OnPointerClick((bounds.Min.X+bounds.Max.X)/2, (bounds.Min.Y+bounds.Max.Y)/2)
					if a.settings.Selected != id || a.view.Selected != id || !a.open || !a.view.Open || a.pinned != initiallyPinned {
						t.Fatalf("selection state = selected:%q open:%v pinned:%v, want selected:%q open:true pinned:%v", a.settings.Selected, a.open, a.pinned, id, initiallyPinned)
					}
					if start != id && (a.groupsOpen || a.recoveryOpen != "") {
						t.Fatal("changing agents did not clear disclosure state")
					}
					a.OnHover(false)
					if a.open != initiallyPinned || a.pinned != initiallyPinned {
						t.Fatalf("pointer leave state = open:%v pinned:%v, want both %v", a.open, a.pinned, initiallyPinned)
					}
				})
			}
		}
	}

	for _, initiallyPinned := range []bool{false, true} {
		a.open, a.pinned = true, initiallyPinned
		a.view = a.buildView()
		bounds := widget.PinBounds(a.view)
		a.OnPointerClick((bounds.Min.X+bounds.Max.X)/2, (bounds.Min.Y+bounds.Max.Y)/2)
		if a.pinned == initiallyPinned || !a.open {
			t.Fatalf("explicit Pin state = open:%v pinned:%v, initial pin:%v", a.open, a.pinned, initiallyPinned)
		}
	}
}
