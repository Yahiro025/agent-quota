import { describe, expect, test } from "bun:test";
import { renderToStaticMarkup } from "react-dom/server";
import type { QuotaAgent } from "./protocol";
import { QuotaPanel } from "./QuotaPanel";

// Server markup keeps these tests DOM-free: effects (and therefore the
// ResizeObserver / invoke calls) never run, so nothing needs Tauri.

const noop = () => {};
const props = {
	selected: undefined,
	onSelect: noop,
	pinned: false,
	onPin: noop,
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

	test("all agents failed keeps the ring row, count, error and hint", () => {
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
		expect(html).toContain("2 failed");
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
		expect(html).toContain("5h · now");
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
		expect(html).toContain("Checking…");
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
	});
});
