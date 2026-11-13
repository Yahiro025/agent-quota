// Diagnostic only: allowlist fields instead of serializing Spotify's runtime objects.
// The queue variants stay separate until a live proof establishes their meaning.
const text = value => typeof value === "string" ? value.slice(0, 512) : null;
const revision = value => typeof value === "bigint" || typeof value === "string" || Number.isSafeInteger(value)
  ? String(value).slice(0,512) : null;
const entries = items => ({total:items.length, truncated:items.length>64, entries:items.slice(0,64).map(track)});

function track(value) {
  const item = value?.contextTrack ?? value?.track ?? value;
  return {
    uri: text(item?.uri),
    uid: text(item?.uid),
    title: text(item?.metadata?.title),
    artist: text(item?.metadata?.artist_name),
    provider: text(value?.provider),
  };
}

function methods(value) {
  if (!value) return [];
  const result = new Set();
  // Inspect function descriptors without evaluating arbitrary source getters.
  for (const object of [value, Object.getPrototypeOf(value)]) {
    if (!object || object === Object.prototype) continue;
    for (const key of Object.getOwnPropertyNames(object)) {
      if (result.size >= 40) break;
      if (key !== "constructor" && typeof Object.getOwnPropertyDescriptor(object, key)?.value === "function") {
        result.add(key.slice(0, 80));
      }
    }
  }
  return [...result];
}

export function captureSpotify(spicetify) {
  if (!spicetify?.Player) return { available: false, reason: "Spicetify not ready" };
  try {
    const queue = spicetify.Queue;
    if (!queue) return { available: false, reason: "Queue not ready" };
    const fields = {};
    for (const name of ["nextTracks", "prevTracks", "queued", "nextUp", "previous", "tracks"]) {
      const items = queue[name];
      if (Array.isArray(items)) {
        fields[name] = entries(items);
      }
    }
    const visible = spicetify.Player.origin?.getQueue?.();
    return {
      available: true,
      current: track(spicetify.Player.data?.item),
      playing: typeof spicetify.Player.isPlaying === "function" ? Boolean(spicetify.Player.isPlaying()) : null,
      queue: { keys: Object.keys(queue).slice(0, 40).map(key => key.slice(0, 80)),
        revision: revision(queue.queueRevision), current: track(queue.track), fields },
      displayed: visible ? {current:track(visible.current),
        queued:entries(Array.isArray(visible.queued)?visible.queued:[]),
        nextUp:entries(Array.isArray(visible.nextUp)?visible.nextUp:[])} : null,
      capabilities: { player: methods(spicetify.Player.origin),
        queue: methods(spicetify.Player.origin?._queue), platformPlayer: methods(spicetify.Platform?.PlayerAPI) },
    };
  } catch {
    // Runtime exceptions can include private source state; never transmit their messages.
    return { available: false, reason: "Queue inspection unavailable" };
  }
}
