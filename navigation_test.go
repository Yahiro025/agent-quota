package main

import (
	"testing"

	"agent-quota/quota"
	"agent-quota/widget"
)

func TestArrowKeysRotateDisplayedAgentsAndPreservePanelState(t *testing.T) {
	t.Setenv("AGENTQUOTA_DATA_DIR", t.TempDir())
	for _, tc := range []struct {
		name, selected, want string
		step                 int
		open, pinned         bool
	}{
		{"right", "claude", "codex", 1, true, false},
		{"left", "devin", "codex", -1, true, true},
		{"wrap right", "devin", "claude", 1, true, true},
		{"wrap left", "claude", "devin", -1, true, false},
		{"closed", "claude", "claude", 1, false, false},
		{"invalid direction", "claude", "claude", 2, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &app{settings: settings{Selected: tc.selected, Visible: map[string]bool{"claude": true, "codex": true, "devin": true}}, scale: 1, open: tc.open, pinned: tc.pinned, groupsOpen: true, recoveryOpen: "claude", focusScroll: 120}
			a.poller = quota.NewPoller(func(string) quota.Reading { return quota.Reading{} }, func(string) bool { return false }, func() {})
			a.view = widget.View{Selected: tc.selected, Items: []widget.Item{{ID: "claude"}, {ID: "codex"}, {ID: "devin", State: quota.Failed}}}
			navigator, ok := any(a).(interface{ OnAgentNavigate(int) })
			if !ok {
				t.Fatal("app does not handle arrow navigation")
			}
			navigator.OnAgentNavigate(tc.step)
			if a.settings.Selected != tc.want || a.open != tc.open || a.pinned != tc.pinned {
				t.Fatalf("selected=%q open=%v pinned=%v, want %q %v %v", a.settings.Selected, a.open, a.pinned, tc.want, tc.open, tc.pinned)
			}
			if tc.want != tc.selected {
				if a.groupsOpen || a.recoveryOpen != "" || a.focusScroll != 0 || loadSettings().Selected != tc.want {
					t.Fatal("arrow selection did not reset detail state and persist selection")
				}
			}
		})
	}
}

func TestArrowNavigationUsesRenderedFallbackAndIgnoresDragOrShortLists(t *testing.T) {
	t.Setenv("AGENTQUOTA_DATA_DIR", t.TempDir())
	for _, tc := range []struct {
		name, want string
		items      []widget.Item
		drag       bool
	}{
		{"rendered fallback", "devin", []widget.Item{{ID: "claude"}, {ID: "devin"}}, false},
		{"empty", "kiro", nil, false},
		{"one item", "kiro", []widget.Item{{ID: "claude"}}, false},
		{"drag", "kiro", []widget.Item{{ID: "claude"}, {ID: "devin"}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &app{settings: settings{Selected: "kiro", Visible: map[string]bool{"claude": true, "devin": true}}, scale: 1, open: true, nativeDrag: tc.drag}
			a.poller = quota.NewPoller(func(string) quota.Reading { return quota.Reading{} }, func(string) bool { return false }, func() {})
			a.view = widget.View{Selected: "claude", Items: tc.items}
			a.OnAgentNavigate(1)
			if a.settings.Selected != tc.want {
				t.Fatalf("selection=%q, want %q", a.settings.Selected, tc.want)
			}
		})
	}
}
