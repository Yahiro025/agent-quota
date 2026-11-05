// Payload of the `media-list` event and `media_list_get` (snake_case, like
// `media-update`). Kept free of Tauri imports so the payload checks can be
// unit-tested without a webview.
export type MediaSource = "Spotify" | "YouTube" | "Windows" | "Tab";

/** An audible browser tab can only be muted or brought forward, never played. */
export type MediaKind = "player" | "tab";

export interface MediaCard {
	generation: string;
	source: MediaSource;
	/** Player display name ("Spotify", "Brave", …); never the raw word "Windows". */
	app: string;
	title: string;
	artist: string;
	is_playing: boolean;
	artwork?: string[] | null;
	position_ms: number;
	duration_ms: number;
	seek_enabled: boolean;
	position_updated_at: number;
	queue: boolean;
	primary: boolean;
	kind: MediaKind;
	muted: boolean;
}

export interface MediaList {
	cards: MediaCard[];
	other_audio: boolean;
}

export const EMPTY_MEDIA_LIST: MediaList = { cards: [], other_audio: false };

const SOURCES: readonly string[] = ["Spotify", "YouTube", "Windows", "Tab"];
const isText = (v: unknown): v is string => typeof v === "string";

/** The backend names Windows players from their SMTC id; older payloads have no app. */
function displayApp(source: MediaSource, app: unknown): string {
	const name = isText(app) ? app.trim() : "";
	if (name && name !== "Windows") return name;
	return source === "Windows" ? "Media" : source;
}
const isBool = (v: unknown): v is boolean => typeof v === "boolean";
const num = (v: unknown) => (typeof v === "number" && Number.isFinite(v) ? v : 0);

function parseCard(value: unknown): MediaCard | null {
	if (!value || typeof value !== "object") return null;
	const c = value as Record<string, unknown>;
	// A card without an addressable generation could neither play nor queue,
	// so one bad card is dropped instead of poisoning the whole list.
	if (!isText(c.generation) || !c.generation) return null;
	if (!isText(c.title) || !isText(c.artist) || !isText(c.source)) return null;
	if (!SOURCES.includes(c.source)) return null;
	if (!isBool(c.is_playing) || !isBool(c.queue) || !isBool(c.primary) || !isBool(c.seek_enabled))
		return null;
	return {
		generation: c.generation,
		source: c.source as MediaSource,
		app: displayApp(c.source as MediaSource, c.app),
		title: c.title,
		artist: c.artist,
		is_playing: c.is_playing,
		artwork:
			Array.isArray(c.artwork) && c.artwork.every(isText) ? (c.artwork as string[]) : null,
		position_ms: num(c.position_ms),
		duration_ms: num(c.duration_ms),
		seek_enabled: c.seek_enabled,
		position_updated_at: num(c.position_updated_at),
		queue: c.queue,
		primary: c.primary,
		// Older payloads predate tabs; a stray kind must not cost the card.
		kind: c.kind === "tab" ? "tab" : "player",
		muted: c.muted === true
	};
}

/** Returns null for a payload this build cannot trust, so the caller keeps the last good list. */
export function parseMediaList(value: unknown): MediaList | null {
	if (!value || typeof value !== "object") return null;
	const raw = value as Record<string, unknown>;
	if (!Array.isArray(raw.cards)) return null;
	const cards: MediaCard[] = [];
	for (const item of raw.cards) {
		const card = parseCard(item);
		if (card) cards.push(card);
	}
	return { cards, other_audio: raw.other_audio === true };
}

/** Position is extrapolated between list events, which only arrive on >1 s moves. */
export function progressPercent(card: MediaCard, now = Date.now()): number {
	if (card.duration_ms <= 0) return 0;
	const elapsed = card.is_playing ? Math.max(0, now - card.position_updated_at) : 0;
	return Math.min(100, Math.max(0, ((card.position_ms + elapsed) / card.duration_ms) * 100));
}

/**
 * The focused card: keep the current one while it is still playing, else the
 * primary, else the first. Derived on every render so a card that disappears
 * hands focus over in the same frame.
 */
export function pickFocus(cards: MediaCard[], current: string | null): MediaCard | null {
	if (cards.length === 0) return null;
	return (
		(current ? cards.find((c) => c.generation === current) : undefined) ??
		cards.find((c) => c.primary) ??
		cards[0]
	);
}
