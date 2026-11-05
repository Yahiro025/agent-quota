import { youTubeSnapshot } from "./providers.js";
import { selectYouTubeOccurrence } from "../youtube/queue.js";
if (!globalThis.__bloomMediaPage) {
	let player = null,
		session = crypto.randomUUID();
	function target() {
		const next = document.getElementById("movie_player");
		if (next !== player) {
			player = next;
			session = crypto.randomUUID();
		}
		return { player, session };
	}
	function describe(index, id) {
		for (const row of document.querySelectorAll("ytd-playlist-panel-video-renderer")) {
			const d = row.data,
				e = d?.navigationEndpoint?.watchEndpoint;
			if (e?.index === index && d?.videoId === id)
				return {
					title: d.title?.simpleText ?? d.title?.runs?.map((r) => r.text).join(""),
					artist: d.shortBylineText?.runs?.map((r) => r.text).join(""),
					duration: d.lengthText?.simpleText
				};
		}
		return null;
	}
	function read() {
		target();
		return youTubeSnapshot(player, session, describe);
	}
	globalThis.__bloomMediaPage = {
		read,
		perform: async (inner, request) => {
			target();
			if (session !== inner) return { status: "stale" };
			const capturedPlayer = player,
				capturedSession = session;
			let status = "dispatched";
			if (request.op === "select")
				status = (
					await selectYouTubeOccurrence(player, session, {
						session,
						revision: request.revision,
						entryId: request.entry?.id,
						index: request.entry?.index,
						videoId: request.entry?.video_id
					})
				).status;
			else {
				const method = {
					playPause: player.getPlayerState() === 1 ? "pauseVideo" : "playVideo",
					next: "nextVideo",
					previous: "previousVideo",
					seek: "seekTo"
				}[request.op];
				if (!method || typeof player?.[method] !== "function") return { status: "unavailable" };
				if (request.op === "seek") {
					const snapshot = read();
					if (
						!snapshot?.seek_enabled ||
						!Number.isSafeInteger(request.position_ms) ||
						request.position_ms < 0 ||
						request.position_ms > snapshot.duration_ms
					)
						return { status: "unavailable" };
					player.seekTo(request.position_ms / 1000, true);
				} else player[method]();
			}
			const snapshot = read();
			if (player !== capturedPlayer || session !== capturedSession) return { status: "stale" };
			return { status, snapshot };
		}
	};
}
