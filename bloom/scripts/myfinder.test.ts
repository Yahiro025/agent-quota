import { describe, expect, test } from "bun:test";
import {
	constrainBarRegions,
	fittedBarHeight,
	myfinderIdleHidden,
	myfinderMaterial,
	NO_MYFINDER
} from "../src/myfinder";
import { nextRegion, type RegionState } from "../src/windowRegion";

describe("MyFinder fit", () => {
	test("sampled light/dark materials retain text contrast across the transparency fallback", () => {
		const luminance = (rgb: number[]) =>
			rgb
				.map((v) => v / 255)
				.map((v) => (v <= 0.04045 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4))
				.reduce((sum, v, i) => sum + v * [0.2126, 0.7152, 0.0722][i], 0);
		const hex = (value: string) => [1, 3, 5].map((i) => parseInt(value.slice(i, i + 2), 16));
		const ratio = (a: number[], b: number[]) =>
			(Math.max(luminance(a), luminance(b)) + 0.05) / (Math.min(luminance(a), luminance(b)) + 0.05);
		for (const level of [0, 24, 64, 100, 117, 118, 128, 170, 230, 255]) {
			const rgb = [level, level, level] as [number, number, number];
			const { style } = myfinderMaterial(rgb);
			expect(ratio(rgb, hex(style["--myfinder-strip-ink"]))).toBeGreaterThanOrEqual(4.5);
			const panel = style["--bloom-bg"]
				.match(/[\d.]+/g)!
				.slice(0, 3)
				.map(Number);
			for (const behind of [0, 255]) {
				const background = panel.map((v) => v * 0.96 + behind * 0.04);
				for (const key of ["--bloom-text", "--bloom-text-muted"] as const)
					expect(ratio(background, hex(style[key]))).toBeGreaterThanOrEqual(4.5);
			}
		}
	});
	test("30 physical pixels at 125% are 24 local pixels; larger user scale keeps readability", () => {
		expect(fittedBarHeight(30, 1.25, 1)).toBe(24);
		for (const dpi of [1, 1.25, 1.5, 2]) expect(fittedBarHeight(24 * dpi, dpi, 1)).toBe(24);
		expect(fittedBarHeight(30, 1.25, 1.5)).toBe(24);
		expect(fittedBarHeight(60, 1.25, 1)).toBe(48);
	});
	test("expanded and held regions never claim neighbouring bar-row points", () => {
		const strip = { x: 400, y: 0, width: 200, height: 24 };
		const body = { x: 250, y: 32, width: 500, height: 300 };
		let state: RegionState = { key: "", current: [], held: [] };
		for (const [now, rects] of [
			[0, [strip]],
			[100, [strip, body]],
			[300, [strip]],
			[1001, [strip]]
		] as const) {
			const next = nextRegion(state, [...rects], now, 700);
			state = next.state;
			const safe = constrainBarRegions(next.rects, strip, 32);
			const contains = (x: number, y: number) =>
				safe.some((r) => x >= r.x && x < r.x + r.width && y >= r.y && y < r.y + r.height);
			expect(contains(300, 12)).toBe(false);
			expect(contains(700, 12)).toBe(false);
			expect(contains(450, 12)).toBe(true);
		}
		// Even an old single-box region held across a mode/DPI change is bounded.
		expect(constrainBarRegions([{ x: 200, y: 0, width: 600, height: 300 }], strip, 32)[0]).toEqual(
			strip
		);
	});
	test("the hanging badge keeps its overhang while the gap beside it stays out", () => {
		const strip = { x: 400, y: 0, width: 200, height: 32 };
		const bar = { x: 400, y: 0, width: 200, height: 24 };
		const badge = { x: 410, y: 0, width: 32, height: 32 };
		const safe = constrainBarRegions([bar, badge], strip, 32);
		expect(safe).toContainEqual(badge);
		const contains = (x: number, y: number) =>
			safe.some((r) => x >= r.x && x < r.x + r.width && y >= r.y && y < r.y + r.height);
		expect(contains(426, 28)).toBe(true);
		expect(contains(460, 28)).toBe(false);
	});
	test("unknown uses standalone behavior; known hidden gates idle but retains active panels", () => {
		expect(myfinderIdleHidden(NO_MYFINDER, false, false)).toBe(false);
		const hidden = { ...NO_MYFINDER, available: true, hidden: true, height: 30 };
		expect(myfinderIdleHidden(hidden, false, false)).toBe(true);
		expect(myfinderIdleHidden(hidden, true, false)).toBe(false);
		expect(myfinderIdleHidden(hidden, false, true)).toBe(false);
		expect(myfinderIdleHidden({ ...hidden, hidden: false }, false, false)).toBe(false);
	});
	test("light and dark bars produce readable corresponding materials without changing saved preferences", () => {
		expect(myfinderMaterial([170, 160, 184]).light).toBe(true);
		expect(myfinderMaterial([24, 26, 29]).light).toBe(false);
		expect(myfinderMaterial(null).style["--bloom-text"]).toBe("#202428");
	});
});
