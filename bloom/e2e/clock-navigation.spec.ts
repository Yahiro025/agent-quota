import { test, expect } from "@playwright/test";
import path from "node:path";
test("installed clock navigation cycles panels without arrows", async ({ page }) => {
	await page.addInitScript({ path: path.join(import.meta.dirname, "tauri-mock.js") });
	await page.addInitScript(() => {
		localStorage.setItem("bloom-notch-default-panel", "calendar");
		localStorage.setItem("bloom-notch-remember-panel", "false");
	});
	await page.goto("/");
	await page.waitForTimeout(1000);
	const clock = page.locator(".time-flip-container");
	await clock.click();
	await expect(page.locator(".calendar-timer-content")).toBeVisible();
	await clock.click();
	await expect(page.locator(".command-center-content-minimal")).toBeVisible();
	await clock.click();
	await expect(page.getByTestId("quota-space")).toBeVisible();
	await clock.click();
	await expect(page.getByTestId("quota-space")).toHaveCount(0);
	await expect(page.locator(".qp")).toBeVisible();
	await expect(page.locator(".panel-arrow")).toHaveCount(0);
});
