import { test, expect, type Page } from "@playwright/test";
import path from "node:path";

const MOCK = path.join(import.meta.dirname, "tauri-mock.js");

const emit = (page: Page, event: string, payload: unknown) =>
	page.evaluate(([e, p]) => (window as any).__emit(e, p), [event, payload]);

const invokes = (page: Page): Promise<{ cmd: string; args: any }[]> =>
	page.evaluate(() => (window as any).__invokes);

const card = (page: Page) => page.locator(".controls-notch-wrapper");

test.beforeEach(async ({ page }) => {
	await page.addInitScript({ path: MOCK });
	// Give the mixer one audio session so expansion renders a tile.
	await page.addInitScript(() => {
		const invoke = (window as any).__TAURI_INTERNALS__.invoke;
		(window as any).__TAURI_INTERNALS__.invoke = (cmd: string, args: any) =>
			cmd === "get_audio_sessions"
				? Promise.resolve([
						{ pid: 4242, name: "Music", process_path: null, volume: 0.5, is_muted: false }
					])
				: invoke(cmd, args);
	});
	await page.goto("/overlay.html");
	await page.waitForTimeout(400);
});

test("edge hover reveals one card with both sliders on the right edge", async ({ page }) => {
	await emit(page, "controls-edge-hover", true);
	await expect(card(page)).toBeVisible();
	await page.waitForTimeout(700);

	// One surface, two sliders.
	await expect(page.locator(".controls-notch-bar")).toHaveCount(2);
	await expect(page.locator(".brightness-notch-icon")).toBeVisible();
	await expect(page.locator(".volume-notch-action")).toBeVisible();

	const box = (await card(page).boundingBox())!;
	// Flush with the right edge; nothing reveals on the left edge.
	expect(box.x + box.width).toBeGreaterThanOrEqual(1279);
	expect(box.x).toBeGreaterThan(1000);
});

test("hardware volume change shows the same right-edge card", async ({ page }) => {
	await emit(page, "volume-change", { volume: 0.7, is_muted: false });
	await expect(card(page)).toBeVisible();
	await expect(page.locator(".controls-notch-bar")).toHaveCount(2);
	const box = (await card(page).boundingBox())!;
	expect(box.x + box.width).toBeGreaterThanOrEqual(1279);
});

test("leaving the edge hides the card", async ({ page }) => {
	await emit(page, "controls-edge-hover", true);
	await expect(card(page)).toBeVisible();
	await emit(page, "controls-edge-hover", false);
	await page.waitForTimeout(2200);
	await expect(card(page)).toHaveCount(0);
});

test("mixer expands inward from the right edge and stays on-screen", async ({ page }) => {
	await emit(page, "controls-edge-hover", true);
	await expect(card(page)).toBeVisible();
	await page.locator(".volume-notch-action").click();
	await expect(page.locator(".volume-mixer-panel")).toBeVisible();
	await page.waitForTimeout(700);

	const cardBox = (await card(page).boundingBox())!;
	const panel = (await page.locator(".volume-mixer-panel").boundingBox())!;
	// The panel sits left of the card columns and remains fully on-screen.
	expect(panel.x).toBeLessThan(cardBox.x + cardBox.width - 42);
	expect(panel.x).toBeGreaterThanOrEqual(0);
	expect(cardBox.x + cardBox.width).toBeGreaterThanOrEqual(1279);
	// The reported native hit rect covers panel + card on the right.
	const rects = (await invokes(page))
		.filter((i) => i.cmd === "set_volume_mixer_rect")
		.map((i) => i.args);
	expect(rects.length).toBeGreaterThan(0);
	const last = rects[rects.length - 1];
	expect(last.x + last.width).toBeGreaterThanOrEqual(1279);
});

test("dragging each slider sends its backend command", async ({ page }) => {
	await emit(page, "controls-edge-hover", true);
	await expect(card(page)).toBeVisible();
	await page.waitForTimeout(700);
	const bars = page.locator(".controls-notch-bar");
	const volBox = (await bars.nth(1).boundingBox())!;
	// Drag the volume bar to ~half height.
	await page.mouse.move(volBox.x + volBox.width / 2, volBox.y + volBox.height - 2);
	await page.mouse.down();
	await page.mouse.move(volBox.x + volBox.width / 2, volBox.y + volBox.height / 2, { steps: 3 });
	await page.mouse.up();
	await page.waitForTimeout(150);
	const vols = (await invokes(page)).filter((i) => i.cmd === "set_volume");
	expect(vols.length).toBeGreaterThan(0);
	expect(vols[vols.length - 1].args.volume).toBeGreaterThan(0.2);
	expect(vols[vols.length - 1].args.volume).toBeLessThan(0.9);

	const brightBox = (await bars.nth(0).boundingBox())!;
	await page.mouse.move(brightBox.x + brightBox.width / 2, brightBox.y + brightBox.height - 2);
	await page.mouse.down();
	await page.mouse.move(brightBox.x + brightBox.width / 2, brightBox.y + 4, { steps: 3 });
	await page.mouse.up();
	await page.waitForTimeout(150);
	const brights = (await invokes(page)).filter((i) => i.cmd === "set_brightness");
	expect(brights.length).toBeGreaterThan(0);
	expect(brights[brights.length - 1].args.brightness).toBeGreaterThan(60);
});
