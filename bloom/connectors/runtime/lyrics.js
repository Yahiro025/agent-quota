// Spotify's color-lyrics response is undocumented; keep the shape handling in
// one place so a field rename fails here and nowhere else.
export function lyricsPath(trackId) {
	return `https://spclient.wg.spotify.com/color-lyrics/v2/track/${encodeURIComponent(trackId)}?format=json&vocalRemoval=false&market=from_token`;
}

export function parseLyrics(payload) {
	const body = payload?.lyrics;
	const synced = body?.syncType === "LINE_SYNCED";
	const lines = (Array.isArray(body?.lines) ? body.lines : [])
		.slice(0, 500)
		.map((line) => {
			const start = Number(line?.startTimeMs);
			return {
				t: Number.isSafeInteger(start) && start >= 0 && start <= 31_536_000_000 ? start : 0,
				text: clipLyric(String(line?.words ?? ""))
			};
		})
		.filter((line) => line.text.trim());
	if (synced) lines.sort((a, b) => a.t - b.t);
	return { synced: synced && lines.length > 0, lines };
}

// Rust checks UTF-8 bytes. Never split a surrogate pair or exceed its wire cap.
function clipLyric(text) {
	let result = "", bytes = 0;
	for (let char of text) {
		const code = char.codePointAt(0);
		if (code >= 0xd800 && code <= 0xdfff) char = "\ufffd";
		const size = new TextEncoder().encode(char).length;
		if (bytes + size > 500) break;
		result += char;
		bytes += size;
	}
	return result;
}

export function createLyricsFeed(fetchLyrics) {
	const cache = new Map(), pending = new Map();
	let current = null;
	return function publish(send, snapshot) {
		const id = /^spotify:track:([A-Za-z0-9]+)$/.exec(snapshot?.current_uri ?? "")?.[1];
		if (!id) { current = null; return; }
		if (!current || current.send !== send || current.session !== snapshot.session ||
			current.occurrence !== snapshot.current_id || current.id !== id) {
			current = { send, session: snapshot.session, occurrence: snapshot.current_id, id, sent: false };
		}
		const selection = current;
		const deliver = (sheet) => {
			if (current !== selection || selection.sent) return;
			selection.sent = true;
			send({ op: "lyrics", session: selection.session, track_id: selection.occurrence, ...sheet });
		};
		if (cache.has(id)) { deliver(cache.get(id)); return; }
		if (!pending.has(id)) {
			if (pending.size >= 8) return;
			const request = Promise.resolve().then(() => fetchLyrics(lyricsPath(id)))
				.then(parseLyrics).catch(() => ({ synced: false, lines: [] }))
				.then((sheet) => {
					cache.set(id, sheet);
					if (cache.size > 8) cache.delete(cache.keys().next().value);
					pending.delete(id);
					return sheet;
				});
			pending.set(id, request);
		}
		// Each selection subscribes once; an old selection cannot overwrite a new one.
		if (!selection.waiting) {
			selection.waiting = true;
			void pending.get(id).then(deliver);
		}
	};
}
