import { expect, test } from "bun:test";
import { formatRemaining, formatTime } from "./time";

test("clocks read m:ss below an hour and h:mm:ss above it", () => {
	expect(formatTime(0)).toBe("0:00");
	expect(formatTime(65_000)).toBe("1:05");
	expect(formatTime(3_599_000)).toBe("59:59");
	expect(formatTime(3_600_000)).toBe("1:00:00");
	expect(formatTime(3_725_000)).toBe("1:02:05");
	expect(formatTime(-500)).toBe("0:00");
});

test("remaining time carries the true minus sign and never goes below zero", () => {
	expect(formatRemaining(10_000, 70_000)).toBe("\u22121:00");
	expect(formatRemaining(0, 3_725_000)).toBe("\u22121:02:05");
	expect(formatRemaining(90_000, 60_000)).toBe("\u22120:00");
});
