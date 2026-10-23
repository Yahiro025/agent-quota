// View model for the Agent Quotas panel: folds one helper snapshot into
// rows the panel renders as-is. Kept React-free so the bun tests exercise
// the same code the component draws. The helper never sends formatted
// time — each row's countdown and clock are both formatted here from that
// row's one reset5h instant, and each detail's both from its resetAt, so
// the two displays can never disagree.

import { band, urgent, type QuotaAgent, type QuotaDetail, type QuotaWindow } from "./protocol";

// A reset instant rendered both ways; which one a row shows is a display
// toggle, not two different times.
export interface QuotaResetView {
	countdown: string;
	clock: string;
}

export interface QuotaDetailView {
	label: string;
	value: string;
	resetText?: string;
	reset?: QuotaResetView;
}

export interface QuotaRow {
	id: string;
	state: QuotaAgent["state"];
	busy: boolean;
	used?: number;
	band?: "green" | "amber" | "red";
	plan?: string;
	detail?: string;
	err?: string;
	resetLabel?: string;
	reset?: QuotaResetView;
	help?: string;
	windows: QuotaWindow[];
	details: QuotaDetailView[];
}

// `panel` is the currently open notch panel. `showInRest` asks for quota on
// the resting island: only an urgent reading can ask, and not while the
// quotas panel is already the open one.
export function quotaView(
	readings: QuotaAgent[],
	panel: string,
	now: Date = new Date()
): { showInRest: boolean; panel: QuotaRow[] } {
	return {
		showInRest: panel !== "quotas" && readings.some(urgent),
		panel: readings.map((reading) => toRow(reading, now))
	};
}

// The urgent subset of readings, as the same rows the panel would draw, so
// the alert strip's name, percent and reset never disagree with the panel.
export function alertRows(readings: QuotaAgent[], now: Date = new Date()): QuotaRow[] {
	return readings.filter(urgent).map((reading) => toRow(reading, now));
}

const toRow = (reading: QuotaAgent, now: Date): QuotaRow => ({
	id: reading.id,
	state: reading.state,
	busy: reading.busy === true,
	used: reading.used,
	band: typeof reading.used === "number" ? band(reading.used) : undefined,
	plan: reading.plan,
	detail: reading.detail,
	err: reading.err,
	resetLabel: reading.resetLabel,
	reset: resetView(reading.reset5h, now),
	help: reading.help,
	windows: reading.windows ?? [],
	details: (reading.details ?? []).map((d) => toDetail(d, now))
});

const toDetail = (detail: QuotaDetail, now: Date): QuotaDetailView => ({
	label: detail.label,
	value: detail.value,
	resetText: detail.resetText,
	reset: resetView(detail.resetAt, now)
});

const resetView = (iso: string | undefined, now: Date): QuotaResetView | undefined => {
	if (!iso) return undefined;
	const instant = new Date(iso);
	if (Number.isNaN(instant.getTime())) return undefined;
	return { countdown: countdownText(instant, now), clock: clockText(instant) };
};

// "1h 30m" style countdown; past instants read "now".
const countdownText = (instant: Date, now: Date): string => {
	const minutes = Math.round((instant.getTime() - now.getTime()) / 60000);
	if (minutes <= 0) return "now";
	const days = Math.floor(minutes / 1440);
	const hours = Math.floor((minutes % 1440) / 60);
	const mins = minutes % 60;
	if (days > 0) return hours > 0 ? `${days}d ${hours}h` : `${days}d`;
	if (hours > 0) return mins > 0 ? `${hours}h ${mins}m` : `${hours}h`;
	return `${mins}m`;
};

const clockText = (instant: Date): string =>
	instant.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });

// The rest is a pure port of the standalone widget's seam view
// (widget/seam.go seamBasis/seamDetail, main.go detailResetLabel and
// resetLabels), so Bloom's quota space names and formats exactly what the
// Go painter did.

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

const windowName = (label: string): string =>
	label === "week" ? "Week" : label === "day" ? "Day" : label;

// poolWindow in widget/pools.go: a 5h pool can be reported at the top level
// or inside a model group.
const hasPoolWindow = (a: QuotaAgent, label: string): boolean =>
	(a.windows ?? []).some((w) => w.label === label) ||
	(a.groups ?? []).some((g) => g.windows.some((w) => w.label === label));

// The basis caption under each ring cell.
export function basis(a: QuotaAgent): string {
	if (a.state === "waiting") return "Checking…";
	if (a.state === "failed") return "Unavailable";
	if (a.state === "stale") return "Stale";
	if (hasPoolWindow(a, "5h")) return "5h";
	if ((a.summary ?? "").toLowerCase() === "unlimited") return "Unlimited";
	if ((a.groups?.length ?? 0) > 0) return "Models";
	if ((a.windows?.length ?? 0) > 0) {
		let w = a.windows![0];
		for (const candidate of a.windows!.slice(1)) {
			if (candidate.used > w.used) w = candidate;
		}
		return windowName(w.label);
	}
	if (a.id === "kiro") return "Credits";
	if (a.id === "cursor") return "Plan";
	if (a.id === "copilot") return "Premium";
	return "Usage";
}

