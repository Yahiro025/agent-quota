import { startConnector } from "./client.js";
import { spotifySnapshot } from "./providers.js";
import { selectOccurrence } from "../proof/spotify-selection.js";
const config = globalThis.__BLOOM_MEDIA_CONFIG__;
let api = null,
	session = crypto.randomUUID();
function current() {
	const next = globalThis.Spicetify?.Player?.origin;
	if (next !== api) {
		api = next;
		session = crypto.randomUUID();
	}
	return spotifySnapshot(globalThis.Spicetify, session);
}
globalThis.__bloomMediaConnection?.stop();
globalThis.__bloomMediaConnection = startConnector(config, {
	inventory: async () => {
		const snapshot = current();
		return { snapshots: snapshot ? [snapshot] : [], unknown_playing: false };
	},
	perform: async (request) => {
		// Tab rows are a Brave concept; Spotify has no tab to mute or reveal.
		if (["mute", "unmute", "show"].includes(request.op)) return { status: "unavailable" };
		if (current()?.session !== request.session) return { status: "stale" };
		let status = "dispatched";
		if (request.op === "select") {
			if (request.entry?.id !== request.entry?.uid) return { status: "stale" };
			status = (
				await selectOccurrence(api, {
					revision: request.revision,
					uri: request.entry?.uri,
					uid: request.entry?.uid
				})
			).status;
		} else if (request.op === "playPause") {
			const method = globalThis.Spicetify.Player.isPlaying() ? "pause" : "resume";
			if (typeof api[method] !== "function") return { status: "unavailable" };
			await api[method]();
		} else if (request.op === "next" || request.op === "previous") {
			const method = request.op === "next" ? "skipToNext" : "skipToPrevious";
			if (typeof api[method] !== "function") return { status: "unavailable" };
			await api[method]();
		} else if (request.op === "seek") {
			const snapshot = current();
			if (
				!snapshot?.seek_enabled ||
				!Number.isSafeInteger(request.position_ms) ||
				request.position_ms < 0 ||
				request.position_ms > snapshot.duration_ms
			)
				return { status: "unavailable" };
			await api.seekTo(request.position_ms);
		}
		return { status, snapshot: current() };
	}
});
