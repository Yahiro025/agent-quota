package widget

import "agent-quota/quota"

// Model groups share a ring per window; the fullest reported group sets its arc.
func poolWindow(it Item, label string) *quota.Window {
	var found *quota.Window
	visit := func(windows []quota.Window) {
		for i := range windows {
			w := &windows[i]
			if w.Label == label && (found == nil || w.Used > found.Used) {
				found = w
			}
		}
	}
	visit(it.Windows)
	for _, group := range it.Groups {
		visit(group.Windows)
	}
	return found
}

func headlineUsed(it Item) float64 {
	if w := poolWindow(it, "5h"); w != nil {
		return w.Used
	}
	return it.Used
}

func ringExtra(it Item) int {
	if poolWindow(it, "5h") != nil && poolWindow(it, "week") != nil {
		return 6
	}
	return 0
}

func viewPillSize(v View) (int, int) {
	if v.Compact && len(v.Items) > 0 {
		it, _ := seamSelected(v)
		return 112 + ringExtra(it), 38
	}
	w, h := PillSize(len(v.Items), PctWidth)
	for _, it := range v.Items {
		w += ringExtra(it)
	}
	return w, h
}
