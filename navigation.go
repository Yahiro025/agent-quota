package main

import "agent-quota/widget"

func (a *app) selectAgent(id string) {
	if a.settings.Selected != id {
		a.groupsOpen, a.recoveryOpen = false, ""
		a.focusScroll = 0
	}
	a.settings.Selected = id
	a.hovered = widget.Target{}
	saveSettings(a.settings)
}

func (a *app) OnAgentNavigate(step int) {
	if !a.open || a.nativeDrag || a.windowDrag || (step != -1 && step != 1) || len(a.view.Items) < 2 {
		return
	}
	index := 0
	for i, item := range a.view.Items {
		if item.ID == a.view.Selected {
			index = i
			break
		}
	}
	index = (index + step + len(a.view.Items)) % len(a.view.Items)
	a.selectAgent(a.view.Items[index].ID)
	a.redraw()
}
