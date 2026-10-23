import { describe, expect, test } from "bun:test";
import type { QuotaAgent } from "./protocol";
import {
	anyBusy,
	basis,
	defaultSelection,
	detailLines,
	failedCount,
	formatReset
} from "./quotaView";

// Mirrors widget/seam.go seamBasis, widget/seam_test.go and
// widget/reset_test.go; each expectation is a literal from the Go port.
const agent = (over: Partial<QuotaAgent>): QuotaAgent => ({
	id: "devin",
	state: "ok",
	...over
});

const NOW = new Date("2026-10-23T00:00:00Z");

describe("basis", () => {
	test("state labels", () => {
		expect(basis(agent({ state: "waiting" }))).toBe("Checking…");
		expect(basis(agent({ state: "failed" }))).toBe("Unavailable");
		expect(basis(agent({ state: "stale" }))).toBe("Stale");
	});

	test("a 5h window gives 5h, including inside groups", () => {
		expect(basis(agent({ windows: [{ label: "5h", used: 19 }] }))).toBe("5h");
		expect(
			basis(
				agent({
					groups: [{ name: "Gemini", windows: [{ label: "5h", used: 0 }] }]
				})
			)
		).toBe("5h");
	});

	test("Unlimited summary gives Unlimited", () => {
		expect(basis(agent({ summary: "Unlimited" }))).toBe("Unlimited");
		expect(basis(agent({ summary: "unlimited" }))).toBe("Unlimited");
	});

	test("groups give Models", () => {
		expect(
			basis(agent({ groups: [{ name: "Gemini", windows: [] }] }))
		).toBe("Models");
	});

	test("the fullest window gives its name", () => {
		expect(
			basis(
				agent({
					windows: [
						{ label: "week", used: 28 },
						{ label: "day", used: 56 }
					]
				})
			)
		).toBe("Day");
	});

	test("per-agent fallbacks", () => {
		expect(basis(agent({ id: "kiro" }))).toBe("Credits");
		expect(basis(agent({ id: "cursor" }))).toBe("Plan");
		expect(basis(agent({ id: "copilot" }))).toBe("Premium");
		expect(basis(agent({ id: "devin" }))).toBe("Usage");
		expect(basis(agent({ id: "claude" }))).toBe("Usage");
	});
});

describe("detailLines", () => {
	// The photo fixture: devin 56% Day + 28% Week on a Pro plan.
	const devin = agent({
		used: 56,
		plan: "Pro",
		details: [
			{ label: "Day", value: "56% used", resetAt: "2026-10-23T06:18:00Z" },
			{ label: "Week", value: "28% used", resetAt: "2026-10-25T06:18:00Z" }
		]
	});

	test("countdown rows join label, value and reset", () => {
		expect(detailLines(devin, NOW, false)).toEqual([
			"Day · 56% used · resets in 6h 18m",
			"Week · 28% used · resets in 2d 6h 18m"
		]);
	});

	test("clock rows use the wall clock", () => {
		const clock = new Date("2026-10-23T06:18:00Z").toLocaleTimeString([], {
			hour: "numeric",
			minute: "2-digit"
		});
		expect(detailLines(devin, NOW, true)[0]).toBe(
			`Day · 56% used · resets ${clock}`
		);
	});

	test("a past reset reads as a wall-clock time", () => {
		const a = agent({
			details: [
				{ label: "5h", value: "19% used", resetAt: "2026-10-22T23:00:00Z" }
			]
		});
		expect(detailLines(a, NOW, false)[0]).toMatch(
			/^5h · 19% used · resets at \d{1,2} \w{3} \d{2}:\d{2}$/
		);
	});

	test("resetText stands in when there is no resetAt", () => {
		const a = agent({
			details: [{ label: "ACU", value: "0 used", resetText: "Nov 1" }]
		});
		expect(detailLines(a, NOW, false)).toEqual([
			"ACU · 0 used · resets Nov 1"
		]);
	});

	test("waiting reads Checking usage", () => {
		expect(detailLines(agent({ state: "waiting" }), NOW, false)).toEqual([
			"Checking usage…"
		]);
	});

	test("failed surfaces the error, then the detail", () => {
		expect(
			detailLines(agent({ state: "failed", err: "Sign-in expired" }), NOW, false)
		).toEqual(["Sign-in expired"]);
		expect(
			detailLines(
				agent({ state: "failed", detail: "no credentials" }),
				NOW,
				false
			)
		).toEqual(["no credentials"]);
	});
});

describe("formatReset", () => {
	test("a time in the past gives now", () => {
		expect(formatReset("2026-10-22T23:59:00Z", NOW, false)).toBe("now");
		expect(formatReset("2026-10-23T00:00:00Z", NOW, false)).toBe("now");
	});

	test("clock: true gives local h:mm AM/PM", () => {
		const at = "2026-10-23T06:18:00Z";
		const expected = new Date(at).toLocaleTimeString([], {
			hour: "numeric",
			minute: "2-digit"
		});
		expect(formatReset(at, NOW, true)).toBe(expected);
	});

	test("countdown for a future time", () => {
		expect(formatReset("2026-10-23T01:30:00Z", NOW, false)).toBe("1h 30m");
	});
});

describe("failedCount", () => {
	test("counts failed agents", () => {
		const readings: QuotaAgent[] = [
			agent({ id: "claude" }),
			agent({ id: "kiro" }),
			agent({ id: "codex" }),
			agent({ id: "cursor" }),
			agent({ id: "copilot", state: "failed", err: "HTTP 401" })
		];
		expect(failedCount(readings)).toBe(1);
	});
});

describe("anyBusy", () => {
	test("true when any reading is busy", () => {
		expect(anyBusy([agent({ busy: true })])).toBe(true);
		expect(anyBusy([agent({ busy: false }), agent({})])).toBe(false);
	});
});

describe("defaultSelection", () => {
	test("picks the first urgent agent", () => {
		const readings = [
			agent({ id: "claude", used: 10 }),
			agent({ id: "codex", used: 95 })
		];
		expect(defaultSelection(readings)).toBe("codex");
	});

	test("picks the first failed agent when nothing is urgent", () => {
		const readings = [
			agent({ id: "claude", used: 10 }),
			agent({ id: "devin", state: "failed", err: "Sign-in expired" })
		];
		expect(defaultSelection(readings)).toBe("devin");
	});

	test("falls back to the first agent, then undefined for empty", () => {
		expect(defaultSelection([agent({ id: "kiro" })])).toBe("kiro");
		expect(defaultSelection([])).toBeUndefined();
	});
});