// One string per detail row of the selected agent: `Label · Value · reset`.
// Go breaks to a second line when the row overflows; CSS wraps here instead.
export function detailLines(a: QuotaAgent, now: Date, clock: boolean): string[] {
	if (a.state === "waiting") return ["Checking usage…"];
	if (a.state === "failed") {
		const message = a.err || a.detail || "";
		return message ? [message] : [];
	}
	const details = a.details ?? [];
	if (details.length === 0) {
		const s = seamDetailText(a, now, clock);
		return s ? [s] : [];
	}
	return details.map((row) => {
		let value = row.label;
		if (row.value) value = value ? value + " · " + row.value : row.value;
		const reset = detailResetText(row, now, clock);
		if (reset) value += " · " + reset;
		return value;
	});
}

export function failedCount(readings: QuotaAgent[]): number {
	return readings.filter((a) => a.state === "failed").length;
}

export function anyBusy(readings: QuotaAgent[]): boolean {
	return readings.some((a) => a.busy === true);
}

// First urgent (90%+) or failed agent, else the first agent.
export function defaultSelection(readings: QuotaAgent[]): string | undefined {
	return (
		readings.find((a) => urgent(a) || a.state === "failed")?.id ?? readings[0]?.id
	);
}

// One reset instant as a countdown or wall-clock string; a past instant
// reads "now" rather than a negative countdown.
export function formatReset(at: string, now: Date, clock: boolean): string {
	const instant = new Date(at);
	if (Number.isNaN(instant.getTime())) return "";
	if (instant.getTime() <= now.getTime()) return "now";
	return clock ? clockText(instant) : countdownText(instant, now);
}

// Go's `resets in …` text for a detail row, floored to whole minutes.
const detailCountdown = (instant: Date, now: Date): string => {
	const mins = Math.floor((instant.getTime() - now.getTime()) / 60000);
	const days = Math.floor(mins / 1440);
	if (days > 0) return `${days}d ${Math.floor(mins / 60) % 24}h ${mins % 60}m`;
	const hours = Math.floor(mins / 60);
	return hours > 0 ? `${hours}h ${mins % 60}m` : `${mins}m`;
};

const pad2 = (n: number): string => String(n).padStart(2, "0");

// Go's FormatLocal "d MMM HH:mm" ("23 Oct 08:00").
const dayClockText = (instant: Date): string =>
	`${instant.getDate()} ${MONTHS[instant.getMonth()]} ${pad2(instant.getHours())}:${pad2(instant.getMinutes())}`;

const sameLocalDay = (a: Date, b: Date): boolean =>
	a.getFullYear() === b.getFullYear() &&
	a.getMonth() === b.getMonth() &&
	a.getDate() === b.getDate();

// detailResetLabel in main.go: resetText fills in when no instant arrives;
// a passed instant shows its wall-clock date rather than "in -3m".
const detailResetText = (row: QuotaDetail, now: Date, clock: boolean): string => {
	const instant = row.resetAt ? new Date(row.resetAt) : undefined;
	if (!instant || Number.isNaN(instant.getTime())) {
		return row.resetText ? "resets " + row.resetText : "";
	}
	if (instant.getTime() <= now.getTime()) return "resets at " + dayClockText(instant);
	if (clock) {
		return sameLocalDay(instant, now)
			? "resets " + clockText(instant)
			: "resets " + dayClockText(instant);
	}
	return "resets in " + detailCountdown(instant, now);
};

// resetLabels in main.go: the 5h row's own reset sentence. A past instant
// still reads "now" here (Bloom keeps the line instead of dropping it).
const reset5hPanelText = (at: string | undefined, now: Date, clock: boolean): string => {
	const instant = at ? new Date(at) : undefined;
	if (!instant || Number.isNaN(instant.getTime())) return "";
	if (instant.getTime() <= now.getTime()) return "5h resets now";
	if (clock) {
		return sameLocalDay(instant, now)
			? "5h resets " + clockText(instant)
			: "5h resets " + dayClockText(instant);
	}
	return "5h resets in " + detailCountdown(instant, now);
};

// metrics in widget/widget.go: summary, else every window as `Name nn%`,
// else the freeform detail string.
const metricsText = (a: QuotaAgent): string => {
	if (a.summary) return a.summary;
	const parts = (a.windows ?? []).map(
		(w) => `${windowName(w.label)} ${wholePercent(w.used)}%`
	);
	return parts.length ? parts.join("   ") : (a.detail ?? "");
};

// quota.Custom(v, 0): .NET "0" rounding, half away from zero.
const wholePercent = (v: number): string =>
	(v < 0 ? "-" : "") + Math.round(Math.abs(v));

// seamDetail in widget/seam.go: the fallback block when no per-window
// detail rows arrived.
const seamDetailText = (a: QuotaAgent, now: Date, clock: boolean): string => {
	// Go only turns reset5h into a row's reset when the state is OK
	// (main.go buildView); a stale row falls back to its resetLabel.
	const reset = a.state === "ok" ? reset5hPanelText(a.reset5h, now, clock) : "";
	if ((a.groups?.length ?? 0) > 0) {
		let s = `${a.groups!.length} model groups`;
		if (reset) s += "  ·  " + reset;
		return s;
	}
	if (reset) {
		return (a.windows?.length ?? 0) > 0 ? metricsText(a) + "  ·  " + reset : reset;
	}
	let s = metricsText(a);
	if (a.resetLabel) s += "  ·  resets " + a.resetLabel;
	return s;
};
