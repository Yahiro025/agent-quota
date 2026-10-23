import { describe, expect, test } from "bun:test";
import { AgentMark, agentName, initials } from "./index";

const IDS = [
	"claude",
	"kiro",
	"codex",
	"cursor",
	"copilot",
	"devin",
	"antigravity"
];

describe("agentName", () => {
	test("gives the seven exact names", () => {
		expect(IDS.map(agentName)).toEqual([
			"Claude",
			"Kiro",
			"Codex",
			"Cursor",
			"Copilot",
			"Devin",
			"Antigravity"
		]);
	});
});

describe("initials", () => {
	test("capitalises the first letter, matching marks.Initials", () => {
		expect(initials("foo")).toBe("F");
		expect(initials("")).toBe("?");
	});
});

describe("AgentMark", () => {
	test("renders a mark for every agent and falls back for unknown ids", () => {
		for (const id of [...IDS, "foo"]) {
			const el = AgentMark({ id });
			expect(el).toBeTruthy();
			expect(el.props["aria-label"]).toBe(agentName(id));
		}
	});
});
