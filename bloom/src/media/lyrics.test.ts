import { expect, test } from "bun:test";
import { activeLyricLine } from "./lyrics";

const lines = [
	{ t: 0, text: "intro" },
	{ t: 5000, text: "verse" },
	{ t: 12000, text: "chorus" }
];

test("active line is the last start at or before the position", () => {
	expect(activeLyricLine(lines, 0)).toBe(0);
	expect(activeLyricLine(lines, 4999)).toBe(0);
	expect(activeLyricLine(lines, 5000)).toBe(1);
	expect(activeLyricLine(lines, 99999)).toBe(2);
});

test("before the first start there is no active line", () => {
	expect(activeLyricLine([{ t: 3000, text: "later" }], 2999)).toBe(-1);
	expect(activeLyricLine([], 1000)).toBe(-1);
});
