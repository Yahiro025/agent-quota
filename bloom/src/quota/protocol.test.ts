import { describe, expect, test } from "bun:test";
import { band, parseSnapshot, urgent, type QuotaAgent } from "./protocol";
import {
	agentRings,
	alertRows,
	detailRows,
	wallClock,
	hoverQuotaAgent,
	hoverQuotaTooltip,
	quotaAlertVisible,
	quotaView
} from "./quotaView";
import { DEFAULT_PANEL, defaultPanel, overviewRows } from "./OverviewPanel";

const SNAPSHOT =
	'{"v":1,"op":"snapshot","readings":[' +
	'{"id":"claude","state":"ok","busy":false,"used":42.5,"detail":"5h window","reset5h":"2026-10-22T15:00:00Z"},' +
	'{"id":"kiro","state":"waiting","busy":true},' +
	'{"id":"codex","state":"stale","busy":false,"used":71.5,"err":"timeout","lastGoodAt":"2026-10-22T09:30:00Z"},' +
	'{"id":"copilot","state":"failed","busy":false,"err":"HTTP 401"},' +
	'{"id":"devin","state":"ok","busy":false,"used":0,"plan":"Core","windows":[{"label":"ACU","used":0}],"groups":[{"name":"all","windows":[{"label":"ACU","used":0}]}],"details":[{"label":"ACU","value":"0 used","resetAt":"2026-11-01T00:00:00Z","resetText":"Nov 1"}]}' +
	"]}";

describe("wallClock", () => {
	test("uses the locale time today and the date plus that time on a later day", () => {
		const now = new Date(2026, 9, 22, 13, 30);
		const today = new Date(2026, 9, 22, 17, 0);
		const later = new Date(2026, 9, 27, 8, 5);
		expect(wallClock(today, now)).toBe(
			today.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" })
		);
		expect(wallClock(later, now)).toBe(
			`27 Oct ${later.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" })}`
		);
	});
});

describe("detailRows", () => {
	const NOW = new Date("2026-10-22T13:30:00Z");

	test("returns nothing for waiting and failed agents", () => {
		expect(detailRows({ id: "kiro", state: "waiting" }, NOW, false)).toEqual([]);
		expect(
			detailRows({ id: "claude", state: "failed", err: "HTTP 401" }, NOW, false)
		).toEqual([]);
	});

	test("maps multi-window detail rows like Devin's day/week", () => {
		const a: QuotaAgent = {
			id: "devin",
			state: "ok",
			used: 56,
			plan: "Pro",
			details: [
				{ label: "Day", value: "56% used", resetAt: "2026-10-22T19:48:00Z" },
				{ label: "Week", value: "28% used", resetAt: "2026-10-24T19:48:00Z" }
			]
		};
		expect(detailRows(a, NOW, false)).toEqual([
			{
				label: "Day",
				used: 56,
				value: "56%",
				reset: "resets in 6h 18m",
				resetClock: wallClock(new Date("2026-10-22T19:48:00Z"), NOW)
			},
			{
				label: "Week",
				used: 28,
				value: "28%",
				reset: "resets in 2d 6h",
				resetClock: wallClock(new Date("2026-10-24T19:48:00Z"), NOW)
			}
		]);
	});

	test("a lone 5h window takes the reset5h instant", () => {
		const a: QuotaAgent = {
			id: "codex",
			state: "ok",
			used: 10,
			windows: [{ label: "5h", used: 10 }],
			reset5h: "2026-10-22T17:00:00Z"
		};
		expect(detailRows(a, NOW, false)).toEqual([
			{
				label: "5h",
				used: 10,
				value: "10%",
				reset: "resets in 3h 30m",
				resetClock: wallClock(new Date("2026-10-22T17:00:00Z"), NOW)
			}
		]);
	});

	test("clock mode swaps the countdown for the wall clock", () => {
		// Local-time instants keep the same calendar day in any timezone.
		const now = new Date(2026, 9, 22, 13, 30);
		const a: QuotaAgent = {
			id: "codex",
			state: "ok",
			used: 10,
			windows: [{ label: "5h", used: 10 }],
			reset5h: new Date(2026, 9, 22, 17, 0).toISOString()
		};
		const clock = new Date(2026, 9, 22, 17, 0).toLocaleTimeString([], {
			hour: "numeric",
			minute: "2-digit"
		});
		expect(detailRows(a, now, true)[0].reset).toBe(`resets ${clock}`);
		expect(detailRows(a, now, true)[0].resetClock).toBeUndefined();
	});

	test("a grouped agent pools its 5h window into one row", () => {
		const a: QuotaAgent = {
			id: "antigravity",
			state: "ok",
			used: 0,
			reset5h: "2026-10-22T18:24:00Z",
			groups: [
				{ name: "Gemini 3 Pro", windows: [{ label: "5h", used: 0 }] },
				{ name: "GPT-OSS 120B", windows: [{ label: "5h", used: 0 }] }
			]
		};
		expect(detailRows(a, NOW, false)).toEqual([
			{
				label: "5h",
				used: 0,
				value: "0%",
				reset: "resets in 4h 54m",
				resetClock: wallClock(new Date("2026-10-22T18:24:00Z"), NOW)
			}
		]);
	});

	test("a stale lone window falls back to resetLabel", () => {
		const a: QuotaAgent = {
			id: "copilot",
			state: "stale",
			used: 90,
			windows: [{ label: "premium", used: 90 }],
			reset5h: "2026-10-22T18:24:00Z",
			resetLabel: "1 Nov"
		};
		expect(detailRows(a, NOW, false)).toEqual([
			{ label: "premium", used: 90, value: "90%", reset: "resets 1 Nov" }
		]);
	});

	test("a bare used value lands on the agent's basis", () => {
		const a: QuotaAgent = { id: "cursor", state: "ok", used: 71 };
		expect(detailRows(a, NOW, false)).toEqual([
			{ label: "Plan", used: 71, value: "71%", reset: undefined }
		]);
	});
});

