// Which agents the notch's quota surfaces draw. The helper keeps polling
// every reader; this is a display filter only. The setting stores the hidden
// ids rather than the visible ones, so an agent added in a later release
// still appears for existing users instead of being silently absent.

import type { QuotaAgent } from "./protocol";

export const QUOTA_HIDDEN_AGENTS_KEY = "bloom-quota-hidden-agents";

// Accepts the settings file's parsed array or its JSON string; anything else
// reads as "nothing hidden", so bad data can never blank the whole panel.
export function parseHiddenAgents(value: unknown): string[] {
	if (Array.isArray(value)) {
		return value.filter((id): id is string => typeof id === "string" && id !== "");
	}
	if (typeof value !== "string" || value.trim() === "") return [];
	try {
		return parseHiddenAgents(JSON.parse(value));
	} catch {
		return [];
	}
}

export function isAgentHidden(hidden: string[], id: string): boolean {
	return hidden.includes(id);
}

export function toggleHiddenAgent(hidden: string[], id: string): string[] {
	return isAgentHidden(hidden, id)
		? hidden.filter((hiddenId) => hiddenId !== id)
		: [...hidden, id];
}

// Snapshot order is the panel's display order, so filtering preserves it.
export function visibleReadings(readings: QuotaAgent[], hidden: string[]): QuotaAgent[] {
	if (hidden.length === 0) return readings;
	return readings.filter((a) => !isAgentHidden(hidden, a.id));
}
