// Chooses how much of each agent the MyFinder bar strip shows. MyFinder's own
// menu grows and shrinks with the foreground app, and Bloom's strip must
// never cover it, so the strip gives up detail in steps as the free width
// between the menu and the clock falls.
import { band, type QuotaAgent } from "./protocol";
import { agentRings } from "./quotaView";

/** Widest first. Each later stage drops something the earlier one shows. */
export type StripStage = "full" | "value" | "hot" | "marks" | "top" | "dot";

export interface StripFit {
	stage: StripStage;
	/** The agents drawn, in their snapshot order. */
	shown: QuotaAgent[];
	/** Agents folded into the "+N" segment. */
	hidden: number;
}

/** The headline percent, or undefined while the agent has no reading. */
export function stripHeadline(a: QuotaAgent): number | undefined {
	return (a.state === "ok" || a.state === "stale") && typeof a.used === "number"
		? Math.round(a.used)
		: undefined;
}

/** The other window's percent when the agent reports two of them. */
export function stripSecond(a: QuotaAgent): number | undefined {
	const { inner, outer } = agentRings(a);
	return inner === undefined || outer === undefined
		? undefined
		: Math.round(Math.min(inner, outer));
}

/** An agent at or above the amber band keeps its number in the "hot" stage. */
export function stripShowsValue(a: QuotaAgent, stage: StripStage): boolean {
	if (stage === "full" || stage === "value") return true;
	const used = stripHeadline(a);
	return stage === "hot" && used !== undefined && band(used) !== "green";
}

// These widths mirror `.quota-housing` in myfinder.css: a 20px ring with 5px
// either side, 12px digits and an 11px second number. An estimate a few pixels
// off is harmless, because the caller keeps a 16px gap to the menu.
const SEGMENT = 30;
const HOUSING_PAD = 6;
const digits = (n: number | undefined, each: number) => (n === undefined ? 1 : String(n).length) * each;

function segmentWidth(a: QuotaAgent, stage: StripStage): number {
	let width = SEGMENT;
	if (stripShowsValue(a, stage)) width += 4 + digits(stripHeadline(a), 7.5) + 9;
	if (stage === "full" && stripSecond(a) !== undefined) width += 4 + digits(stripSecond(a), 6.5);
	return width;
}

/** Width of the housing for `agents` at `stage`, in CSS pixels. */
export function housingWidth(agents: QuotaAgent[], stage: StripStage): number {
	return HOUSING_PAD + agents.reduce((sum, a) => sum + segmentWidth(a, stage), 0);
}

const DOT_WIDTH = 22;
const MORE_WIDTH = 30;

/** The widest stage whose housing fits in `room` CSS pixels. */
export function fitQuotaStrip(agents: QuotaAgent[], room: number): StripFit {
	for (const stage of ["full", "value", "hot", "marks"] as const) {
		if (housingWidth(agents, stage) <= room) return { stage, shown: agents, hidden: 0 };
	}
	// The fullest agents stay: the strip exists to warn, and a full agent
	// folded into "+N" would defeat that.
	const byUsage = agents
		.map((a, index) => ({ a, index, used: stripHeadline(a) ?? -1 }))
		.sort((x, y) => y.used - x.used || x.index - y.index);
	for (let keep = agents.length - 1; keep >= 1; keep--) {
		const kept = new Set(byUsage.slice(0, keep).map((entry) => entry.a.id));
		const shown = agents.filter((a) => kept.has(a.id));
		if (housingWidth(shown, "marks") + MORE_WIDTH <= room) {
			return { stage: "top", shown, hidden: agents.length - keep };
		}
	}
	return { stage: "dot", shown: [], hidden: agents.length };
}

/** The band of the fullest agent: the colour of the single dot. */
export function worstBand(agents: QuotaAgent[]): ReturnType<typeof band> | undefined {
	const used = agents.map(stripHeadline).filter((n): n is number => n !== undefined);
	return used.length === 0 ? undefined : band(Math.max(...used));
}

export const STRIP_DOT_WIDTH = DOT_WIDTH;
