import { test, expect } from "@playwright/test";
import path from "node:path";

const MOCK = path.join(import.meta.dirname, "tauri-mock.js");

// The widget layout saved from Settings must drive the collapsed notch. Before
// this test, App.tsx rendered a hard-coded weather/battery pair and ignored
// bloom-status-widgets entirely.
test("renders the saved left and right widgets in the resting notch", async ({ page }) => {
	await page.addInitScript({ path: MOCK });
	await page.addInitScript(() => {
		localStorage.setItem(
			"bloom-status-widgets",
			JSON.stringify({ left: ["cpu", "weather"], right: ["ram", "net"] })
		);
	});

	await page.goto("/");
	await expect(page.locator("div.bloom")).toBeVisible();
	await page.waitForTimeout(900);

	await page.locator("div.bloom").hover();
	await expect(page.locator(".pill-cluster")).toBeVisible();

	// Weather stays enabled in this layout but has no reading in the mock, so
	// it renders nothing: CPU left, RAM and Net right, and no battery.
	const widgets = page.locator(".pill-cluster .passive-feature");
	await expect(widgets).toHaveCount(3);
	await expect(widgets.nth(0).locator(".label")).toHaveText("42%");
	await expect(widgets.nth(1).locator(".label")).toHaveText("55%");
	await expect(widgets.nth(2).locator(".label")).toHaveText("↑1.0K ↓2.0K");

	const time = page.locator(".pill-cluster .time-center");
	const cpuBox = (await widgets.nth(0).boundingBox())!;
	const timeBox = (await time.boundingBox())!;
	const ramBox = (await widgets.nth(1).boundingBox())!;
	expect(cpuBox.x + cpuBox.width).toBeLessThanOrEqual(timeBox.x + 0.5);
	expect(timeBox.x + timeBox.width).toBeLessThanOrEqual(ramBox.x + 0.5);
});

// The pill must cover the row it draws. Zeroing the measured width or freezing
// it left the resting notch narrower than the clock, which clipped it.
test("the resting pill covers the clock", async ({ page }) => {
	await page.addInitScript({ path: MOCK });
	await page.addInitScript(() => {
		localStorage.setItem("bloom-status-widgets", JSON.stringify({ left: ["cpu"], right: ["net"] }));
	});

	await page.goto("/");
	await expect(page.locator("div.bloom")).toBeVisible();
	await page.waitForTimeout(1200);

	const pill = (await page.locator("div.bloom").boundingBox())!;
	const time = (await page.locator(".pill-cluster .time-center").boundingBox())!;
	expect(time.x + time.width).toBeLessThanOrEqual(pill.x + pill.width + 0.5);
});

// The notch background is painted by --notch-bg. Hard-coding black there made
// the Appearance opacity slider a no-op.
test("applies the saved background opacity to the notch", async ({ page }) => {
	await page.addInitScript({ path: MOCK });
	await page.addInitScript(() => {
		localStorage.setItem("bloom-theme-mode", "dark");
		localStorage.setItem("bloom-theme-opacity", "0.1");
	});

	await page.goto("/");
	await expect(page.locator("div.bloom")).toBeVisible();
	await page.waitForTimeout(900);

	const notchBg = await page
		.locator("div.bloom")
		.evaluate((el) => getComputedStyle(el).getPropertyValue("--notch-bg").trim());
	expect(notchBg).toContain("0.1");
});
