import { describe, expect, test } from "bun:test";
import { AgentMark, agentName, initials } from "./index";

const IDS = [
	"claude",
	"kiro",
	"codex",
	"cursor",
	"copilot",
	"devin",
	"antigravity",
	"grokbot",
	"commandcode",
	"qoder"
];

describe("agentName", () => {
	test("gives the ten exact names", () => {
		expect(IDS.map(agentName)).toEqual([
			"Claude",
			"Kiro",
			"Codex",
			"Cursor",
			"Copilot",
			"Devin",
			"Antigravity",
			"Grok Bot",
			"Command Code",
			"Qoder"
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
