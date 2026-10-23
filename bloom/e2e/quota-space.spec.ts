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

const emitReadings = (page: Page, readings: QuotaAgent[]) =>
	page.evaluate(
		([ev, pl]) => (window as any).__emit(ev, pl),
		["quota-snapshot", readings]
	);

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

const box = async (page: Page, selector: string) =>
	(await page.locator(selector).boundingBox())!;

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
		test(`the ring row stays inside the island at scale ${bloomScale}`, async ({
			page
		}) => {
			await page.evaluate(
				(v) => localStorage.setItem("bloom-scale", v),
				String(bloomScale)
			);
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
				expect(cell.x + cell.width).toBeLessThanOrEqual(
					pill.x + pill.width + 0.5
				);
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

	test("the footer keeps clearance above the island's bottom edge", async ({ page }) => {
		await emitReadings(page, sevenFixture());
		await page.waitForTimeout(1200);
		const pill = await box(page, "div.bloom");
		const footer = await box(page, ".qs-footer");
		expect(pill.y + pill.height - (footer.y + footer.height)).toBeGreaterThanOrEqual(4);
	});

	test("long detail content scrolls and keeps the footer reachable", async ({ page }) => {
		await emitReadings(page, longFixture());
		await page.getByTestId("quota-cell-claude").click();
		await page.waitForTimeout(400);
		const dims = await page
			.locator(".quota-viewport")
			.evaluate((e) => ({ scroll: e.scrollHeight, client: e.clientHeight }));
		expect(dims.scroll).toBeGreaterThan(dims.client);
		await page.getByTestId("quota-refresh").scrollIntoViewIfNeeded();
		await expect(page.getByTestId("quota-refresh")).toBeVisible();
	});

	test("leaving the quota space releases its height", async ({ page }) => {
		await emitReadings(page, sevenFixture());
		await page.waitForTimeout(1200);
		// Cycle to the next panel; the quota space must unmount and report a
		// zero height so the window can shrink back.
		const next = page.locator("button.panel-arrow.next");
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
		test(`the scrolled footer stays inside the window at scale ${bloomScale}`, async ({
			page
		}) => {
			await page.setViewportSize({ width: 640, height: 480 });
			await page.evaluate(
				(v) => localStorage.setItem("bloom-scale", v),
				String(bloomScale)
			);
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
			const footer = await box(page, ".qs-footer");
			expect(footer.y + footer.height).toBeLessThanOrEqual(innerHeight + 0.5);
		});
	}
});
