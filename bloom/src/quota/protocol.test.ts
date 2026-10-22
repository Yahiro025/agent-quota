import { describe, expect, test } from "bun:test";
import { band, parseSnapshot, urgent, type QuotaAgent } from "./protocol";
import { alertRows, quotaView } from "./quotaView";

const SNAPSHOT =
	'{"v":1,"op":"snapshot","readings":[' +
	'{"id":"claude","state":"ok","busy":false,"used":42.5,"detail":"5h window","reset5h":"2026-10-22T15:00:00Z"},' +
	'{"id":"kiro","state":"waiting","busy":true},' +
	'{"id":"codex","state":"stale","busy":false,"used":71.5,"err":"timeout","lastGoodAt":"2026-10-22T09:30:00Z"},' +
	'{"id":"copilot","state":"failed","busy":false,"err":"HTTP 401"},' +
	'{"id":"devin","state":"ok","busy":false,"used":0,"plan":"Core","windows":[{"label":"ACU","used":0}],"groups":[{"name":"all","windows":[{"label":"ACU","used":0}]}],"details":[{"label":"ACU","value":"0 used","resetAt":"2026-11-01T00:00:00Z","resetText":"Nov 1"}]}' +
	"]}";

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
				'{"v":1,"op":"ready","agents":["claude","kiro","codex","cursor","copilot","devin","antigravity"]}'
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
