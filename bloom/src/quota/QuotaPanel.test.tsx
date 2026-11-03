import { describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import type { QuotaAgent } from "./protocol";
import { QuotaHoverCard, QuotaPanel } from "./QuotaPanel";

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
