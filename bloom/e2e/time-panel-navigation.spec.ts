import { test, expect } from "@playwright/test";
import path from "node:path";

const mock = path.join(import.meta.dirname, "tauri-mock.js");

test.beforeEach(async ({ page }) => {
	await page.addInitScript({ path: mock });
});

test("clicking the time opens and cycles the notch panels", async ({ page }) => {
	await page.goto("/");
	await page.waitForTimeout(900);
	const time = page.locator(".time-flip-container");
	await time.click();
	await expect(page.locator(".calendar-timer-content")).toBeVisible();
	await time.click();
	await expect(page.locator(".command-center-content-minimal")).toBeVisible();
	await time.click();
	await expect(page.getByTestId("quota-space")).toBeVisible();
	await time.click();
	await expect(page.locator(".qp-body")).toBeVisible();
	await time.click();
	await expect(page.locator(".calendar-timer-content")).toBeVisible();
	await expect(page.locator(".panel-arrow")).toHaveCount(0);
});

test("cycling skips the calendar when it is disabled", async ({ page }) => {
	await page.addInitScript(() => localStorage.setItem("bloom-calendar-enabled", "false"));
	await page.goto("/");
	await page.waitForTimeout(900);
	const time = page.locator(".time-flip-container");
	await time.click();
	await expect(page.locator(".command-center-content-minimal")).toBeVisible();
	await time.click();
	await expect(page.getByTestId("quota-space")).toBeVisible();
	await time.click();
	await expect(page.locator(".qp-body")).toBeVisible();
	await time.click();
	await expect(page.locator(".command-center-content-minimal")).toBeVisible();
	await expect(page.locator(".calendar-timer-content")).toHaveCount(0);
});

test("clicking a finished timer still resets it instead of switching panels", async ({ page }) => {
	await page.addInitScript(() => localStorage.setItem("bloom-timer-last-duration", "1"));
	await page.goto("/");
	await page.waitForTimeout(900);
	const time = page.locator(".time-flip-container");
	await time.click();
	await expect(page.locator(".calendar-timer-content")).toBeVisible();
	await page.locator(".timer-btn-play").click();
	await expect(page.locator(".timer-finished")).toBeVisible({ timeout: 7000 });
	await time.click();
	await expect(page.locator(".timer-finished")).toHaveCount(0);
	await expect(page.locator(".calendar-timer-content")).toBeVisible();
});
