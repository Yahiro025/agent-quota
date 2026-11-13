// View model for the Agent Quotas panel: folds one helper snapshot into
// rows the panel renders as-is. Kept React-free so the bun tests exercise
// the same code the component draws. The helper never sends formatted
// time — each row's countdown and clock are both formatted here from that
// row's one reset5h instant, and each detail's both from its resetAt, so
// the two displays can never disagree.

import { band, urgent, type QuotaAgent, type QuotaDetail, type QuotaWindow } from "./protocol";
import { agentName } from "./marks";

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

// One structured metric row of the detail card: label, band-painted
// percentage when a number exists, and that row's own reset sentence.
export interface QuotaMetricRow {
	label: string;
	used?: number;
	value: string;
	reset?: string;
	resetClock?: string;
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

// `panel` is the currently open notch panel. `showInRest` is true when an
// urgent reading exists and the quotas panel is not already open. The strip
// still waits for a hover; see `quotaAlertVisible`.
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

// The usage-limit strip is a hover detail of the collapsed notch. An urgent
// reading does not paint it on the resting island, and an open panel is left
// alone so calendar, music, and the quota space stay visible.
export function quotaAlertVisible(showInRest: boolean, hovered: boolean, open: boolean): boolean {
	return showInRest && hovered && !open;
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
	if (a.id === "grokbot") return "Week";
	if (a.id === "qoder") return "Credits";
	return "Usage";
}

// One string per detail row of the selected agent: `Label · Value · reset`.
// The panel draws the same rows via detailRows; these lines feed the hover
// tooltip so the two surfaces never disagree.
export function detailLines(a: QuotaAgent, now: Date, clock: boolean): string[] {
	if (a.state === "waiting") return ["Checking usage…"];
	if (a.state === "failed") {
		const message = a.err || a.detail || "";
		return message ? [message] : [];
	}
	return detailRows(a, now, clock).map((row) =>
		[row.label, row.value, row.reset].filter(Boolean).join(" · ")
	);
}

// The selected agent's usage as structured rows. `details` rows carry their
// own labels and reset instants; bare `windows` rows get a reset only from
// reset5h (5h window or a lone window) or resetLabel on a lone window;
// grouped agents pool their 5h window into one summary row.
export function detailRows(a: QuotaAgent, now: Date, clock: boolean): QuotaMetricRow[] {
	if (a.state === "waiting" || a.state === "failed") return [];
	if ((a.details?.length ?? 0) > 0) {
		return a.details!.map((row) => {
			const m = /^\s*(\d+(?:\.\d+)?)%/.exec(row.value);
			const pooled = m
				? undefined
				: (a.windows ?? []).find(
						(w) => w.label.toLowerCase() === row.label.toLowerCase()
					);
			const parsed = m ? parseFloat(m[1]) : pooled?.used;
			const used =
				typeof parsed === "number"
					? Math.min(Math.max(parsed, 0), 100)
					: undefined;
			const value =
				used !== undefined
					? `${wholePercent(used)}%`
					: row.value.replace(/\s+used$/i, "").trim() || row.value;
			return withClock(
				{
					label: row.label,
					used,
					value,
					reset: detailResetText(row, now, clock) || undefined
				},
				row.resetAt,
				now,
				clock
			);
		});
	}
	if ((a.windows?.length ?? 0) > 0) {
		return a.windows!.map((w) => {
			const fromInstant =
				(w.label === "5h" || a.windows!.length === 1) && a.state === "ok" && a.reset5h;
			return withClock(
				{
					label: windowName(w.label),
					used: w.used,
					value: `${wholePercent(w.used)}%`,
					reset: fromInstant
						? isoResetText(a.reset5h, now, clock) || undefined
						: a.resetLabel && a.windows!.length === 1
							? "resets " + a.resetLabel
							: undefined
				},
				fromInstant ? a.reset5h : undefined,
				now,
				clock
			);
		});
	}
	if ((a.groups?.length ?? 0) > 0) {
		const used = poolWindow(a, "5h") ?? a.used;
		if (typeof used === "number" || a.reset5h) {
			return [
				withClock(
					{
						label: "5h",
						used: typeof used === "number" ? used : undefined,
						value: typeof used === "number" ? `${wholePercent(used)}%` : "—",
						reset:
							a.state === "ok" && a.reset5h
								? isoResetText(a.reset5h, now, clock) || undefined
								: undefined
					},
					a.state === "ok" ? a.reset5h : undefined,
					now,
					clock
				)
			];
		}
	}
	if (typeof a.used === "number") {
		const fromInstant = a.state === "ok" && !!a.reset5h;
		return [
			withClock(
				{
					label: basis(a),
					used: a.used,
					value: `${wholePercent(a.used)}%`,
					reset: fromInstant
						? isoResetText(a.reset5h, now, clock) || undefined
						: a.resetLabel
							? "resets " + a.resetLabel
							: undefined
				},
				fromInstant ? a.reset5h : undefined,
				now,
				clock
			)
		];
	}
	if (a.summary || a.detail) {
		return [{ label: basis(a), value: a.summary || a.detail || "" }];
	}
	return [];
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

// `resets in …` text for a detail row: whole hours drop the trailing "0m",
// multi-day resets drop minute noise, and a past instant reads "now".
const detailCountdown = (instant: Date, now: Date): string => {
	if (instant.getTime() <= now.getTime()) return "now";
	const minutes = Math.max(
		1,
		Math.round((instant.getTime() - now.getTime()) / 60000)
	);
	const days = Math.floor(minutes / 1440);
	const hours = Math.floor((minutes % 1440) / 60);
	const mins = minutes % 60;
	if (days > 0) return hours > 0 ? `${days}d ${hours}h` : `${days}d`;
	if (hours > 0) return mins > 0 ? `${hours}h ${mins}m` : `${hours}h`;
	return `${mins}m`;
};

const sameLocalDay = (a: Date, b: Date): boolean =>
	a.getFullYear() === b.getFullYear() &&
	a.getMonth() === b.getMonth() &&
	a.getDate() === b.getDate();

// detailResetLabel in main.go: resetText fills in when no instant arrives;
// a passed instant shows its wall-clock date rather than "in -3m".
const detailResetText = (row: QuotaDetail, now: Date, clock: boolean): string => {
    return isoResetText(row.resetAt, now, clock) || (row.resetText ? "resets " + row.resetText : "");
};

// A reset5h ISO instant as the same reset sentence a detail row gets; the
// row's own label already names the window, so no "5h" prefix here.
const isoResetText = (at: string | undefined, now: Date, clock: boolean): string => {
	const instant = at ? new Date(at) : undefined;
	if (!instant || Number.isNaN(instant.getTime())) return "";
	if (instant.getTime() <= now.getTime()) return "resets now";
	if (clock) return "resets " + wallClock(instant, now);
	return "resets in " + detailCountdown(instant, now);
};

// Same locale clock as the notch. A later day keeps the "d MMM" date in front of it.
export function wallClock(instant: Date, now: Date): string {
	const time = clockText(instant);
	if (sameLocalDay(instant, now)) return time;
	return `${instant.getDate()} ${MONTHS[instant.getMonth()]} ${time}`;
}

const withClock = (
	row: QuotaMetricRow,
	iso: string | undefined,
	now: Date,
	clock: boolean
): QuotaMetricRow => {
	if (clock || !iso) return row;
	const instant = new Date(iso);
	if (Number.isNaN(instant.getTime()) || instant.getTime() <= now.getTime()) return row;
	return { ...row, resetClock: wallClock(instant, now) };
};

// quota.Custom(v, 0): .NET "0" rounding, half away from zero.
const wholePercent = (v: number): string =>
	(v < 0 ? "-" : "") + Math.round(Math.abs(v));

// --- collapsed-notch hover badge --------------------------------------
// poolWindow in widget/pools.go: the fullest window carrying a label wins,
// whether it sits at the top level or inside a model group.
const poolWindow = (a: QuotaAgent, label: string): number | undefined => {
	let found: number | undefined;
	const visit = (windows: QuotaWindow[] | undefined) => {
		for (const w of windows ?? []) {
			if (w.label === label && (found === undefined || w.used > found)) found = w.used;
		}
	};
	visit(a.windows);
	for (const g of a.groups ?? []) visit(g.windows);
	return found;
};

// headlineUsed in widget/pools.go: the 5h pool when reported, else the
// agent's own percentage.
const headlineUsed = (a: QuotaAgent): number | undefined => poolWindow(a, "5h") ?? a.used;

// The one agent the collapsed notch's hover badge shows: the pinned
// selection, else the busiest numeric reading (ties keep the earlier
// agent; a reported 0 still beats no number), else the default selection.
export function hoverQuotaAgent(
	readings: QuotaAgent[],
	selected?: string
): QuotaAgent | undefined {
	if (readings.length === 0) return undefined;
	if (selected !== undefined) {
		const pinned = readings.find((a) => a.id === selected);
		if (pinned) return pinned;
	}
	let best: QuotaAgent | undefined;
	let bestUsed = -Infinity;
	for (const a of readings) {
		const used = headlineUsed(a);
		if (typeof used === "number" && used > bestUsed) {
			best = a;
			bestUsed = used;
		}
	}
	if (best !== undefined) return best;
	return readings.find((a) => a.id === defaultSelection(readings)) ?? readings[0];
}

// The agent after currentId in snapshot order, wrapping to the first.
// A missing current lands on the first reading. A one-agent list stays put.
// Waiting and failed readings are real steps.
export function nextAgent(readings: QuotaAgent[], currentId?: string): string | undefined {
	if (readings.length === 0) return undefined;
	const index = readings.findIndex((a) => a.id === currentId);
	return readings[(index + 1) % readings.length]?.id;
}

// The two concentric arcs around an agent mark: inner is the shorter
// window (5h, or Devin's day), outer the longer one (week). With fewer
// than two usage metrics the outer track stays empty; a waiting or failed
// agent draws both tracks empty. Windows are pooled from `a.windows` and
// `a.groups[].windows` like pools.go, and detail rows can donate their
// leading percentage when the windows alone can't fill two rings.
export function agentRings(a: QuotaAgent): {
	inner: number | undefined;
	outer: number | undefined;
} {
	if (a.state === "waiting" || a.state === "failed") {
		return { inner: undefined, outer: undefined };
	}
	const fiveH = poolWindow(a, "5h");
	const week = poolWindow(a, "week");
	if (fiveH !== undefined && week !== undefined) return { inner: fiveH, outer: week };
	const day = poolWindow(a, "day");
	if (day !== undefined && week !== undefined) return { inner: day, outer: week };

	// Distinct window values in reported order, then detail percentages.
	const wins: number[] = [];
	const labels = new Set<string>();
	const collect = (windows: QuotaWindow[] | undefined) => {
		for (const w of windows ?? []) {
			if (labels.has(w.label)) continue;
			labels.add(w.label);
			wins.push(w.used);
		}
	};
	collect(a.windows);
	for (const g of a.groups ?? []) collect(g.windows);
	if (wins.length < 2) {
		for (const d of a.details ?? []) {
			const m = /^(\d+(?:\.\d+)?)%/.exec(d.value) ?? /^(\d+(?:\.\d+)?)%/.exec(d.label);
			if (m === null) continue;
			const pct = parseFloat(m[1]);
			if (!wins.includes(pct)) wins.push(pct);
			if (wins.length >= 2) break;
		}
	}
	if (wins.length >= 2) {
		const inner = typeof a.used === "number" ? a.used : wins[0];
		const outer = wins.find((v) => v !== inner) ?? wins[1];
		return { inner, outer };
	}
	if (wins.length === 1 || typeof a.used === "number") {
		return { inner: headlineUsed(a), outer: undefined };
	}
	return { inner: undefined, outer: undefined };
}

// The badge's tooltip: name and plan on the first line, then the same
// detail lines the panel would draw (or the basis caption when there are
// none), so the hover text never disagrees with the open space.
export function hoverQuotaTooltip(a: QuotaAgent, now: Date = new Date()): string {
	const head = agentName(a.id) + (a.plan ? ` · ${a.plan}` : "");
	const lines = detailLines(a, now, false);
	return [head, ...(lines.length > 0 ? lines : [basis(a)])].join("\n");
}
