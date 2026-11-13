import { describe, expect, test } from "bun:test";
import {
	areRectsEqual,
	findSelectedElement,
	hasSizeChanged,
	measureElement
} from "./useSlidingPad";

describe("useSlidingPad helpers", () => {
	test("findSelectedElement finds element by data-key and selectors", () => {
		const child1 = { id: "c1" } as HTMLElement;
		const child2 = { id: "c2" } as HTMLElement;

		const container = {
			querySelector: (selector: string) => {
				if (selector.includes('[data-key="codex"]')) return child2;
				if (selector.includes('[data-key="claude"]')) return child1;
				if (selector.includes('[data-pad-key="devin"]')) return child1;
				if (selector.includes('.selected')) return child2;
				return null;
			}
		} as unknown as HTMLElement;

		expect(findSelectedElement(container, "codex")).toBe(child2);
		expect(findSelectedElement(container, "claude")).toBe(child1);
		expect(findSelectedElement(container, "devin")).toBe(child1);
		expect(findSelectedElement(container, "cursor")).toBe(child2); // falls back to .selected
	});

	test("findSelectedElement returns null when selector matches nothing", () => {
		const container = {
			querySelector: () => null
		} as unknown as HTMLElement;

		expect(findSelectedElement(container, "nonexistent")).toBeNull();
		expect(findSelectedElement(container)).toBeNull();
	});

	test("measureElement extracts layout metrics unaffected by transforms", () => {
		const el = {
			offsetLeft: 120,
			offsetTop: 8,
			offsetWidth: 64,
			offsetHeight: 72
		} as HTMLElement;

		const rect = measureElement(el);
		expect(rect).toEqual({
			x: 120,
			y: 8,
			width: 64,
			height: 72
		});
	});

	test("areRectsEqual compares rect dimensions accurately", () => {
		const r1 = { x: 10, y: 20, width: 30, height: 40 };
		const r2 = { x: 10, y: 20, width: 30, height: 40 };
		const r3 = { x: 11, y: 20, width: 30, height: 40 };
		expect(areRectsEqual(r1, r2)).toBe(true);
		expect(areRectsEqual(r1, r3)).toBe(false);
		expect(areRectsEqual(null, null)).toBe(true);
		expect(areRectsEqual(r1, null)).toBe(false);
		expect(areRectsEqual(null, r1)).toBe(false);
	});

	test("hasSizeChanged ignores initial observation and detects size differences", () => {
		const size1 = { width: 100, height: 50 };
		const size2 = { width: 100, height: 50 };
		const size3 = { width: 120, height: 50 };
		const size4 = { width: 100, height: 60 };

		// Initial observation (prev is null) returns false
		expect(hasSizeChanged(null, size1)).toBe(false);
		// Identical size returns false
		expect(hasSizeChanged(size1, size2)).toBe(false);
		// Width changed returns true
		expect(hasSizeChanged(size1, size3)).toBe(true);
		// Height changed returns true
		expect(hasSizeChanged(size1, size4)).toBe(true);
	});
});
