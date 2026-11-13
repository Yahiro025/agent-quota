import { test, expect } from "@playwright/test";
import path from "node:path";

const mock = path.join(import.meta.dirname, "tauri-mock.js");

test.beforeEach(async ({ page }) => {
	await page.addInitScript({ path: mock });
});

test("clicking the resting pill row opens and cycles the notch panels", async ({ page }) => {
	await page.goto("/");
	await page.waitForTimeout(900);
	const row = page.locator(".main-row");
	await row.click({ position: { x: 6, y: 14 } });
	await expect(page.locator(".calendar-timer-content")).toBeVisible();
	await row.click({ position: { x: 6, y: 14 } });
	await expect(page.getByTestId("quota-space")).toBeVisible();
	await row.click({ position: { x: 6, y: 14 } });
	await expect(page.locator(".qp-body")).toBeVisible();
	await row.click({ position: { x: 6, y: 14 } });
	await expect(page.locator(".calendar-timer-content")).toBeVisible();
	await expect(page.locator(".panel-arrow")).toHaveCount(0);
});

test("starting a timer from the Timer panel shows a countdown in the resting pill, and with no timer the pill shows no time text", async ({
	page
}) => {
	await page.goto("/");
	await page.waitForTimeout(900);

	// No timer running: the time slot stays empty and shows no time text.
	const timeCenter = page.locator(".time-center");
	await expect(timeCenter).toHaveText("");
	await expect(page.locator(".compact-timer")).toHaveCount(0);

	const row = page.locator(".main-row");
	await row.click({ position: { x: 6, y: 14 } });
	await expect(page.locator(".calendar-timer-content")).toBeVisible();
	await page.locator(".timer-btn-play").click();

	// Collapse back to the resting pill and confirm the countdown appears.
	await page.mouse.move(640, 780);
	await expect(page.locator(".calendar-timer-content")).toHaveCount(0, { timeout: 4000 });
	const chip = timeCenter.locator(".compact-timer");
	await expect(chip).toBeVisible();
	await expect(chip).toHaveText(/^\d+:\d{2}$/);
});

test("clicking a finished timer still resets it instead of switching panels", async ({ page }) => {
	await page.addInitScript(() => localStorage.setItem("bloom-timer-last-duration", "1"));
	await page.goto("/");
	await page.waitForTimeout(900);
	const row = page.locator(".main-row");
	await row.click({ position: { x: 6, y: 14 } });
	await expect(page.locator(".calendar-timer-content")).toBeVisible();
	await page.locator(".timer-btn-play").click();
	await expect(page.locator(".timer-finished")).toBeVisible({ timeout: 7000 });
	await page.locator(".time-flip-container").click();
	await expect(page.locator(".timer-finished")).toHaveCount(0);
	await expect(page.locator(".calendar-timer-content")).toBeVisible();
});