describe("hoverQuotaTooltip", () => {
	test("joins the name, plan and metric rows", () => {
		const a: QuotaAgent = {
			id: "devin",
			state: "ok",
			used: 56,
			plan: "Pro",
			details: [
				{ label: "Day", value: "56% used", resetAt: "2026-10-22T19:48:00Z" },
				{ label: "Week", value: "28% used", resetAt: "2026-10-24T19:48:00Z" }
			]
		};
		expect(hoverQuotaTooltip(a, new Date("2026-10-22T13:30:00Z"))).toBe(
			"Devin · Pro\nDay · 56% · resets in 6h 18m\nWeek · 28% · resets in 2d 6h"
		);
	});

	test("falls back to the basis caption without rows", () => {
		expect(
			hoverQuotaTooltip({ id: "kiro", state: "ok" }, new Date())
		).toBe("Kiro\nCredits");
	});
});

describe("parseSnapshot", () => {
	test("returns the readings array for a snapshot line", () => {
		const readings = parseSnapshot(SNAPSHOT);
		expect(readings).not.toBeNull();
		expect(readings).toHaveLength(5);
		expect(readings![0].id).toBe("claude");
		expect(readings![0].state).toBe("ok");
		expect(readings![0].used).toBe(42.5);
	});

	test("keeps used: 0 instead of treating it as absent", () => {
		const readings = parseSnapshot(SNAPSHOT)!;
		const devin = readings.find((r) => r.id === "devin")!;
		expect(devin.used).toBe(0);
		expect(devin.windows).toEqual([{ label: "ACU", used: 0 }]);
		expect(devin.groups).toEqual([
			{ name: "all", windows: [{ label: "ACU", used: 0 }] }
		]);
		expect(devin.details).toEqual([
			{
				label: "ACU",
				value: "0 used",
				resetAt: "2026-11-01T00:00:00Z",
				resetText: "Nov 1"
			}
		]);
	});

	test("keeps used and err on a stale agent", () => {
		const readings = parseSnapshot(SNAPSHOT)!;
		const codex = readings.find((r) => r.id === "codex")!;
		expect(codex.state).toBe("stale");
		expect(codex.used).toBe(71.5);
		expect(codex.err).toBe("timeout");
		expect(codex.lastGoodAt).toBe("2026-10-22T09:30:00Z");
	});

	test("returns null for ready, bye, and error lines", () => {
		expect(
			parseSnapshot(
				'{"v":1,"op":"ready","agents":["claude","kiro","codex","cursor","copilot","devin","antigravity","grokbot","commandcode","qoder"]}'
			)
		).toBeNull();
		expect(parseSnapshot('{"v":1,"op":"bye"}')).toBeNull();
		expect(
			parseSnapshot('{"v":1,"op":"error","message":"kiro timeout"}')
		).toBeNull();
	});

	test("returns null for blank and non-JSON lines", () => {
		expect(parseSnapshot("")).toBeNull();
		expect(parseSnapshot("   ")).toBeNull();
		expect(parseSnapshot("not json at all")).toBeNull();
	});

	test("returns null for a snapshot line without a readings array", () => {
		expect(parseSnapshot('{"v":1,"op":"snapshot"}')).toBeNull();
	});
});

