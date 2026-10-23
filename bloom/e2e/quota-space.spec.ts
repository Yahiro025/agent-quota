import { test, expect, type Page } from "@playwright/test";
import path from "node:path";
import type { QuotaAgent } from "../src/quota/protocol";

const MOCK = path.join(import.meta.dirname, "tauri-mock.js");
const SHOT = path.resolve(
	import.meta.dirname,
	"../../docs/verification/quota-space/quota-space.png"
);

const later = (ms: number) => new Date(Date.now() + ms).toISOString();
const HOUR = 3600_000;

// The reference photo's snapshot: Claude unavailable, Codex 10% on its 5h
// window, Cursor 71% on Plan, Devin 56% Day + 28% Week on a Pro plan, and
// Antigravity idle at 0% on three model groups.
const fixture = (): QuotaAgent[] => [
	{
		id: "claude",
		state: "failed",
		err: "Sign-in expired.",
		help: "Open Claude Code to renew its sign-in, then choose Refresh."
	},
	{
		id: "codex",
		state: "ok",
		used: 10,
		windows: [{ label: "5h", used: 10 }],
		reset5h: later(3.4 * HOUR)
	},
	{ id: "cursor", state: "ok", used: 71 },
	{
		id: "devin",
		state: "ok",
		used: 56,
		plan: "Pro",
		windows: [
			{ label: "week", used: 28 },
			{ label: "day", used: 56 }
		],
		details: [
			{ label: "Day", value: "56% used", resetAt: later(6.3 * HOUR) },
			{ label: "Week", value: "28% used", resetAt: later(2 * 24 * HOUR + 6.3 * HOUR) }
		]
	},
	{
		id: "antigravity",
		state: "ok",
		used: 0,
		reset5h: later(4.9 * HOUR),
		groups: [
			{ name: "Gemini 3 Pro", windows: [{ label: "5h", used: 0 }] },
			{ name: "Claude Sonnet 4.5", windows: [{ label: "5h", used: 0 }] },
			{ name: "GPT-OSS 120B", windows: [{ label: "5h", used: 0 }] }
		]
	}
];

const emitSnapshot = (page: Page) =>
	page.evaluate(
		([ev, pl]) => (window as any).__emit(ev, pl),
		["quota-snapshot", fixture()]
	);

const invokes = (page: Page): Promise<{ cmd: string; args: any }[]> =>
	page.evaluate(() => (window as any).__invokes);

const quotaSpace = (page: Page) => page.getByTestId("quota-space");

// Click the island, then cycle panels until the quota space mounts. The
// remembered default may already be quotas, so this never assumes a count.
const openQuotaSpace = async (page: Page) => {
	await page.locator("div.bloom").click();
	const qs = quotaSpace(page);
	for (let i = 0; i < 4 && (await qs.count()) === 0; i++) {
		await page.locator("button.panel-arrow.next").click();
		await page.waitForTimeout(250);
	}
	await expect(qs).toBeVisible();
};

test.beforeEach(async ({ page }) => {
	await page.addInitScript({ path: MOCK });
	await page.goto("/");
	await expect(page.locator("div.bloom")).toBeVisible();
	// Startup animation: visibility poll -> ready -> expand.
	await page.waitForTimeout(900);
	await emitSnapshot(page);
	await openQuotaSpace(page);
});

test("renders the reference layout", async ({ page }) => {
	const qs = quotaSpace(page);
	// Header caption and the five ring cells, in order.
	await expect(qs.locator(".qs-caption")).toHaveText("Used");
	await expect(page.getByTestId("quota-pin")).toBeVisible();
	await expect(page.getByTestId("quota-close")).toBeVisible();
	await expect(page.getByTestId("quota-cell-claude").locator(".qs-basis")).toHaveText(
		"Unavailable"
	);
	await expect(page.getByTestId("quota-cell-codex").locator(".qs-pct")).toHaveText("10%");
	await expect(page.getByTestId("quota-cell-codex").locator(".qs-basis")).toHaveText("5h");
	await expect(page.getByTestId("quota-cell-cursor").locator(".qs-pct")).toHaveText("71%");
	await expect(page.getByTestId("quota-cell-cursor").locator(".qs-basis")).toHaveText("Plan");
	await expect(page.getByTestId("quota-cell-devin").locator(".qs-pct")).toHaveText("56%");
	await expect(page.getByTestId("quota-cell-devin").locator(".qs-basis")).toHaveText("Day");
	await expect(page.getByTestId("quota-cell-antigravity").locator(".qs-pct")).toHaveText(
		"0%"
	);
	await expect(page.getByTestId("quota-cell-antigravity").locator(".qs-basis")).toHaveText(
		"5h"
	);

	// The photo selects Devin; click its cell to match.
	await page.getByTestId("quota-cell-devin").click();
	const detail = page.getByTestId("quota-detail");
	await expect(detail.locator(".qs-detail-title")).toHaveText("Devin · Pro");
	await expect(detail).toContainText("Day · 56% used · resets in 6h");
	await expect(detail).toContainText("Week · 28% used · resets in 2d 6h");
	await expect(qs.locator(".qs-divider")).toBeVisible();
	await expect(page.getByTestId("quota-failed")).toHaveText("1 failed");
	await expect(page.getByTestId("quota-refresh")).toContainText("Refresh");

	// The panel reports a real height to the window fitter while mounted.
	const heights = (await invokes(page))
		.filter((i) => i.cmd === "set_quota_panel_height")
		.map((i) => i.args?.height);
	expect(heights.some((h) => typeof h === "number" && h > 0)).toBe(true);

	// Screenshot the space itself: `.qs` is the unconstrained content, so the
	// shot keeps the footer the island's scroll cap would crop on screen.
	await qs.screenshot({ path: SHOT });
});

