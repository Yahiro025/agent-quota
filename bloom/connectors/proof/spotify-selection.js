const pending = new WeakSet();

// Installed Spotify 1.2.53's skipToNext silently performs a plain skip when an
// occurrence cannot be found. Validate immediately before that call, with no await.
export async function selectOccurrence(api, request, options = {}) {
  if (!api?._queue?.getInternalQueue || !api?.getQueue || !api?.skipToNext) return {status:"unavailable"};
  if (pending.has(api)) return {status:"busy"};
  const now = options.now ?? (() => performance.now());
  const wait = options.wait ?? (ms => new Promise(resolve => setTimeout(resolve, ms)));
  let dispatchSettled = false, dispatchAttached = false, finished = false, failed = false;
  try {
    const queue = api._queue.getInternalQueue();
    let matches=0;
    for(const item of queue.nextTracks??[]) {
      if(item.contextTrack?.uid===request?.uid && item.contextTrack?.uri===request?.uri) matches++;
    }
    if (typeof request?.uid !== "string" || !request.uid || typeof request.uri !== "string" ||
        typeof request.revision !== "string" || String(queue.queueRevision) !== request.revision ||
        matches<1) {
      return {status:"stale"};
    }
    pending.add(api);
    const deadline = now() + 5000;
    const dispatch = api.skipToNext({uri:request.uri, uid:request.uid});
    Promise.resolve(dispatch).then(() => {}, () => {failed = true;}).finally(() => {
      dispatchSettled = true;
      if (finished) pending.delete(api);
    });
    dispatchAttached = true;
    do {
      if (now() >= deadline) return {status:"uncertain"};
      if (failed) return {status:"uncertain"};
      const current = api.getQueue()?.current;
      if (current?.uid === request.uid && current?.uri === request.uri && api._state?.isPaused === false) {
        return {status:"confirmed", uri:request.uri, uid:request.uid};
      }
      const remaining = deadline - now();
      if (remaining <= 0) return {status:"uncertain"};
      await wait(Math.min(100, remaining));
    } while (true);
  } catch {
    // Source error strings can contain private data. Dispatch might have succeeded.
    if (!dispatchAttached) dispatchSettled = true;
    return {status:"uncertain"};
  } finally {
    finished = true;
    if (dispatchSettled) pending.delete(api);
  }
}
