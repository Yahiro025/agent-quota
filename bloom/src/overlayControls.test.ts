import { describe, expect, test } from "bun:test";
import {
	controlsCardWidth,
	controlsCardRect,
	expandedLeftEdge,
	CONTROL_COLUMN_WIDTH,
	CONTROLS_CARD_HEIGHT
} from "./overlayControls";

describe("controls card geometry", () => {
	test("width follows enabled columns", () => {
		expect(controlsCardWidth(true, true)).toBe(2 * CONTROL_COLUMN_WIDTH);
		expect(controlsCardWidth(true, false)).toBe(CONTROL_COLUMN_WIDTH);
		expect(controlsCardWidth(false, true)).toBe(CONTROL_COLUMN_WIDTH);
		expect(controlsCardWidth(false, false)).toBe(0);
	});

	test("card is anchored to the right edge and vertically centered", () => {
		const w = controlsCardWidth(true, true);
		const r = controlsCardRect(1280, 800, w);
		expect(r.x + r.width).toBe(1280); // flush with the right edge
		expect(r.x).toBe(1280 - w);
		expect(r.y + r.height / 2).toBe(400);
		expect(r.height).toBe(CONTROLS_CARD_HEIGHT);
	});

	test("mixer expansion grows leftward and stays on-screen", () => {
		const w = controlsCardWidth(true, true);
		const left = expandedLeftEdge(1280, w, 204);
		expect(left).toBe(1280 - w - 204);
		expect(left).toBeGreaterThanOrEqual(0);
	});
});
