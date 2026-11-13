import { describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import type { QuotaAgent } from "./protocol";
import { QuotaChip, QuotaHoverCard, QuotaPanel, QuotaRingMark } from "./QuotaPanel";

// Server markup keeps these tests DOM-free: effects (and therefore the
// ResizeObserver / invoke calls) never run, so nothing needs Tauri.

const noop = () => {};
const props = {
	selected: undefined,
	onSelect: noop,
	pinned: false,
	onPin: noop,
	onCompact: noop,
	onClose: noop
};

describe("QuotaPanel", () => {
	test("the MyFinder menu keeps waiting and failed agents selectable without inventing usage", () => {
		const readings: QuotaAgent[] = [
			{ id: "claude", state: "waiting" },
			{ id: "kiro", state: "failed", err: "Usage unavailable" },
			{ id: "codex", state: "stale", used: 76, err: "Latest check failed" }
		];
		const html = renderToStaticMarkup(<QuotaPanel readings={readings} {...props} selected="codex" menu />);
		expect((html.match(/class="qs-menu-row"/g) ?? []).length).toBe(3);
		expect(html).toContain("Checking…");
		expect(html).toContain("Unavailable");
		expect(html).toContain("76% used, last good reading");
		expect(html).toContain("Latest check failed");
		expect(html).not.toContain(">0%<");
		expect(html).not.toContain('class="qs-rings"');
	});

	test("the MyFinder menu renders QuotaDisplay, segmented meters, groups, and nav without HTML title tooltips", () => {
		const readings: QuotaAgent[] = [
			{
				id: "claude",
				state: "ok",
				plan: "Max",
				used: 42,
				windows: [
					{ label: "5h", used: 42 },
					{ label: "week", used: 84 }
				],
				reset5h: new Date(Date.now() + 2 * 3600_000).toISOString(),
				groups: [{ name: "Sonnet", windows: [{ label: "5h", used: 42 }] }]
			},
			{
				id: "codex",
				state: "ok",
				used: 95,
				windows: [{ label: "5h", used: 95 }]
			}
		];
		const html = renderToStaticMarkup(
			<QuotaPanel
				readings={readings}
				{...props}
				selected="claude"
				menu
				onModeChange={noop}
				musicAvailable={false}
			/>
		);
		expect(html).toContain("Agent quota");
		expect(html).toContain("qs-display");
		expect(html).toContain("% used");
		expect(html).toContain('class="qs-meter"');
		expect(html).toContain('data-band="green"');
		expect(html).toContain('data-band="amber"');
		expect(html).toContain('data-band="red"');
		expect(html).toMatch(/--lit:41\.6/);
		expect(html).toContain('data-testid="quota-groups-toggle"');
		expect(html).toContain('data-testid="quota-nav-quotas"');
		expect(html).toContain('data-testid="quota-nav-music"');
		expect(html).toContain('data-testid="quota-nav-controls"');
		// Music tab is disabled when musicAvailable={false}
		const musicBtn = html.match(/<button\b[^>]*data-testid="quota-nav-music"[^>]*>/)?.[0] ?? "";
		expect(musicBtn).toContain('disabled=""');
		// Every button in menu mode must be focus-free and omit native title tooltips
		const buttons = html.match(/<button\b[^>]*>/g) ?? [];
		expect(buttons.length).toBeGreaterThanOrEqual(8);
		for (const tag of buttons) {
			expect(tag).toContain('tabindex="-1"');
			expect(tag).toContain('type="button"');
			expect(tag).not.toContain(" title=");
		}
	});

	test("the MyFinder menu enables Music tab when musicAvailable is true and displays failed refresh action", () => {
		const readings: QuotaAgent[] = [
			{
				id: "kiro",
				state: "failed",
				err: "Session expired",
				help: "Retry check"
			}
		];
		const html = renderToStaticMarkup(
			<QuotaPanel
				readings={readings}
				{...props}
				selected="kiro"
				menu
				onModeChange={noop}
				musicAvailable
			/>
		);
		const musicBtn = html.match(/<button\b[^>]*data-testid="quota-nav-music"[^>]*>/)?.[0] ?? "";
		expect(musicBtn).not.toContain("disabled");
		expect(html).toContain("Session expired");
		expect(html).toContain("Retry check");
		expect(html).toContain('data-testid="quota-hint"');
	});

	test("the MyFinder menu displays ShinyText on busy or waiting agents, and shows stale/failed notes with actions", () => {
		const readings: QuotaAgent[] = [
			{ id: "cursor", state: "waiting" },
			{ id: "claude", state: "ok", used: 30, busy: true },
			{ id: "kiro", state: "failed", err: "Unauthorized", help: "Re-authenticate" },
			{ id: "codex", state: "stale", used: 75, lastGoodAt: new Date(Date.now() - 120_000).toISOString(), err: "Rate limited" }
		];

		// Selected = waiting agent: ShinyText on status "Checking…"
		const htmlWaiting = renderToStaticMarkup(
			<QuotaPanel readings={readings} {...props} selected="cursor" menu onModeChange={noop} />
		);
		expect(htmlWaiting).toContain("qs-display");
		expect(htmlWaiting).toContain("shiny-text");
		expect(htmlWaiting).toContain("Checking…");

		// Selected = busy agent: ShinyText on agent name
		const htmlBusy = renderToStaticMarkup(
			<QuotaPanel readings={readings} {...props} selected="claude" menu onModeChange={noop} />
		);
		expect(htmlBusy).toContain("shiny-text");
		expect(htmlBusy).toContain("Claude");

		// Selected = failed agent: note and refresh hint action button
		const htmlFailed = renderToStaticMarkup(
			<QuotaPanel readings={readings} {...props} selected="kiro" menu onModeChange={noop} />
		);
		expect(htmlFailed).toContain("Unauthorized");
		expect(htmlFailed).toContain("Re-authenticate");
		expect(htmlFailed).toContain('data-testid="quota-hint"');

		// Selected = stale agent: last good note and error
		const htmlStale = renderToStaticMarkup(
			<QuotaPanel readings={readings} {...props} selected="codex" menu onModeChange={noop} />
		);
		expect(htmlStale).toContain("Last good");
		expect(htmlStale).toContain("Rate limited");
		expect(htmlStale).toContain("75% used, last good reading");
	});

	test("the MyFinder compact menu renders all agents as QuotaChip capsules, a 1-line detail strip, and focus-free controls", () => {
		const readings: QuotaAgent[] = [
			{
				id: "claude",
				state: "ok",
				plan: "Max",
				used: 100,
				windows: [{ label: "5h", used: 100 }],
				reset5h: new Date(Date.now() + 14 * 60_000).toISOString()
			},
			{
				id: "commandcode",
				state: "ok",
				plan: "Go",
				used: 58,
				windows: [
					{ label: "5h", used: 1 },
					{ label: "week", used: 58 }
				],
				reset5h: new Date(Date.now() + 4.8 * 3600_000).toISOString()
			},
			{ id: "cursor", state: "waiting" },
			{ id: "kiro", state: "failed", err: "Session expired", help: "Retry" },
			{
				id: "codex",
				state: "stale",
				used: 63,
				lastGoodAt: new Date(Date.now() - 120_000).toISOString(),
				err: "Rate limited"
			}
		];

		// Non-compact MyFinder menu renders Compact button in header
		const htmlExpanded = renderToStaticMarkup(
			<QuotaPanel readings={readings} {...props} selected="commandcode" menu onModeChange={noop} />
		);
		expect(htmlExpanded).toContain('data-testid="quota-compact"');
		expect(htmlExpanded).not.toContain('data-testid="quota-expand"');

		// Compact MyFinder menu renders QuotaChip grid, compact detail, Expand button, and no row list / SpotlightCard
		const htmlCompact = renderToStaticMarkup(
			<QuotaPanel
				readings={readings}
				{...props}
				selected="commandcode"
				menu
				compact
				onExpand={noop}
				onModeChange={noop}
			/>
		);
		expect(htmlCompact).toContain("qs-menu-compact");
		expect(htmlCompact).toContain("qs-menu-chips");
		expect((htmlCompact.match(/class="quota-chip"/g) ?? []).length).toBe(5);
		expect(htmlCompact).toContain('data-testid="quota-expand"');
		expect(htmlCompact).toContain('data-testid="quota-compact-detail"');
		expect(htmlCompact).toContain("Command Code");
		expect(htmlCompact).toContain("Go");
		expect(htmlCompact).toContain("5h 1%");
		expect(htmlCompact).not.toContain('class="qs-menu-rows"');
		expect(htmlCompact).not.toContain('class="qs-display');
		expect(htmlCompact).not.toContain('class="qs-rings"');

		const buttons = htmlCompact.match(/<button\b[^>]*>/g) ?? [];
		expect(buttons.length).toBeGreaterThanOrEqual(10);
		for (const tag of buttons) {
			expect(tag).toContain('tabindex="-1"');
			expect(tag).toContain('type="button"');
			expect(tag).not.toContain(" title=");
		}

		// Failed selected agent in compact mode shows error and inline retry hint
		const htmlFailed = renderToStaticMarkup(
			<QuotaPanel
				readings={readings}
				{...props}
				selected="kiro"
				menu
				compact
				onExpand={noop}
				onModeChange={noop}
			/>
		);
		expect(htmlFailed).toContain("Session expired");
		expect(htmlFailed).toContain("Retry");
		expect(htmlFailed).toContain('data-testid="quota-hint"');

		// Stale selected agent shows Last good timestamp
		const htmlStale = renderToStaticMarkup(
			<QuotaPanel
				readings={readings}
				{...props}
				selected="codex"
				menu
				compact
				onExpand={noop}
				onModeChange={noop}
			/>
		);
		expect(htmlStale).toContain("Last good");
	});
	test("a zero-agent snapshot renders the empty state", () => {
		const html = renderToStaticMarkup(<QuotaPanel readings={[]} {...props} />);
		expect(html).toContain("No agents enabled");
		expect(html).toContain(">Used<");
		expect(html).not.toContain("quota-failed");
		expect(html).not.toContain("quota-cell-");
	});

	test("all agents failed keeps the ring row, error and hint, no global count", () => {
		const readings: QuotaAgent[] = [
			{
				id: "claude",
				state: "failed",
				err: "oauth expired",
				help: "Sign in again"
			},
			{ id: "devin", state: "failed", detail: "http 400", help: "Run devin login" }
		];
		const html = renderToStaticMarkup(<QuotaPanel readings={readings} {...props} />);
		expect(html).toContain('data-testid="quota-cell-claude"');
		expect(html).toContain('data-testid="quota-cell-devin"');
		expect(html).not.toContain("quota-failed");
		// The default selection is the first failed agent.
		expect(html).toContain("oauth expired");
		expect(html).toContain("Sign in again");
		expect(html).toContain('data-testid="quota-hint"');
		expect(html).toContain("Unavailable");
	});

	test("a past 5h reset reads now, never a negative countdown", () => {
		const readings: QuotaAgent[] = [
			{
				id: "claude",
				state: "ok",
				used: 40,
				windows: [{ label: "5h", used: 40 }],
				reset5h: new Date(Date.now() - 60_000).toISOString()
			}
		];
		const html = renderToStaticMarkup(<QuotaPanel readings={readings} {...props} />);
		expect(html).toContain('data-testid="quota-reset"');
		expect(html).toContain("resets now");
		expect(html).not.toContain("-1m");
	});

	test("refresh is disabled while any agent is busy", () => {
		const readings: QuotaAgent[] = [
			{ id: "kiro", state: "ok", used: 10, busy: true },
			{ id: "devin", state: "ok", used: 20 }
		];
		const html = renderToStaticMarkup(<QuotaPanel readings={readings} {...props} />);
		expect(html).toContain('data-testid="quota-refresh"');
		expect(html).toContain('disabled=""');
		expect(html).toContain('aria-label="Checking…"');
	});

	test("the header offers the compact toggle", () => {
		const readings: QuotaAgent[] = [{ id: "claude", state: "ok", used: 40 }];
		const html = renderToStaticMarkup(<QuotaPanel readings={readings} {...props} />);
		expect(html).toContain('data-testid="quota-compact"');
		expect(html).toContain('aria-label="Compact quota space"');
	});

	test("every control is a focus-free button", () => {
		const readings: QuotaAgent[] = [
			{
				id: "claude",
				state: "ok",
				used: 40,
				windows: [{ label: "5h", used: 40 }],
				reset5h: new Date(Date.now() + 3600_000).toISOString()
			}
		];
		const html = renderToStaticMarkup(<QuotaPanel readings={readings} {...props} />);
		const buttons = html.match(/<button\b[^>]*>/g) ?? [];
		expect(buttons.length).toBeGreaterThan(2);
		for (const tag of buttons) {
			expect(tag).toContain('tabindex="-1"');
			expect(tag).toContain('type="button"');
		}
		expect(html).not.toContain("onkeydown");
		expect(html).toContain("Just checked");
		expect(html).not.toContain("1 failed");
	});
});

describe("QuotaHoverCard", () => {
	test("one row per window, no reader count and no source line", () => {
		const html = renderToStaticMarkup(
			<QuotaHoverCard
				a={{
					id: "codex",
					state: "ok",
					used: 55,
					details: [
						{
							label: "5h",
							value: "55% used",
							resetAt: new Date(Date.now() + 4 * 3600_000).toISOString()
						},
						{
							label: "Week",
							value: "16% used",
							resetAt: new Date(Date.now() + 5 * 86400_000).toISOString()
						}
					]
				}}
				place="2 of 5"
			/>
		);
		expect(html).toContain("Codex");
		expect(html).toContain("2 of 5");
		expect(html).toContain(">5h<");
		expect(html).toContain(">55%<");
		expect(html).toContain(">Week<");
		expect(html).toContain(">16%<");
		expect(html).toContain("resets in");
		expect(html).toContain("qh-reset-at");
		expect(html).not.toContain("1 failed");
		expect(html).not.toContain("Source observed");
		expect(html).not.toContain("% used");
	});
});

describe("QuotaRingMark", () => {
	const dual: QuotaAgent = {
		id: "codex",
		state: "ok",
		used: 30,
		windows: [
			{ label: "5h", used: 30 },
			{ label: "week", used: 60 }
		]
	};
	const count = (html: string, cls: string) => (html.match(new RegExp(`class="${cls}[ "]`, "g")) ?? []).length;

	test("the notch keeps both concentric rings", () => {
		const html = renderToStaticMarkup(<QuotaRingMark a={dual} />);
		expect(count(html, "qs-ring-track")).toBe(2);
		expect(count(html, "qs-ring-fill")).toBe(2);
	});
});

describe("QuotaChip", () => {
	const chip = (a: QuotaAgent) =>
		renderToStaticMarkup(
			<QuotaChip a={a}>
				<i />
			</QuotaChip>
		);

	test("the meter fills to the headline value in its band colour", () => {
		const html = chip({ id: "qoder", state: "ok", used: 100 });
		expect(html).toContain("--quota-fill:100%");
		expect(html).toContain("--quota-color:#FF453A");
		expect(html).toContain(">100<");
	});

	test("the number stays true past the cap while the fill stops at full", () => {
		const html = chip({ id: "cursor", state: "ok", used: 104.4 });
		expect(html).toContain("--quota-fill:100%");
		expect(html).toContain(">104<");
	});

	test("a waiting or failed agent shows a dash and no meter, never zero", () => {
		for (const state of ["waiting", "failed"] as const) {
			const html = chip({ id: "claude", state });
			expect(html).toContain("—");
			expect(html).not.toContain("--quota-fill");
			expect(html).not.toContain(">0<");
		}
	});

	test("a stale reading keeps its last good value and says so", () => {
		const html = chip({ id: "codex", state: "stale", used: 76 });
		expect(html).toContain('data-state="stale"');
		expect(html).toContain("--quota-fill:76%");
	});
});
