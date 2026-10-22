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
