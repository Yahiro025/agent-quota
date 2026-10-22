// One visibility controller for the notch. Panels open by user choice, so
// alerts (an urgent quota reading, a finished timer) only record themselves
// — they never move the open panel. Gestures that mean "the user is busy"
// (typing, dragging, pairing, connecting) set `deferred`, and an alert
// arriving then is dropped outright: nothing is stored to replay later.

export type Attention = { panel: string; alert: string | null; deferred: boolean };

export type AttentionEvent =
	| { type: "open-panel"; panel: string }
	| { type: "alert"; id: string }
	| { type: "alert-finished" }
	| { type: "typing" }
	| { type: "dragging" }
	| { type: "pairing" }
	| { type: "connecting" }
	| { type: "clear-defer" };

export function reduceAttention(prev: Attention, event: AttentionEvent): Attention {
	switch (event.type) {
		case "open-panel":
			return event.panel === prev.panel ? prev : { ...prev, panel: event.panel };
		case "alert":
			return prev.deferred || event.id === prev.alert ? prev : { ...prev, alert: event.id };
		case "alert-finished":
			return prev.alert === null ? prev : { ...prev, alert: null };
		case "typing":
		case "dragging":
		case "pairing":
		case "connecting":
			return prev.deferred ? prev : { ...prev, deferred: true };
		case "clear-defer":
			return prev.deferred ? { ...prev, deferred: false } : prev;
	}
}
