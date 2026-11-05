import { describe, expect, test } from "bun:test";
import type { QuotaAgent } from "./protocol";
import {
	parseHiddenAgents,
	toggleHiddenAgent,
	visibleReadings
} from "./agentVisibility";

const agent = (id: string): QuotaAgent => ({ id, state: "ok" });

describe("parseHiddenAgents", () => {
	test("reads the persisted JSON array", () => {
		expect(parseHiddenAgents('["cursor","devin"]')).toEqual(["cursor", "devin"]);
	});

	test("reads an already-parsed array", () => {
		expect(parseHiddenAgents(["cursor"])).toEqual(["cursor"]);
	});

	test("treats absent, blank and invalid values as nothing hidden", () => {
		expect(parseHiddenAgents(null)).toEqual([]);
		expect(parseHiddenAgents(undefined)).toEqual([]);
		expect(parseHiddenAgents("")).toEqual([]);
		expect(parseHiddenAgents("not json")).toEqual([]);
		expect(parseHiddenAgents('{"cursor":true}')).toEqual([]);
	});

	test("drops non-string entries", () => {
		expect(parseHiddenAgents('["cursor",3,null,""]')).toEqual(["cursor"]);
	});
});

describe("toggleHiddenAgent", () => {
	test("hides an agent that is showing", () => {
		expect(toggleHiddenAgent([], "cursor")).toEqual(["cursor"]);
	});

	test("shows an agent that is hidden without reordering the rest", () => {
		expect(toggleHiddenAgent(["cursor", "devin"], "cursor")).toEqual(["devin"]);
	});
});

describe("visibleReadings", () => {
	test("keeps snapshot order", () => {
		const readings = [agent("claude"), agent("copilot"), agent("devin")];
		expect(visibleReadings(readings, ["copilot"]).map((a) => a.id)).toEqual([
			"claude",
			"devin"
		]);
	});

	test("hiding every agent yields an empty panel", () => {
		expect(visibleReadings([agent("claude")], ["claude"])).toEqual([]);
	});

	test("an empty hidden list returns the same array", () => {
		const readings = [agent("claude")];
		expect(visibleReadings(readings, [])).toBe(readings);
	});
});
