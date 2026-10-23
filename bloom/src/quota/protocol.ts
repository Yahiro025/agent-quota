// Client half of the quota helper protocol: the helper writes one compact
// JSON object per line on stdout (see the Protocol section of
// docs/superpowers/specs/2026-10-22-bloom-agent-quota-design.md). Only
// snapshot lines carry readings; ready, bye, error, blank, and non-JSON
// lines parse to null so the caller can fold every line through one check.

export type QuotaState = "waiting" | "ok" | "stale" | "failed";

export interface QuotaWindow {
	label: string;
	used: number;
}

export interface QuotaGroup {
	name: string;
	windows: QuotaWindow[];
}

export interface QuotaDetail {
	label: string;
	value: string;
	resetAt?: string;
	resetText?: string;
}

// One agent reading inside a snapshot. `used` is absent when the agent has
// no percentage at all, so a reported 0 must survive as a number; times are
// RFC3339Nano UTC strings, and "stale" keeps the last good `used` beside
// the latest failure in `err`.
export interface QuotaAgent {
	id: string;
	state: QuotaState;
	busy?: boolean;
	used?: number;
	detail?: string;
	err?: string;
	help?: string;
	plan?: string;
	summary?: string;
	resetLabel?: string;
	reset5h?: string;
	observedAt?: string;
	lastGoodAt?: string;
	windows?: QuotaWindow[];
	groups?: QuotaGroup[];
	details?: QuotaDetail[];
}

// Returns the readings array for a snapshot line and null for anything
// else, including a snapshot with no readings array.
export function parseSnapshot(line: string): QuotaAgent[] | null {
	const trimmed = line.trim();
	if (trimmed === "") {
		return null;
	}
	let message: { op?: unknown; readings?: unknown };
	try {
		message = JSON.parse(trimmed);
	} catch {
		return null;
	}
	if (message?.op !== "snapshot" || !Array.isArray(message.readings)) {
		return null;
	}
	return message.readings as QuotaAgent[];
}

// Ring colour thresholds, shared with the Go painter: green below 70,
// amber from 70, red from 90.
export function band(used: number): "green" | "amber" | "red" {
	if (used >= 90) {
		return "red";
	}
	if (used >= 70) {
		return "amber";
	}
	return "green";
}

// Urgent means a red reading, nothing else: a missing percentage is never
// urgent, and timer completion is the caller's concern, not this check's.
export function urgent(agent: QuotaAgent): boolean {
	return typeof agent.used === "number" && agent.used >= 90;
}
