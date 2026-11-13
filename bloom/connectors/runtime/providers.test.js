import { expect, test } from "bun:test";
import { spotifySnapshot, youTubeSnapshot } from "./providers.js";
function spotify() {
	const current = {
		uri: "spotify:track:current",
		uid: "current",
		metadata: { title: "Current", artist_name: "Artist" }
	};
	const entries = [
		{ uri: "spotify:track:a", uid: "a1", metadata: { title: "A" } },
		{ uri: "spotify:track:a", uid: "a2", metadata: { title: "A" } }
	];
	const raw = {
		queueRevision: 9223372036854775806n,
		nextTracks: entries.map((contextTrack) => ({ contextTrack }))
	};
	return {
		token: "private-do-not-export",
		Player: {
			origin: {
				getQueue: () => ({ current, queued: entries, nextUp: [] }),
				_queue: { getInternalQueue: () => raw },
				seekTo() {}
			},
			data: { item: current },
			getProgress: () => 1000,
			getDuration: () => 200000,
			isPlaying: () => true
		}
	};
}
test("Spotify wire data preserves occurrence IDs and int64 revisions without account data", () => {
	const s = spotify(),
		q = spotifySnapshot(s, "instance");
	expect(q.revision).toBe("9223372036854775806");
	expect(q.entries.map((e) => [e.id, e.uid, e.uri])).toEqual([
		["a1", "a1", "spotify:track:a"],
		["a2", "a2", "spotify:track:a"]
	]);
	expect(q.current_id).toBe("current");
	expect(q.current_uri).toBe("spotify:track:current");
	expect(q.queue_available).toBe(true);
	expect(JSON.stringify(q)).not.toContain("private-do-not-export");
});
test("repeated copies of one occurrence stay listed", () => {
	const s = spotify();
	s.Player.origin._queue
		.getInternalQueue()
		.nextTracks.push({ contextTrack: { uri: "spotify:track:a", uid: "a2" } });
	const q = spotifySnapshot(s, "instance");
	expect(q.queue_available).toBe(true);
	expect(q.entries.map((e) => e.uid)).toEqual(["a1", "a2"]);
	expect(q.reason).toBeNull();
});
test("a full album repeated in the internal queue stays listed once", () => {
	const s = spotify();
	const album = Array.from({ length: 16 }, (_, i) => ({
		uri: `spotify:track:t${i}`,
		uid: `u${i}`,
		metadata: { title: `T${i}` }
	}));
	const copies = Array.from({ length: 5 }, () => album.map((contextTrack) => ({ contextTrack })));
	s.Player.origin.getQueue = () => ({ current: album[0], queued: [], nextUp: album.slice(1) });
	s.Player.origin._queue.getInternalQueue = () => ({ queueRevision: 1n, nextTracks: copies.flat() });
	const q = spotifySnapshot(s, "instance");
	expect(q.queue_available).toBe(true);
	expect(q.entries.map((e) => e.uid)).toEqual(album.slice(1).map((track) => track.uid));
	expect(q.reason).toBeNull();
});
test("a visible row missing from the internal queue disables the queue", () => {
	const s = spotify();
	s.Player.origin._queue.getInternalQueue().nextTracks.pop();
	const q = spotifySnapshot(s, "instance");
	expect(q.queue_available).toBe(false);
	expect(q.entries).toEqual([]);
});
test("opaque source identities are rejected rather than shortened into a different occurrence", () => {
	const s = spotify(),
		raw = s.Player.origin._queue.getInternalQueue();
	raw.nextTracks[0].contextTrack.uid = "x".repeat(257);
	const q = spotifySnapshot(s, "instance");
	expect(q.queue_available).toBe(true);
	expect(q.entries.map((e) => e.uid)).toEqual(["a2"]);
});
test("metadata clipping never creates an unpaired UTF16 surrogate in the Rust wire", () => {
	const s = spotify();
	s.Player.data.item.metadata.title = "x".repeat(511) + "🎵";
	expect(spotifySnapshot(s, "instance").title).toBe("x".repeat(511));
});
test("YouTube wire IDs retain absolute positions and do not expose recommendations", () => {
	const ids = ["AAAAAAAAAAA", "BBBBBBBBBBB", "AAAAAAAAAAA"];
	const p = {
		getPlaylist: () => ids,
		getPlaylistId: () => "PL-test",
		getPlaylistIndex: () => 0,
		getVideoData: () => ({ video_id: ids[0], title: "Current", author: "Artist" }),
		getCurrentTime: () => 1.2,
		getDuration: () => 99,
		getPlayerState: () => 1,
		seekTo() {}
	};
	const q = youTubeSnapshot(p, "page");
	expect(q.current_id).toBe("0:AAAAAAAAAAA");
	expect(q.entries.map((e) => e.id)).toEqual(["1:BBBBBBBBBBB", "2:AAAAAAAAAAA"]);
	p.getPlaylist = () => null;
	expect(youTubeSnapshot(p, "page").queue_available).toBe(false);
});

