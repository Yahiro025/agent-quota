import { describe, expect, test } from "bun:test";
import { captureSpotify } from "./spotify-snapshot.js";

// This boundary must never serialize the source object (which may contain account data).
function source(queue) {
  return {
    Queue: queue,
    Player: { data: { item: { uri: "spotify:track:current", uid: "current", metadata: { title: "Current" } } },
      origin: { skipToNext() {}, _queue: {} }, isPlaying: () => true },
    Platform: { PlayerAPI: { skipToNext() {} } },
    accountToken: "must-not-serialize",
  };
}

describe("Spotify read-only proof snapshot", () => {
  test("reports absent or uninitialized APIs without crashing", () => {
    expect(captureSpotify(undefined)).toEqual({ available: false, reason: "Spicetify not ready" });
  });

  test("retains separate occurrences while excluding unrelated account data", () => {
    const result = captureSpotify(source({ queueRevision: "r1", nextTracks: [
      { uri: "spotify:track:a", uid: "first", metadata: { title: "A" }, token: "hidden" },
      { uri: "spotify:track:a", uid: "second", metadata: { title: "A" } },
    ] }));
    expect(result.available).toBe(true);
    expect(result.queue.fields.nextTracks.entries.map(t => [t.uri, t.uid])).toEqual([
      ["spotify:track:a", "first"], ["spotify:track:a", "second"],
    ]);
    expect(result.queue.revision).toBe("r1");
    expect(result.current.uri).toBe("spotify:track:current");
    expect(JSON.stringify(result)).not.toContain("must-not-serialize");
    expect(JSON.stringify(result)).not.toContain("hidden");
  });

  test("inspects older queue shapes without merging their different meanings", () => {
    const result = captureSpotify(source({ queued: [{ contextTrack: { uri: "spotify:track:q", uid: "q" } }],
      nextUp: [{ track: { uri: "spotify:track:p", uid: "p" } }] }));
    expect(result.queue.fields.queued.entries[0].uid).toBe("q");
    expect(result.queue.fields.nextUp.entries[0].uid).toBe("p");
    expect(result.capabilities.player).toContain("skipToNext");
  });

  test("bounds observation size without silently claiming a complete queue", () => {
    const result = captureSpotify(source({ nextTracks: Array.from({length: 200}, (_, i) => ({
      uri: `spotify:track:${i}`, uid: String(i), metadata: { title: "x".repeat(10000) },
    })) }));
    expect(result.queue.fields.nextTracks.entries.length).toBe(64);
    expect(result.queue.fields.nextTracks.total).toBe(200);
    expect(result.queue.fields.nextTracks.truncated).toBe(true);
    expect(result.queue.fields.nextTracks.entries[0].title.length).toBe(512);
    expect(JSON.stringify(result).length).toBeLessThan(50000);
  });

  test("never invokes unrelated getters or toJSON", () => {
    const q = { nextTracks: [], get accessToken() { throw new Error("private"); }, toJSON() {throw new Error("private");} };
    expect(captureSpotify(source(q)).available).toBe(true);
  });

  test("contains unavailable queue getters without exposing their error text", () => {
    const s = source({});
    Object.defineProperty(s, "Queue", { get() { throw new Error("secret-account-data"); } });
    expect(captureSpotify(s)).toEqual({ available: false, reason: "Queue inspection unavailable" });
  });

  test("preserves an int64 revision as text and reads the displayed queue separately", () => {
    const s=source({queueRevision: 9223372036854775806n, nextTracks: []});
    s.Player.origin.getQueue=()=>({current:{uri:"spotify:track:current",uid:"current"}, queued:[{uri:"spotify:track:a",uid:"a"}],nextUp:[]});
    const result=captureSpotify(s);
    expect(result.queue.revision).toBe("9223372036854775806");
    expect(result.displayed.queued.entries[0].uid).toBe("a");
  });
});