describe("band", () => {
	test("is green below 70", () => {
		expect(band(0)).toBe("green");
		expect(band(69)).toBe("green");
	});

	test("is amber from 70 and below 90", () => {
		expect(band(70)).toBe("amber");
		expect(band(89)).toBe("amber");
	});

	test("is red from 90", () => {
		expect(band(90)).toBe("red");
		expect(band(100)).toBe("red");
	});
});

describe("urgent", () => {
	const agent = (used?: number): QuotaAgent => ({
		id: "claude",
		state: "ok",
		...(used === undefined ? {} : { used })
	});

	test("is false when used is absent", () => {
		expect(urgent(agent())).toBe(false);
	});

	test("is false below 90", () => {
		expect(urgent(agent(0))).toBe(false);
		expect(urgent(agent(89))).toBe(false);
	});

	test("is true at 90", () => {
		expect(urgent(agent(90))).toBe(true);
	});
});

describe("quotaView", () => {
	// One fixed now keeps every countdown in these tests deterministic.
	const NOW = new Date("2026-10-22T13:30:00Z");

	test("quota_hidden_on_rest_until_urgent", () => {
		const calm = parseSnapshot(SNAPSHOT)!;
		expect(calm.every((r) => typeof r.used !== "number" || r.used < 90)).toBe(true);
		expect(quotaView(calm, "status", NOW).showInRest).toBe(false);

		const hot = calm.map((r) => (r.id === "claude" ? { ...r, used: 95 } : r));
		expect(quotaView(hot, "status", NOW).showInRest).toBe(true);
		// Already showing the quotas panel is not a rest display.
		expect(quotaView(hot, "quotas", NOW).showInRest).toBe(false);
	});

	test("quota_alert_shows_only_while_the_collapsed_notch_is_hovered", () => {
		expect(quotaAlertVisible(true, false, false)).toBe(false);
		expect(quotaAlertVisible(true, true, false)).toBe(true);
		expect(quotaAlertVisible(true, true, true)).toBe(false);
		expect(quotaAlertVisible(false, true, false)).toBe(false);
	});

	test("countdown_and_clock_share_one_instant", () => {
		const readings: QuotaAgent[] = [
			{
				id: "claude",
				state: "ok",
				used: 42.5,
				reset5h: "2026-10-22T15:00:00Z",
				details: [
					{ label: "5h", value: "42.5% used", resetAt: "2026-10-22T15:00:00Z" }
				]
			}
		];
		const row = quotaView(readings, "quotas", NOW).panel[0];
		const instant = new Date("2026-10-22T15:00:00Z");
		const clock = instant.toLocaleTimeString([], {
			hour: "numeric",
			minute: "2-digit"
		});
		// 15:00Z is 90 minutes past NOW; the row's countdown and clock are
		// both that one reset5h instant, and the detail's both its resetAt.
		expect(row.reset?.countdown).toBe("1h 30m");
		expect(row.reset?.clock).toBe(clock);
		expect(row.details[0].reset?.countdown).toBe("1h 30m");
		expect(row.details[0].reset?.clock).toBe(clock);
	});
});

describe("overviewRows", () => {
	test("overview_lists_only_live_activities", () => {
		expect(
			overviewRows({ music: false, timer: false, shelfCount: 0, urgent: false })
		).toEqual([]);
		expect(
			overviewRows({ music: true, timer: false, shelfCount: 0, urgent: false })
		).toEqual(["Music"]);
		expect(
			overviewRows({ music: false, timer: true, shelfCount: 2, urgent: true })
		).toEqual(["Timer", "Shelf", "Quota"]);
		expect(
			overviewRows({ music: true, timer: true, shelfCount: 1, urgent: true })
		).toEqual(["Music", "Timer", "Shelf", "Quota"]);
	});

	test("overview_is_not_the_default_panel", () => {
		expect(defaultPanel()).toBe(DEFAULT_PANEL);
		expect(DEFAULT_PANEL).not.toBe("overview");
	});
});

describe("alertRows", () => {
	const NOW = new Date("2026-10-22T13:30:00Z");

	test("keeps only readings at or above 90, in snapshot order", () => {
		const readings = parseSnapshot(SNAPSHOT)!;
		const hot = readings.map((r) =>
			r.id === "claude" ? { ...r, used: 95 } : r.id === "codex" ? { ...r, used: 91 } : r
		);
		const rows = alertRows(hot, NOW);
		expect(rows.map((r) => r.id)).toEqual(["claude", "codex"]);
	});

	test("is empty when nothing is urgent", () => {
		const readings = parseSnapshot(SNAPSHOT)!;
		const warm = readings.map((r) => (r.id === "claude" ? { ...r, used: 89.9 } : r));
		expect(warm.some((r) => typeof r.used === "number" && r.used >= 90)).toBe(false);
		expect(alertRows(warm, NOW)).toHaveLength(0);
	});

	test("carries the band and reset the strip draws", () => {
		const readings: QuotaAgent[] = [
			{ id: "claude", state: "ok", used: 95, reset5h: "2026-10-22T15:00:00Z" }
		];
		const row = alertRows(readings, NOW)[0];
		expect(row.band).toBe("red");
		expect(row.reset?.countdown).toBe("1h 30m");
	});
});

