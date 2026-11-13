import { readYouTubeQueue } from "../youtube/queue.js";
export function text(value, limit = 512) {
	if (typeof value !== "string") return "";
	let result = value.slice(0, limit);
	const last = result.charCodeAt(result.length - 1);
	if (last >= 0xd800 && last <= 0xdbff) result = result.slice(0, -1);
	return result;
}
const milliseconds = (n) =>
	Number.isFinite(n) ? Math.max(0, Math.min(31_536_000_000, Math.round(n))) : 0;
const duration = (n) =>
	n > 0 ? `${Math.floor(n / 60000)}:${String(Math.floor(n / 1000) % 60).padStart(2, "0")}` : null;

const SPOTIFY_REASON = "Queue unavailable for this Spotify context.";
// Autoplay suggestions are not queue occurrences: they have no selectable
// occurrence in the internal queue. Unknown providers are re-read once and
// then fail closed instead of being dropped silently.
const NON_OCCURRENCE_PROVIDERS = new Set(["autoplay", "recommendation", "recommendations"]);

function providerOf(track) {
	for (const value of [track?.provider, track?.contextTrack?.provider, track?.track?.provider]) {
		if (typeof value === "string" && value) return value.toLowerCase();
	}
	return "";
}

function spotifyFailure(base, total) {
	return {
		...base,
		queue_available: false,
		reason: SPOTIFY_REASON,
		entries: [],
		total_upcoming: Number.isSafeInteger(total) && total <= 4294967295 ? total : null
	};
}

function spotifyAttempt(spicetify, session, allowRetry) {
	const player = spicetify?.Player,
		api = player?.origin,
		queue = api?.getQueue?.(),
		raw = api?._queue?.getInternalQueue?.();
	const current = queue?.current,
		item = player?.data?.item;
	const title = text(current?.metadata?.title ?? current?.name ?? item?.metadata?.title);
	if (!current?.uid || !current?.uri || !title) return null;
	const revision =
		typeof raw?.queueRevision === "bigint" ||
		typeof raw?.queueRevision === "string" ||
		Number.isSafeInteger(raw?.queueRevision)
			? String(raw.queueRevision)
			: "unavailable";
	const total = (queue?.queued?.length ?? 0) + (queue?.nextUp?.length ?? 0);
	const durationMs = milliseconds(player.getDuration?.() ?? 0);
	const base = {
		session,
		revision,
		current_id: text(current.uid, 256),
		current_uri: text(current.uri, 256),
		title,
		artist: text(current.metadata?.artist_name ?? item?.metadata?.artist_name),
		is_playing: !!player.isPlaying?.(),
		position_ms: milliseconds(player.getProgress?.() ?? 0),
		duration_ms: durationMs,
		seek_enabled:
			durationMs > 0 &&
			/^spotify:(track|episode):/.test(current.uri) &&
			typeof api.seekTo === "function",
		observed_ms: Date.now()
	};
	if (
		revision === "unavailable" ||
		!Array.isArray(queue?.queued) ||
		!Array.isArray(queue?.nextUp) ||
		!Array.isArray(raw?.nextTracks)
	) {
		return spotifyFailure(base, total);
	}
	const occurrences = new Set();
	for (const entry of raw.nextTracks) {
		const track = entry?.contextTrack;
		if (
			typeof track?.uid === "string" &&
			track.uid &&
			track.uid.length <= 256 &&
			typeof track?.uri === "string" &&
			track.uri.length <= 256 &&
			/^spotify:(track|episode):/.test(track.uri)
		) {
			occurrences.add(`${track.uid}\u0000${track.uri}`);
		}
	}
	const entries = [],
		seen = new Set();
	let omitted = 0;
	for (let i = 0; i < Math.min(total, 64); i++) {
		const track =
			i < queue.queued.length ? queue.queued[i] : queue.nextUp[i - queue.queued.length];
		if (
			typeof track?.uid !== "string" ||
			!track.uid ||
			track.uid.length > 256 ||
			typeof track?.uri !== "string" ||
			track.uri.length > 256 ||
			!/^spotify:(track|episode):/.test(track.uri)
		) {
			// A malformed row cannot be selected as an occurrence. Drop it
			// instead of discarding an otherwise usable queue.
			omitted++;
			continue;
		}
		if (seen.has(track.uid)) {
			// Duplicate occurrence identities make the snapshot ambiguous.
			return spotifyFailure(base, total);
		}
		const key = `${track.uid}\u0000${track.uri}`;
		if (occurrences.has(key)) {
			seen.add(track.uid);
			entries.push({
				id: text(track.uid, 256),
				uid: text(track.uid, 256),
				uri: text(track.uri, 256),
				title: text(track.metadata?.title ?? track.name) || "Untitled track",
				artist: text(track.metadata?.artist_name),
				duration: duration(milliseconds(track.duration?.milliseconds ?? track.duration))
			});
			continue;
		}
		if (NON_OCCURRENCE_PROVIDERS.has(providerOf(track))) {
			omitted++;
			continue;
		}
		// A valid-looking row with no internal occurrence may be a real context
		// row that has not loaded yet. Re-read once; if it is still missing,
		// fail closed instead of silently hiding an upcoming track.
		if (allowRetry) return spotifyAttempt(spicetify, session, false);
		return spotifyFailure(base, total);
	}
	if (!entries.length && total > 0) {
		// Every visible row was a non-occurrence, for example autoplay
		// suggestions. That is not an empty real queue.
		return spotifyFailure(base, total);
	}
	return {
		...base,
		queue_available: true,
		reason: null,
		entries,
		total_upcoming: Number.isSafeInteger(total) && total <= 4294967295 ? total - omitted : null
	};
}

export function spotifySnapshot(spicetify, session) {
	try {
		return spotifyAttempt(spicetify, session, true);
	} catch {
		return null;
	}
}

export function youTubeSnapshot(player, session, describe) {
	try {
		const data = player?.getVideoData?.(),
			title = text(data?.title);
		if (!title || !data.video_id) return null;
		const queue = readYouTubeQueue(player, session, describe),
			durationMs = milliseconds((player.getDuration?.() ?? 0) * 1000);
		return {
			session,
			revision: queue.revision ?? "unavailable",
			current_id: `${player.getPlaylistIndex?.() ?? -1}:${data.video_id}`,
			title,
			artist: text(data.author),
			is_playing: player.getPlayerState?.() === 1,
			position_ms: milliseconds((player.getCurrentTime?.() ?? 0) * 1000),
			duration_ms: durationMs,
			seek_enabled: durationMs > 0 && typeof player.seekTo === "function",
			queue_available: queue.available,
			reason: queue.available ? null : "Queue unavailable for this YouTube page.",
			total_upcoming: queue.totalUpcoming ?? null,
			entries: (queue.entries ?? []).map((e) => ({
				id: e.entryId,
				title: text(e.title) || `Video ${e.index + 1}`,
				artist: text(e.artist),
				duration: e.duration || null,
				index: e.index,
				video_id: e.videoId
			})),
			observed_ms: Date.now()
		};
	} catch {
		return null;
	}
}