function filteredSpotify({
	queued = [],
	nextUp = [],
	nextTracks = [],
	current = { uri: "spotify:track:current", uid: "current", metadata: { title: "Current" } }
} = {}) {
	return {
		Player: {
			origin: {
				getQueue: () => ({ current, queued, nextUp }),
				_queue: { getInternalQueue: () => ({ queueRevision: 7n, nextTracks }) },
				seekTo() {}
			},
			data: { item: current },
			getProgress: () => 0,
			getDuration: () => 200000,
			isPlaying: () => true
		}
	};
}

test("autoplay suggestions are omitted without discarding the real queue", () => {
	const a = { uri: "spotify:track:a", uid: "a1", metadata: { title: "A" } };
	const b = { uri: "spotify:track:b", uid: "b1", metadata: { title: "B" } };
	const suggestion = {
		uri: "spotify:track:r",
		uid: "r1",
		metadata: { title: "R" },
		provider: "autoplay"
	};
	const q = spotifySnapshot(
		filteredSpotify({
			queued: [a, b, suggestion],
			nextTracks: [a, b].map((contextTrack) => ({ contextTrack }))
		}),
		"instance"
	);
	expect(q.queue_available).toBe(true);
	expect(q.entries.map((e) => e.uid)).toEqual(["a1", "b1"]);
	expect(q.total_upcoming).toBe(2);
	expect(q.reason).toBeNull();
});

test("a queue made only of autoplay suggestions stays unavailable", () => {
	const suggestion = {
		uri: "spotify:track:r",
		uid: "r1",
		metadata: { title: "R" },
		provider: "autoplay"
	};
	const q = spotifySnapshot(filteredSpotify({ queued: [suggestion], nextTracks: [] }), "instance");
	expect(q.queue_available).toBe(false);
	expect(q.entries).toEqual([]);
	expect(q.reason).toContain("Queue unavailable");
});

test("an unknown unmatched row is re-read once and can recover", () => {
	const current = {
		uri: "spotify:track:current",
		uid: "current",
		metadata: { title: "Current" }
	};
	const a = { uri: "spotify:track:a", uid: "a1", metadata: { title: "A" } };
	const unknown = { uri: "spotify:track:u", uid: "u1", metadata: { title: "U" } };
	const s = filteredSpotify({
		queued: [a, unknown],
		nextTracks: [a].map((contextTrack) => ({ contextTrack })),
		current
	});
	let reads = 0;
	s.Player.origin.getQueue = () => {
		reads++;
		return reads === 1
			? { current, queued: [a, unknown], nextUp: [] }
			: { current, queued: [a], nextUp: [] };
	};
	const q = spotifySnapshot(s, "instance");
	expect(reads).toBe(2);
	expect(q.queue_available).toBe(true);
	expect(q.entries.map((e) => e.uid)).toEqual(["a1"]);
});

test("an unmatched real-looking row fails closed after one re-read", () => {
	const current = {
		uri: "spotify:track:current",
		uid: "current",
		metadata: { title: "Current" }
	};
	const a = { uri: "spotify:track:a", uid: "a1", metadata: { title: "A" } };
	const unknown = { uri: "spotify:track:u", uid: "u1", metadata: { title: "U" } };
	const s = filteredSpotify({
		queued: [a, unknown],
		nextTracks: [a].map((contextTrack) => ({ contextTrack })),
		current
	});
	let reads = 0;
	s.Player.origin.getQueue = () => {
		reads++;
		return { current, queued: [a, unknown], nextUp: [] };
	};
	const q = spotifySnapshot(s, "instance");
	expect(reads).toBe(2);
	expect(q.queue_available).toBe(false);
	expect(q.entries).toEqual([]);
});

test("a malformed row is omitted rather than disabling the queue", () => {
	const a = { uri: "spotify:track:a", uid: "a1", metadata: { title: "A" } };
	const bad = { uri: "spotify:track:bad", metadata: { title: "Bad" } };
	const q = spotifySnapshot(
		filteredSpotify({
			queued: [bad, a],
			nextTracks: [a].map((contextTrack) => ({ contextTrack }))
		}),
		"instance"
	);
	expect(q.queue_available).toBe(true);
	expect(q.entries.map((e) => e.uid)).toEqual(["a1"]);
	expect(q.total_upcoming).toBe(1);
});

test("an empty real queue stays available", () => {
	const q = spotifySnapshot(filteredSpotify({ queued: [], nextTracks: [] }), "instance");
	expect(q.queue_available).toBe(true);
	expect(q.entries).toEqual([]);
	expect(q.total_upcoming).toBe(0);
});

test("duplicate occurrence identities still fail closed", () => {
	const a = { uri: "spotify:track:a", uid: "a1", metadata: { title: "A" } };
	const b = { uri: "spotify:track:b", uid: "a1", metadata: { title: "B" } };
	const q = spotifySnapshot(
		filteredSpotify({
			queued: [a, b],
			nextTracks: [a, b].map((contextTrack) => ({ contextTrack }))
		}),
		"instance"
	);
	expect(q.queue_available).toBe(false);
});
