import { expect, test } from "bun:test";
import { createLyricsFeed, lyricsPath, parseLyrics } from "./lyrics.js";

test("timed lyrics keep their millisecond starts", () => {
	const sheet = parseLyrics({
		lyrics: {
			syncType: "LINE_SYNCED",
			lines: [
				{ startTimeMs: "0", words: "intro" },
				{ startTimeMs: "5400", words: "verse" }
			]
		}
	});
	expect(sheet).toEqual({
		synced: true,
		lines: [
			{ t: 0, text: "intro" },
			{ t: 5400, text: "verse" }
		]
	});
});

test("unsynced and empty sheets are not clickable", () => {
	expect(
		parseLyrics({ lyrics: { syncType: "UNSYNCED", lines: [{ startTimeMs: "0", words: "a" }] } })
	).toEqual({ synced: false, lines: [{ t: 0, text: "a" }] });
	expect(parseLyrics({ lyrics: { syncType: "LINE_SYNCED", lines: [] } })).toEqual({
		synced: false,
		lines: []
	});
	expect(parseLyrics(null)).toEqual({ synced: false, lines: [] });
});

test("the path carries the track id and no credentials", () => {
	expect(lyricsPath("abc123")).toBe(
		"https://spclient.wg.spotify.com/color-lyrics/v2/track/abc123?format=json&vocalRemoval=false&market=from_token"
	);
});

const tick = () => new Promise((resolve) => setTimeout(resolve, 0));
const snapshot = (id, occurrence = id) => ({ session: "session", current_id: occurrence, current_uri: `spotify:track:${id}` });
const payload = (text) => ({ lyrics: { syncType: "LINE_SYNCED", lines: [{ startTimeMs: "0", words: text }] } });

test("lyrics use the track URI and resend cached sheets on replay and reconnect", async () => {
	const paths = [], sent = [];
	const publish = createLyricsFeed(async (path) => { paths.push(path); return payload("line"); });
	const send = (message) => sent.push(message);
	for (const id of ["a", "b", "a"]) { publish(send, snapshot(id, `queue-${id}`)); await tick(); }
	expect(paths).toEqual([lyricsPath("a"), lyricsPath("b")]);
	expect(sent.map((sheet) => sheet.track_id)).toEqual(["queue-a", "queue-b", "queue-a"]);
	publish(send, snapshot("a", "queue-a"));
	expect(sent).toHaveLength(3);
	publish((message) => sent.push(message), snapshot("a", "queue-a"));
	expect(sent).toHaveLength(4);
});

test("late replies cannot replace the latest selection", async () => {
	const requests = new Map(), sent = [];
	const publish = createLyricsFeed((path) => new Promise((resolve) => requests.set(path, resolve)));
	const send = (sheet) => sent.push(sheet);
	publish(send, snapshot("a")); publish(send, snapshot("b")); await tick();
	requests.get(lyricsPath("b"))(payload("B")); await tick();
	requests.get(lyricsPath("a"))(payload("A")); await tick();
	expect(sent.map((sheet) => sheet.track_id)).toEqual(["b"]);
});

test("failed sheets obey the same eight-track cache bound", async () => {
	let requests = 0;
	const publish = createLyricsFeed(async () => { requests++; throw new Error("unavailable"); });
	const send = () => {};
	for (let i = 0; i < 9; i++) { publish(send, snapshot(`track${i}`)); await tick(); }
	publish(send, snapshot("track0")); await tick();
	expect(requests).toBe(10);
});

test("multibyte lines fit the Rust byte cap without broken surrogates", () => {
	for (const text of ["\u4e2d".repeat(200), "\ud83d\ude00".repeat(200), "x".repeat(499) + "\ud800"]) {
		const line = parseLyrics(payload(text)).lines[0];
		expect(new TextEncoder().encode(line.text).length).toBeLessThanOrEqual(500);
		expect(line.text.isWellFormed()).toBe(true);
	}
});
