import { describe, expect, test } from "bun:test";
import type { QuotaAgent } from "./protocol";
import { fitQuotaStrip, housingWidth, stripSecond, worstBand } from "./stripFit";

const agent = (id: string, used: number, week?: number): QuotaAgent => ({
	id,
	state: "ok",
	used,
	windows:
		week === undefined
			? undefined
			: [
					{ label: "5h", used },
					{ label: "week", used: week }
				]
});

const seven = [
	agent("claude", 26, 3),
	agent("codex", 64, 12),
	agent("cursor", 0),
	agent("antigravity", 100),
	agent("grokbot", 9),
	agent("commandcode", 59, 20),
	agent("qoder", 100)
];

describe("fitQuotaStrip", () => {
	test("gives up detail in order as the room falls", () => {
		const stages = [1000, 420, 300, 220, 120, 20].map((room) => fitQuotaStrip(seven, room).stage);
		expect(stages).toEqual(["full", "value", "hot", "marks", "top", "dot"]);
	});

	test("every chosen stage fits the room it was given", () => {
		for (let room = 0; room <= 700; room += 7) {
			const fit = fitQuotaStrip(seven, room);
			if (fit.stage === "dot") continue;
			const more = fit.hidden > 0 ? 30 : 0;
			expect(housingWidth(fit.shown, fit.stage === "top" ? "marks" : fit.stage) + more).toBeLessThanOrEqual(room);
		}
	});

	test("the fullest agents stay when some are folded into +N", () => {
		const fit = fitQuotaStrip(seven, 100);
		expect(fit.stage).toBe("top");
		expect(fit.shown.map((a) => a.id)).toEqual(["antigravity", "qoder"]);
		expect(fit.hidden).toBe(5);
	});

	test("unbounded room shows everything", () => {
		expect(fitQuotaStrip(seven, Infinity)).toEqual({ stage: "full", shown: seven, hidden: 0 });
	});
});

test("the second number is the lower of two windows", () => {
	expect(stripSecond(seven[0])).toBe(3);
	expect(stripSecond(seven[2])).toBeUndefined();
});

test("the dot takes the band of the fullest agent", () => {
	expect(worstBand(seven)).toBe("red");
	expect(worstBand([agent("claude", 26)])).toBe("green");
	expect(worstBand([{ id: "claude", state: "failed" }])).toBeUndefined();
});