describe("hoverQuotaAgent", () => {
	const agent = (id: string, used?: number, extra: Partial<QuotaAgent> = {}): QuotaAgent => ({
		id,
		state: "ok",
		...(used === undefined ? {} : { used }),
		...extra
	});

	test("returns undefined when there are no readings", () => {
		expect(hoverQuotaAgent([])).toBeUndefined();
		expect(hoverQuotaAgent([], "cursor")).toBeUndefined();
	});

	test("honors a pinned selection ahead of the busiest reading", () => {
		const readings = [agent("codex", 10), agent("cursor", 71), agent("devin", 56)];
		expect(hoverQuotaAgent(readings, "devin")?.id).toBe("devin");
	});

	test("defaults to the highest-used agent", () => {
		const readings = [agent("codex", 10), agent("cursor", 71), agent("devin", 56)];
		expect(hoverQuotaAgent(readings)?.id).toBe("cursor");
		// An unknown selection is the same as none.
		expect(hoverQuotaAgent(readings, "kiro")?.id).toBe("cursor");
	});

	test("keeps the earlier agent on a tie", () => {
		const readings = [agent("codex", 40), agent("cursor", 40)];
		expect(hoverQuotaAgent(readings)?.id).toBe("codex");
	});

	test("a grouped 5h pool counts toward the headline usage", () => {
		const readings = [
			agent("cursor", 71),
			agent("antigravity", 0, {
				groups: [{ name: "g", windows: [{ label: "5h", used: 80 }] }]
			})
		];
		expect(hoverQuotaAgent(readings)?.id).toBe("antigravity");
	});

	test("a reported zero still beats no number at all", () => {
		const readings: QuotaAgent[] = [
			{ id: "claude", state: "failed" },
			agent("antigravity", 0)
		];
		expect(hoverQuotaAgent(readings)?.id).toBe("antigravity");
	});

	test("falls back to the default selection when nothing is numeric", () => {
		const readings: QuotaAgent[] = [
			{ id: "kiro", state: "waiting" },
			{ id: "claude", state: "failed" }
		];
		// defaultSelection prefers an urgent or failed agent.
		expect(hoverQuotaAgent(readings)?.id).toBe("claude");
	});
});

describe("agentRings", () => {
	test("pools 5h and week windows into inner and outer", () => {
		const a: QuotaAgent = {
			id: "codex",
			state: "ok",
			used: 10,
			windows: [
				{ label: "5h", used: 10 },
				{ label: "week", used: 40 }
			]
		};
		expect(agentRings(a)).toEqual({ inner: 10, outer: 40 });
	});

	test("pairs a day and a week window like Devin's", () => {
		const a: QuotaAgent = {
			id: "devin",
			state: "ok",
			used: 56,
			windows: [
				{ label: "week", used: 28 },
				{ label: "day", used: 56 }
			]
		};
		expect(agentRings(a)).toEqual({ inner: 56, outer: 28 });
	});

	test("reads pool windows out of model groups", () => {
		const a: QuotaAgent = {
			id: "antigravity",
			state: "ok",
			groups: [
				{ name: "g", windows: [{ label: "5h", used: 33 }] },
				{ name: "h", windows: [{ label: "week", used: 50 }] }
			]
		};
		expect(agentRings(a)).toEqual({ inner: 33, outer: 50 });
	});

	test("two distinct windows still draw both rings", () => {
		const a: QuotaAgent = {
			id: "kiro",
			state: "ok",
			windows: [
				{ label: "credits", used: 62 },
				{ label: "month", used: 20 }
			]
		};
		expect(agentRings(a)).toEqual({ inner: 62, outer: 20 });
	});

	test("a single metric leaves the outer ring empty", () => {
		expect(agentRings({ id: "cursor", state: "ok", used: 71 })).toEqual({
			inner: 71,
			outer: undefined
		});
	});

	test("waiting and failed agents draw two empty tracks", () => {
		expect(agentRings({ id: "kiro", state: "waiting" })).toEqual({
			inner: undefined,
			outer: undefined
		});
		expect(agentRings({ id: "claude", state: "failed", used: 95 })).toEqual({
			inner: undefined,
			outer: undefined
		});
	});
});
