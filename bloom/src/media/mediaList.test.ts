import { expect, test } from "bun:test";
import {
	EMPTY_MEDIA_LIST,
	parseMediaList,
	pickFocus,
	progressPercent,
	type MediaCard
} from "./mediaList";
const card = (extra: Record<string, unknown> = {}) => ({
	generation: "spotify:a",
	source: "Spotify",
	title: "Track",
	artist: "Artist",
	is_playing: true,
	artwork: ["blob:art"],
	position_ms: 1000,
	duration_ms: 10000,
	seek_enabled: true,
	position_updated_at: 1000,
	queue: true,
	primary: false,
	...extra
});
test("a well-formed list keeps every card and its snake_case fields", () => {
	const list = parseMediaList({ cards: [card(), card({ generation: "youtube:b", source: "YouTube", artwork: null })], other_audio: true });
	expect(list?.cards.map((c) => c.generation)).toEqual(["spotify:a", "youtube:b"]);
	expect(list?.cards[1].artwork).toBeNull();
	expect(list?.other_audio).toBe(true);
});
test("a malformed list is ignored so the last good list stays on screen", () => {
	expect(parseMediaList(null)).toBeNull();
	expect(parseMediaList("cards")).toBeNull();
	expect(parseMediaList({ other_audio: true })).toBeNull();
	expect(parseMediaList({ cards: "none", other_audio: false })).toBeNull();
});
test("a single unusable card is dropped, not the whole list", () => {
	const list = parseMediaList({
		cards: [
			card(),
			card({ generation: "" }),
			card({ source: "Speaker" }),
			card({ is_playing: "yes" }),
			card({ queue: undefined }),
			card({ artwork: "blob:art" })
		],
		other_audio: "true"
	});
	expect(list?.cards).toHaveLength(2);
	// Only a real boolean counts; a stray string must not show the row.
	expect(list?.other_audio).toBe(false);
	expect(parseMediaList({ cards: [card({ artwork: "blob:art" })] })?.cards[0].artwork).toBeNull();
});
test("progress extrapolates while playing and never leaves the bar", () => {
	const playing: MediaCard = { ...card(), position_updated_at: 1000 } as MediaCard;
	expect(progressPercent(playing, 6000)).toBeCloseTo(60, 5);
	expect(progressPercent({ ...playing, is_playing: false }, 6000)).toBeCloseTo(10, 5);
	expect(progressPercent({ ...playing, duration_ms: 0 }, 6000)).toBe(0);
	expect(progressPercent({ ...playing, position_ms: 99000 }, 6000)).toBe(100);
	expect(EMPTY_MEDIA_LIST.cards).toHaveLength(0);
});

test("app falls back to the source, and Windows never leaks as a player name", () => {
	const list = parseMediaList({
		cards: [
			card({ app: "Brave", source: "Windows" }),
			card({ source: "Spotify" }),
			card({ app: "Windows", source: "Windows" }),
			card({ app: 42, source: "YouTube" })
		]
	});
	expect(list?.cards.map((c) => c.app)).toEqual(["Brave", "Spotify", "Media", "YouTube"]);
});

test("kind and muted default safely, and a tab card survives parsing", () => {
	const list = parseMediaList({
		cards: [
			card(),
			card({ kind: "video", muted: "true" }),
			card({ generation: "youtube:tab:7", source: "Tab", kind: "tab", muted: true, duration_ms: 0 })
		]
	});
	expect(list?.cards.map((c) => c.kind)).toEqual(["player", "player", "tab"]);
	expect(list?.cards.map((c) => c.muted)).toEqual([false, false, true]);
	// Tab is a real source, so the card is not dropped with the unknown ones.
	expect(list?.cards).toHaveLength(3);
});

test("focus keeps the current card, else the primary, else the first", () => {
	const cards = [
		{ ...card(), generation: "a", primary: false } as MediaCard,
		{ ...card(), generation: "b", primary: true } as MediaCard,
		{ ...card(), generation: "c", primary: false } as MediaCard
	];
	expect(pickFocus(cards, "c")?.generation).toBe("c");
	expect(pickFocus(cards, "gone")?.generation).toBe("b");
	expect(pickFocus(cards, null)?.generation).toBe("b");
	expect(pickFocus([cards[0], cards[2]], null)?.generation).toBe("a");
	expect(pickFocus([], "a")).toBeNull();
});
