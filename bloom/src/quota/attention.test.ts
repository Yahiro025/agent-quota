import { describe, expect, test } from "bun:test";
import { reduceAttention, type Attention } from "./attention";

const idle = (over: Partial<Attention> = {}): Attention => ({
	panel: "status",
	alert: null,
	deferred: false,
	...over
});

describe("reduceAttention", () => {
	test("open-panel sets the panel and nothing else", () => {
		const next = reduceAttention(idle(), { type: "open-panel", panel: "quotas" });
		expect(next).toEqual({ panel: "quotas", alert: null, deferred: false });
	});

	test("an alert while idle records the id and keeps the panel", () => {
		const next = reduceAttention(idle({ panel: "calendar" }), {
			type: "alert",
			id: "quota"
		});
		expect(next.alert).toBe("quota");
		expect(next.panel).toBe("calendar");
	});

	test("timer completion arrives as an alert with id timer", () => {
		const next = reduceAttention(idle(), { type: "alert", id: "timer" });
		expect(next.alert).toBe("timer");
	});

	test("an alert while deferred stores nothing and keeps the panel", () => {
		const prev = idle({ panel: "command-center", deferred: true });
		const next = reduceAttention(prev, { type: "alert", id: "quota" });
		expect(next).toEqual({ panel: "command-center", alert: null, deferred: true });
	});

	test("alert-finished clears the alert and leaves the panel", () => {
		const prev = idle({ panel: "quotas", alert: "quota" });
		const next = reduceAttention(prev, { type: "alert-finished" });
		expect(next.alert).toBeNull();
		expect(next.panel).toBe("quotas");
	});

	test("clear-defer re-enables alerts after a deferring gesture", () => {
		let state = reduceAttention(idle(), { type: "typing" });
		state = reduceAttention(state, { type: "alert", id: "quota" });
		expect(state.alert).toBeNull();
		state = reduceAttention(state, { type: "clear-defer" });
		expect(state.deferred).toBe(false);
		state = reduceAttention(state, { type: "alert", id: "quota" });
		expect(state.alert).toBe("quota");
	});

	test("typing, dragging, pairing, and connecting each set deferred", () => {
		for (const type of ["typing", "dragging", "pairing", "connecting"] as const) {
			expect(reduceAttention(idle(), { type }).deferred).toBe(true);
		}
	});

	test("two alerts during typing are dropped, then a third lands after clear-defer", () => {
		let state = reduceAttention(idle({ panel: "calendar" }), { type: "typing" });
		state = reduceAttention(state, { type: "alert", id: "quota" });
		state = reduceAttention(state, { type: "alert", id: "timer" });
		expect(state.alert).toBeNull();
		expect(state.panel).toBe("calendar");
		state = reduceAttention(state, { type: "clear-defer" });
		state = reduceAttention(state, { type: "alert", id: "quota" });
		expect(state.alert).toBe("quota");
	});

	test("no-op events return the same object", () => {
		const openSame = idle({ panel: "status" });
		expect(reduceAttention(openSame, { type: "open-panel", panel: "status" })).toBe(openSame);

		const noAlert = idle();
		expect(reduceAttention(noAlert, { type: "alert-finished" })).toBe(noAlert);

		const deferred = idle({ deferred: true });
		for (const type of ["typing", "dragging", "pairing", "connecting"] as const) {
			expect(reduceAttention(deferred, { type })).toBe(deferred);
		}

		const notDeferred = idle();
		expect(reduceAttention(notDeferred, { type: "clear-defer" })).toBe(notDeferred);

		const withAlert = idle({ alert: "quota" });
		expect(reduceAttention(withAlert, { type: "alert", id: "quota" })).toBe(withAlert);
	});
});
