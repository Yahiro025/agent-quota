import { test, expect, type Page } from "@playwright/test";
import path from "node:path";
import type { QuotaAgent } from "../src/quota/protocol";

const MOCK = path.join(import.meta.dirname, "tauri-mock.js");

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

const emitReadings = (page: Page, readings: QuotaAgent[]) =>
	page.evaluate(([ev, pl]) => (window as any).__emit(ev, pl), ["quota-snapshot", readings]);

const emitSnapshot = (page: Page) => emitReadings(page, fixture());

const invokes = (page: Page): Promise<{ cmd: string; args: any }[]> =>
	page.evaluate(() => (window as any).__invokes);

const quotaSpace = (page: Page) => page.getByTestId("quota-space");

// Click the island, then cycle panels until the quota space mounts. The
// remembered default may already be quotas, so this never assumes a count.
const openQuotaSpace = async (page: Page) => {
	await page.locator("div.bloom").click();
	const qs = quotaSpace(page);
	for (let i = 0; i < 4 && (await qs.count()) === 0; i++) {
		await page.locator(".time-flip-container").click();
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

test("renders the reference layout", async ({ page }, testInfo) => {
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
	await expect(page.getByTestId("quota-cell-antigravity").locator(".qs-pct")).toHaveText("0%");
	await expect(page.getByTestId("quota-cell-antigravity").locator(".qs-basis")).toHaveText("5h");

	// The photo selects Devin; click its cell to match.
	await page.getByTestId("quota-cell-devin").click();
	const detail = page.getByTestId("quota-detail");
	await expect(detail.locator(".qs-detail-title")).toHaveText("Devin · Pro");
	const rows = detail.locator(".qs-metric-row");
	await expect(rows).toHaveCount(2);
	await expect(rows.nth(0).locator(".qs-metric-label")).toHaveText("Day");
	await expect(rows.nth(0).locator(".qs-metric-val")).toHaveText("56%");
	await expect(rows.nth(0).locator(".qs-metric-reset")).toContainText("resets in 6h");
	await expect(rows.nth(1).locator(".qs-metric-label")).toHaveText("Week");
	await expect(rows.nth(1).locator(".qs-metric-val")).toHaveText("28%");
	await expect(rows.nth(1).locator(".qs-metric-reset")).toContainText("resets in 2d 6h");
	await expect(qs.locator(".qs-divider")).toHaveCount(0);
	await expect(page.getByTestId("quota-failed")).toHaveCount(0);
	await expect(qs.locator(".qs-header-actions .qs-icon-btn").first()).toBeVisible();
	await expect(page.getByTestId("quota-refresh")).toBeVisible();
	await expect(page.getByTestId("quota-refresh")).toHaveAttribute("aria-label", "Refresh");

	// The panel reports a real height to the window fitter while mounted.
	const heights = (await invokes(page))
		.filter((i) => i.cmd === "set_quota_panel_height")
		.map((i) => i.args?.height);
	expect(heights.some((h) => typeof h === "number" && h > 0)).toBe(true);

	// Screenshot the space itself: `.qs` is the unconstrained content, so the
	// shot keeps the footer the island's scroll cap would crop on screen.
	await qs.screenshot({ path: testInfo.outputPath("quota-space.png") });
});

test("the detail card stays inside the island", async ({ page }) => {
	// The island grows to hold the status row plus the measured `.qs` extent;
	// the detail card is the lowest block, so it proves the fit.
	await page.waitForTimeout(1200);
	const pill = (await page.locator("div.bloom").boundingBox())!;
	const detail = (await page.getByTestId("quota-detail").boundingBox())!;
	expect(detail.y + detail.height).toBeLessThanOrEqual(pill.y + pill.height);
});

test("selecting a ring cell swaps the detail", async ({ page }) => {
	await page.getByTestId("quota-cell-codex").click();
	await expect(page.getByTestId("quota-cell-codex")).toHaveClass(/selected/);
	await expect(page.getByTestId("quota-cell-devin")).not.toHaveClass(/selected/);
	const detail = page.getByTestId("quota-detail");
	await expect(detail.locator(".qs-detail-title")).toHaveText("Codex");
	await expect(detail.locator(".qs-metric-label")).toHaveText("5h");
	await expect(detail.locator(".qs-metric-reset")).toContainText("resets in");
});

test("the reset display toggles countdown to clock", async ({ page }) => {
	await page.getByTestId("quota-cell-codex").click();
	const reset = page.getByTestId("quota-reset");
	const metricReset = page.getByTestId("quota-detail").locator(".qs-metric-reset");
	await expect(reset).toContainText("Countdown");
	await expect(metricReset).toContainText("resets in");
	await reset.click();
	await expect(reset).toContainText("Clock");
	await expect(metricReset).not.toContainText("resets in");
	await expect(metricReset).toContainText(/\d{1,2}:\d{2}/);
	await reset.click();
	await expect(metricReset).toContainText("resets in");
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

test("compact moves the marks into the notch row", async ({ page }) => {
	const before = (await page.locator("div.bloom").boundingBox())!;
	await page.getByTestId("quota-compact").click();
	// The panel is gone; the marks now stand where the CPU/network widgets sit.
	await expect(page.getByTestId("quota-space")).toHaveCount(0);
	const strip = page.getByTestId("notch-quota-strip");
	await expect(strip).toBeVisible();
	await expect(strip.locator("[data-testid^='notch-quota-mark-']")).toHaveCount(5);
	await expect(page.getByTestId("notch-quota-mark-cursor")).toHaveClass(/selected/);
	await expect(page.locator(".time-center")).toBeVisible();
	await expect
		.poll(async () => (await page.locator("div.bloom").boundingBox())!.height)
		.toBeLessThan(before.height);
	expect(
		(await invokes(page)).some(
			(i) =>
				i.cmd === "save_setting" &&
				i.args?.key === "bloom-quota-compact" &&
				i.args?.value === "true"
		)
	).toBe(true);

	// Pinning holds the strip on screen after the pointer leaves.
	await page.getByTestId("notch-quota-pin").click();
	await page.mouse.move(640, 780);
	await page.waitForTimeout(700);
	await expect(strip).toBeVisible();

	// Expanding restores the full panel.
	await page.getByTestId("notch-quota-expand").click();
	await expect(page.getByTestId("quota-space")).toBeVisible();
});

test("hovering a strip mark shows that agent's usage", async ({ page }) => {
	await page.getByTestId("quota-compact").click();
	const mark = page.getByTestId("notch-quota-mark-devin");
	await mark.hover();
	const card = page.getByTestId("quota-hover-card");
	await expect(card).toBeVisible();
	await expect(card).toContainText("Devin");
	await expect(card).toContainText("Day");
	await expect(card).toContainText("56%");
	await expect(card).toContainText("resets in");

	// Leave the mark: the card goes with it.
	await page.mouse.move(640, 780);
	await expect(card).toHaveCount(0);
});

test("controls never take focus", async ({ page }) => {
	// Opening the panel itself can leave the time control focused; the contract
	// is that clicking quota-space controls never moves that focus.
	const active = () =>
		page.evaluate(() => `${document.activeElement?.tagName}.${document.activeElement?.className}`);
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

/* The helper reports up to seven readers. With all of them present the
   island must grow to fit the ring row — never the other way round. */
const SEVEN_IDS = ["claude", "kiro", "codex", "cursor", "copilot", "devin", "antigravity"];

const sevenFixture = (): QuotaAgent[] => [
	{
		id: "claude",
		state: "failed",
		err: "Sign-in expired.",
		help: "Open Claude Code to renew its sign-in, then choose Refresh."
	},
	{
		id: "kiro",
		state: "ok",
		used: 62,
		detail: "Credits",
		windows: [{ label: "credits", used: 62 }]
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
		id: "copilot",
		state: "stale",
		used: 90,
		err: "gh api timed out",
		lastGoodAt: new Date(Date.now() - 42 * 60000).toISOString(),
		observedAt: new Date(Date.now() - 60000).toISOString()
	},
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

// The reference photo's Antigravity card: one row per model group, with the
// full model names that a single-line label lane used to ellipsize.
const HOVER_MODELS = [
	"Gemini 3 Pro (High) / 5h",
	"Gemini 3 Pro (Low) / 5h",
	"Claude Sonnet 4.5 (Thinking) / 5h",
	"Claude Sonnet 4.5 / 5h",
	"Gemini 2.5 Pro / 5h"
];

const hoverModelFixture = (): QuotaAgent[] => {
	const agents = sevenFixture();
	agents[6] = {
		id: "antigravity",
		state: "ok",
		used: 100,
		reset5h: later(4.9 * HOUR),
		details: HOVER_MODELS.map((label, i) => ({
			label,
			value: `${[1, 1, 94, 100, 1][i]}% used`,
			resetAt: later((i + 1) * 6 * HOUR)
		}))
	};
	return agents;
};

// A selected agent whose detail block is far taller than any screen cap.
const longFixture = (): QuotaAgent[] => [
	{
		id: "claude",
		state: "ok",
		used: 12,
		details: Array.from({ length: 60 }, (_, i) => ({
			label: `Window ${i + 1} with a long quota label`,
			value: `${i}% used`,
			resetAt: later((i + 1) * HOUR)
		}))
	},
	...sevenFixture().slice(1)
];

const box = async (page: Page, selector: string) => (await page.locator(selector).boundingBox())!;

// The island's width and position spring together; wait until the rect is
// stable for three samples before asserting geometry.
const settlePill = async (page: Page) => {
	let last = "";
	for (let i = 0; i < 30; i++) {
		const b = await box(page, "div.bloom");
		const cur = `${b.x}|${b.width}|${b.height}`;
		if (cur === last) {
			const again = await box(page, "div.bloom");
			if (`${again.x}|${again.width}|${again.height}` === cur) return;
		}
		last = cur;
		await page.waitForTimeout(120);
	}
};

test.describe("seven-agent density", () => {
	test("the ring row stays inside the island", async ({ page }) => {
		await emitReadings(page, sevenFixture());
		await settlePill(page);
		const pill = await box(page, "div.bloom");
		const qs = await box(page, "[data-testid=quota-space]");
		expect(qs.x).toBeGreaterThanOrEqual(pill.x - 0.5);
		expect(qs.x + qs.width).toBeLessThanOrEqual(pill.x + pill.width + 0.5);
		for (const id of SEVEN_IDS) {
			const cell = await box(page, `[data-testid=quota-cell-${id}]`);
			expect(cell.x).toBeGreaterThanOrEqual(pill.x - 0.5);
			expect(cell.x + cell.width).toBeLessThanOrEqual(pill.x + pill.width + 0.5);
		}
	});

	test("each cell's contents stay inside its own box", async ({ page }) => {
		await emitReadings(page, sevenFixture());
		await settlePill(page);
		for (const id of SEVEN_IDS) {
			const cell = page.getByTestId(`quota-cell-${id}`);
			const cr = (await cell.boundingBox())!;
			// Text lines ellipsize at the cell edge; the ring block sits
			// inside the tile like in the standalone widget.
			for (const part of [".qs-name", ".qs-basis", ".qs-cell-top"]) {
				const pr = (await cell.locator(part).boundingBox())!;
				expect(pr.x).toBeGreaterThanOrEqual(cr.x - 0.5);
				expect(pr.x + pr.width).toBeLessThanOrEqual(cr.x + cr.width + 0.5);
			}
		}
	});

	// zoom leaves scrollWidth in unzoomed CSS px while rects scale: the
	// width contract must hold at every Bloom scale, not just 1.
	for (const bloomScale of [0.8, 1.25]) {
		test(`the ring row stays inside the island at scale ${bloomScale}`, async ({ page }) => {
			await page.evaluate((v) => localStorage.setItem("bloom-scale", v), String(bloomScale));
			await page.reload();
			await page.waitForTimeout(900);
			await emitReadings(page, sevenFixture());
			await openQuotaSpace(page);
			await settlePill(page);
			const pill = await box(page, "div.bloom");
			const qs = await box(page, "[data-testid=quota-space]");
			expect(qs.x).toBeGreaterThanOrEqual(pill.x - 0.5);
			expect(qs.x + qs.width).toBeLessThanOrEqual(pill.x + pill.width + 0.5);
			for (const id of SEVEN_IDS) {
				const cell = await box(page, `[data-testid=quota-cell-${id}]`);
				expect(cell.x).toBeGreaterThanOrEqual(pill.x - 0.5);
				expect(cell.x + cell.width).toBeLessThanOrEqual(pill.x + pill.width + 0.5);
			}
		});
	}

	test("the selected cell reads differently from a hovered one", async ({ page }) => {
		await emitReadings(page, sevenFixture());
		const sel = page.getByTestId("quota-cell-devin");
		await sel.click();
		const hov = page.getByTestId("quota-cell-codex");
		await hov.hover();
		const paint = (el: typeof sel) =>
			el.evaluate((e) => {
				const s = getComputedStyle(e);
				return `${s.backgroundColor}|${s.boxShadow}`;
			});
		expect(await paint(sel)).not.toBe(await paint(hov));
	});

	test("the detail card keeps clearance above the island's bottom edge", async ({ page }) => {
		await emitReadings(page, sevenFixture());
		await page.waitForTimeout(1200);
		const pill = await box(page, "div.bloom");
		const detail = await box(page, ".qs-detail");
		expect(pill.y + pill.height - (detail.y + detail.height)).toBeGreaterThanOrEqual(4);
	});

	test("long detail content scrolls and keeps the detail card reachable", async ({ page }) => {
		await emitReadings(page, longFixture());
		await page.getByTestId("quota-cell-claude").click();
		await page.waitForTimeout(400);
		const dims = await page
			.locator(".quota-viewport")
			.evaluate((e) => ({ scroll: e.scrollHeight, client: e.clientHeight }));
		expect(dims.scroll).toBeGreaterThan(dims.client);
		await page.locator(".qs-detail").scrollIntoViewIfNeeded();
		await expect(page.locator(".qs-detail")).toBeVisible();
	});

	test("leaving the quota space releases its height", async ({ page }) => {
		await emitReadings(page, sevenFixture());
		await page.waitForTimeout(1200);
		// Cycle to the next panel; the quota space must unmount and report a
		// zero height so the window can shrink back.
		const next = page.locator(".time-flip-container");
		for (let i = 0; i < 4 && (await quotaSpace(page).count()) > 0; i++) {
			await next.click();
			await page.waitForTimeout(400);
		}
		await expect(quotaSpace(page)).toHaveCount(0);
		const heights = (await invokes(page))
			.filter((i) => i.cmd === "set_quota_panel_height")
			.map((i) => i.args?.height);
		expect(heights).toContain(0);
	});
});

/* The island is bounded by the display, not by content alone: max-content
   measures every string unwrapped, so reader errors, hints, detail labels
   and group names can otherwise push cells offscreen (width) or drop the
   footer past the window's bottom edge (height). */
const LONG_STRING = `reader failed fetching https://quota.invalid/api/${"segment-".repeat(24)}end`;

// Long unbroken strings in every lane that feeds `.qs` max-content.
const wideFixture = (): QuotaAgent[] => {
	const agents = sevenFixture();
	agents[0] = { ...agents[0], err: LONG_STRING, help: LONG_STRING };
	agents[5] = {
		...agents[5],
		details: [{ label: LONG_STRING, value: "56% used", resetAt: later(6.3 * HOUR) }]
	};
	agents[6] = {
		...agents[6],
		groups: [{ name: LONG_STRING, windows: [{ label: "5h", used: 0 }] }]
	};
	return agents;
};

// Load at a chosen scale and viewport, then open quotas on wideFixture.
const openWide = async (page: Page, bloomScale: number, w: number, h: number) => {
	await page.setViewportSize({ width: w, height: h });
	await page.evaluate((v) => localStorage.setItem("bloom-scale", v), String(bloomScale));
	await page.reload();
	await page.waitForTimeout(900);
	await emitReadings(page, wideFixture());
	await openQuotaSpace(page);
	await settlePill(page);
};

test.describe("display bounds", () => {
	// A narrow display plus unbounded strings: the island must stop at the
	// available width while capped cells ellipsize rather than clip.
	for (const bloomScale of [0.8, 1, 1.25]) {
		test(`long strings keep the island inside the display at scale ${bloomScale}`, async ({
			page
		}) => {
			await openWide(page, bloomScale, 560, 720);
			const innerWidth = await page.evaluate(() => window.innerWidth);
			const pill = await box(page, "div.bloom");
			const qs = await box(page, "[data-testid=quota-space]");
			expect(pill.width).toBeLessThanOrEqual(innerWidth + 0.5);
			expect(qs.x).toBeGreaterThanOrEqual(pill.x - 0.5);
			expect(qs.x + qs.width).toBeLessThanOrEqual(pill.x + pill.width + 0.5);
			for (const id of SEVEN_IDS) {
				const cell = await box(page, `[data-testid=quota-cell-${id}]`);
				expect(cell.x).toBeGreaterThanOrEqual(pill.x - 0.5);
				expect(cell.x + cell.width).toBeLessThanOrEqual(pill.x + pill.width + 0.5);
			}
			// Capped cells ellipsize; the name lane keeps its own type size.
			const name = page.getByTestId("quota-cell-devin").locator(".qs-name");
			expect(await name.evaluate((e) => getComputedStyle(e).fontSize)).toBe("10.5px");
		});
	}

	// Sixty detail rows outgrow any monitor: the scrollport must stop at
	// the window's interior (below the 36px status row) so the footer is
	// reachable inside the window, not merely DOM-visible past its edge.
	for (const bloomScale of [0.8, 1, 1.25]) {
		test(`the scrolled detail card stays inside the window at scale ${bloomScale}`, async ({
			page
		}) => {
			await page.setViewportSize({ width: 640, height: 480 });
			await page.evaluate((v) => localStorage.setItem("bloom-scale", v), String(bloomScale));
			await page.reload();
			await page.waitForTimeout(900);
			await emitReadings(page, longFixture());
			await openQuotaSpace(page);
			await page.getByTestId("quota-cell-claude").click();
			await page.waitForTimeout(400);
			const innerHeight = await page.evaluate(() => window.innerHeight);
			const vp = page.locator(".quota-viewport");
			const dims = await vp.evaluate((e) => ({
				scroll: e.scrollHeight,
				client: e.clientHeight,
				bottom: e.getBoundingClientRect().bottom
			}));
			expect(dims.scroll).toBeGreaterThan(dims.client);
			// The viewport's painted bottom edge may not pass the window's
			// bottom edge — the physical clip a mocked harness cannot show.
			expect(dims.bottom).toBeLessThanOrEqual(innerHeight + 0.5);
			await vp.evaluate((e) => {
				e.scrollTop = e.scrollHeight;
			});
			const detail = await box(page, ".qs-detail");
			expect(detail.y + detail.height).toBeLessThanOrEqual(innerHeight + 0.5);
		});
	}
});

/* The collapsed notch's hover badge: one compact ring tile for the pinned
   agent (or the busiest reading when nothing is pinned), beside the clock. */
test.describe("hover quota badge", () => {
	// Close the space the beforeEach opened, then hover the collapsed pill.
	const hoverCollapsed = async (page: Page) => {
		await page.getByTestId("quota-close").click();
		await expect(quotaSpace(page)).toHaveCount(0, { timeout: 4000 });
		await page.locator("div.bloom").hover();
	};

	test("shows the busiest agent's dual-ring mark beside the clock", async ({ page }) => {
		await hoverCollapsed(page);
		const badge = page.getByTestId("notch-hover-quota");
		await expect(badge).toBeVisible();
		// The clock stays centered and visible beside it.
		await expect(page.locator(".time-center")).toBeVisible();

		// Cursor is the fixture's highest-used agent at 71%.
		await expect(badge).toHaveAttribute("aria-label", "Cursor. Show next agent");
		// Always both concentric tracks, plus the brand mark — and no text.
		await expect(badge.locator("circle.qs-ring-track")).toHaveCount(2);
		await expect(badge.locator("svg[aria-label='Cursor']")).toHaveCount(1);
		await expect(badge).not.toContainText("%");
	});

	test("shows every full model name in the hover card", async ({ page }) => {
		await emitReadings(page, hoverModelFixture());
		await hoverCollapsed(page);
		const badge = page.getByTestId("notch-hover-quota");
		await expect(badge).toHaveAttribute("aria-label", "Antigravity. Show next agent");
		await badge.hover();

		const card = page.getByTestId("quota-hover-card");
		await expect(card).toBeVisible();
		await expect(card.locator(".qh-place")).toHaveText("7 of 7");

		// Every reader-supplied model name is rendered in full, not clipped
		// with an ellipsis in a narrow first grid track.
		const labels = card.locator(".qh-label");
		await expect(labels).toHaveText(HOVER_MODELS);
		for (const label of await labels.all()) {
			const metrics = await label.evaluate((el) => ({
				clientWidth: el.clientWidth,
				scrollWidth: el.scrollWidth,
				whiteSpace: getComputedStyle(el).whiteSpace,
				textOverflow: getComputedStyle(el).textOverflow
			}));
			expect(metrics.scrollWidth).toBeLessThanOrEqual(metrics.clientWidth + 1);
			expect(metrics.whiteSpace).not.toBe("nowrap");
			expect(metrics.textOverflow).not.toBe("ellipsis");
		}

		// The label owns the first line; bar, percent and reset sit below it
		// without ever covering the name.
		const rows = card.locator(".qh-row");
		await expect(rows).toHaveCount(HOVER_MODELS.length);
		for (let i = 0; i < HOVER_MODELS.length; i++) {
			const row = rows.nth(i);
			const label = (await row.locator(".qh-label").boundingBox())!;
			const usage = (await row.locator(".qh-usage").boundingBox())!;
			const reset = (await row.locator(".qh-reset").boundingBox())!;
			expect(label.y + label.height).toBeLessThanOrEqual(usage.y + 0.5);
			expect(label.y + label.height).toBeLessThanOrEqual(reset.y + 0.5);
			expect(usage.x + usage.width).toBeLessThanOrEqual(reset.x + 0.5);
		}

		// The wider content grows downward, never sideways out of the display.
		const innerWidth = await page.evaluate(() => window.innerWidth);
		const box = (await card.boundingBox())!;
		expect(box.x).toBeGreaterThanOrEqual(0);
		expect(box.x + box.width).toBeLessThanOrEqual(innerWidth + 0.5);
	});

	test("keeps full model names readable on a narrow display and larger scale", async ({ page }) => {
		await page.setViewportSize({ width: 560, height: 720 });
		await page.evaluate(() => localStorage.setItem("bloom-scale", "1.25"));
		await page.reload();
		await page.waitForTimeout(900);

		// A longer future name must wrap inside the label line, not ellipsize.
		const longLabel = "Claude Sonnet 4.5 (Thinking) with Extended Context / 5h";
		const agents = hoverModelFixture();
		agents[6] = {
			...agents[6],
			details: [longLabel, ...HOVER_MODELS.slice(1)].map((label, i) => ({
				label,
				value: `${[94, 1, 1, 100, 1][i]}% used`,
				resetAt: later((i + 1) * 6 * HOUR)
			}))
		};
		await emitReadings(page, agents);
		await openQuotaSpace(page);
		await hoverCollapsed(page);

		const badge = page.getByTestId("notch-hover-quota");
		await expect(badge).toHaveAttribute("aria-label", "Antigravity. Show next agent");
		await badge.hover();
		const card = page.getByTestId("quota-hover-card");
		await expect(card).toBeVisible();
		await expect(card.locator(".qh-label").first()).toHaveText(longLabel);

		for (const label of await card.locator(".qh-label").all()) {
			const metrics = await label.evaluate((el) => ({
				clientWidth: el.clientWidth,
				scrollWidth: el.scrollWidth
			}));
			expect(metrics.scrollWidth).toBeLessThanOrEqual(metrics.clientWidth + 1);
		}

		const viewport = await page.evaluate(() => ({
			width: window.innerWidth,
			height: window.innerHeight
		}));
		const box = (await card.boundingBox())!;
		expect(box.x).toBeGreaterThanOrEqual(0);
		expect(box.y).toBeGreaterThanOrEqual(0);
		expect(box.x + box.width).toBeLessThanOrEqual(viewport.width + 0.5);
		expect(box.y + box.height).toBeLessThanOrEqual(viewport.height + 0.5);
	});

	test("follows the agent selected in the quota space", async ({ page }) => {
		await page.getByTestId("quota-cell-devin").click();
		await hoverCollapsed(page);
		const badge = page.getByTestId("notch-hover-quota");
		await expect(badge).toBeVisible();
		await expect(badge).toHaveAttribute("aria-label", "Devin. Show next agent");
		await expect(badge.locator("svg[aria-label='Devin']")).toHaveCount(1);
	});

	test("cycles to the next agent without opening or taking focus", async ({ page }) => {
		await hoverCollapsed(page);
		const badge = page.getByTestId("notch-hover-quota");
		await expect(badge).toBeVisible();
		const active = () =>
			page.evaluate(
				() => `${document.activeElement?.tagName}.${document.activeElement?.className}`
			);
		const before = await active();
		await badge.click();
		await expect(quotaSpace(page)).toHaveCount(0);
		await expect(badge.locator("svg[aria-label='Devin']")).toHaveCount(1);
		expect(await active()).toBe(before);
	});

	// Resting cluster order: waveform, quota, the clock, then album art.
	test("shows waveform, quota, the clock and album art in resting order without overlap", async ({
		page
	}) => {
		await page.evaluate(() =>
			(window as any).__emit("media-update", {
				title: "Starlight",
				artist: "Muse",
				is_playing: true,
				has_media: true,
				artwork: [],
				position_ms: 1000,
				duration_ms: 180000
			})
		);
		await emitReadings(page, fixture());
		await hoverCollapsed(page);
		await settlePill(page);

		const visualizer = page.locator(".visualizer-horizontal");
		const badge = page.getByTestId("notch-hover-quota");
		const time = page.locator(".time-center");
		const art = page.locator(".album-art-activity");
		await expect(visualizer).toBeVisible();
		await expect(badge).toBeVisible();
		await expect(time).toBeVisible();
		await expect(art).toBeVisible();
		await expect(page.locator(".passive-feature")).toHaveCount(0);

		const pillBox = await box(page, "div.bloom");
		const visualizerBox = (await visualizer.boundingBox())!;
		const badgeBox = (await badge.boundingBox())!;
		const timeBox = (await time.boundingBox())!;
		const artBox = (await art.boundingBox())!;

		expect(visualizerBox.x).toBeGreaterThanOrEqual(pillBox.x - 0.5);
		expect(visualizerBox.x + visualizerBox.width).toBeLessThanOrEqual(badgeBox.x + 0.5);
		expect(badgeBox.x + badgeBox.width).toBeLessThanOrEqual(timeBox.x + 0.5);
		expect(timeBox.x + timeBox.width).toBeLessThanOrEqual(artBox.x + 0.5);
		expect(artBox.x + artBox.width).toBeLessThanOrEqual(pillBox.x + pillBox.width + 0.5);
	});
});