test("the footer stays inside the island", async ({ page }) => {
	// The island grows to hold the status row plus the measured `.qs` extent;
	// the failed/refresh footer is the lowest element, so it proves the fit.
	await page.waitForTimeout(1200);
	const pill = (await page.locator("div.bloom").boundingBox())!;
	const footer = (await page.getByTestId("quota-failed").boundingBox())!;
	const refresh = (await page.getByTestId("quota-refresh").boundingBox())!;
	expect(footer.y + footer.height).toBeLessThanOrEqual(pill.y + pill.height);
	expect(refresh.y + refresh.height).toBeLessThanOrEqual(pill.y + pill.height);
});

test("selecting a ring cell swaps the detail", async ({ page }) => {
	await page.getByTestId("quota-cell-codex").click();
	await expect(page.getByTestId("quota-cell-codex")).toHaveClass(/selected/);
	await expect(page.getByTestId("quota-cell-devin")).not.toHaveClass(/selected/);
	const detail = page.getByTestId("quota-detail");
	await expect(detail.locator(".qs-detail-title")).toHaveText("Codex");
	await expect(detail).toContainText("5h resets in");
});

test("the reset display toggles countdown to clock", async ({ page }) => {
	await page.getByTestId("quota-cell-codex").click();
	const reset = page.getByTestId("quota-reset");
	await expect(reset).toContainText("5h · in");
	await reset.click();
	await expect(reset).toContainText(/5h · at \d{1,2}:\d{2}/);
	await reset.click();
	await expect(reset).toContainText("5h · in");
});

test("refresh and the recovery hint call the helper", async ({ page }) => {
	await page.getByTestId("quota-refresh").click();
	let calls = (await invokes(page)).filter((i) => i.cmd === "refresh_quota");
	expect(calls).toContainEqual({ cmd: "refresh_quota", args: { id: null } });

	// The failed agent's hint is a per-agent refresh button.
	await page.getByTestId("quota-cell-claude").click();
	const hint = page.getByTestId("quota-hint");
	await expect(hint).toContainText("Open Claude Code");
	await hint.click();
	calls = (await invokes(page)).filter((i) => i.cmd === "refresh_quota");
	expect(calls).toContainEqual({ cmd: "refresh_quota", args: { id: "claude" } });
});

test("model groups expand from the toggle", async ({ page }) => {
	await page.getByTestId("quota-cell-antigravity").click();
	const toggle = page.getByTestId("quota-groups-toggle");
	await expect(toggle).toContainText("3 model groups");
	await expect(page.getByTestId("quota-groups")).toHaveCount(0);
	await toggle.click();
	const groups = page.getByTestId("quota-groups");
	await expect(groups).toBeVisible();
	await expect(groups.locator(".qs-group-name")).toHaveText([
		"Gemini 3 Pro",
		"Claude Sonnet 4.5",
		"GPT-OSS 120B"
	]);
});

test("pin holds the notch open through pointer-leave", async ({ page }) => {
	const qs = quotaSpace(page);
	// Sanity: unpinned, leaving the pointer collapses the notch.
	await page.mouse.move(640, 780);
	await expect(qs).toHaveCount(0, { timeout: 4000 });

	await openQuotaSpace(page);
	await page.getByTestId("quota-pin").click();
	await page.mouse.move(640, 780);
	await page.waitForTimeout(700);
	await expect(qs).toBeVisible();

	// Unpinning releases the deferred leave and the notch closes.
	await page.getByTestId("quota-pin").click();
	await expect(qs).toHaveCount(0, { timeout: 4000 });
});

test("close collapses the notch", async ({ page }) => {
	const qs = quotaSpace(page);
	const before = (await page.locator("div.bloom").boundingBox())!;
	await page.getByTestId("quota-close").click();
	await expect(qs).toHaveCount(0, { timeout: 4000 });
	const after = (await page.locator("div.bloom").boundingBox())!;
	expect(after.height).toBeLessThan(before.height);
});

test("controls never take focus", async ({ page }) => {
	// Opening the panel itself can leave a panel arrow focused; the contract
	// is that clicking quota-space controls never moves that focus.
	const active = () =>
		page.evaluate(
			() => `${document.activeElement?.tagName}.${document.activeElement?.className}`
		);
	const before = await active();
	await page.getByTestId("quota-cell-codex").click();
	expect(await active()).toBe(before);
	await page.getByTestId("quota-reset").click();
	expect(await active()).toBe(before);
	await page.getByTestId("quota-refresh").click();
	expect(await active()).toBe(before);
	await page.getByTestId("quota-pin").click();
	expect(await active()).toBe(before);
});
