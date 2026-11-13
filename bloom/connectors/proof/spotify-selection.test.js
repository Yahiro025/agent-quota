import { expect, test } from "bun:test";
import { selectOccurrence } from "./spotify-selection.js";

function runtime() {
  const tracks = ["a", "b", "a", "d"].map((id, i) => ({ contextTrack: {uri: `spotify:track:${id}`, uid: `occ-${i}`} }));
  let current = {uri: "spotify:track:old", uid: "old"};
  const calls = [];
  const api = { _state: {isPaused:false}, _queue: {getInternalQueue: () => ({queueRevision: 123n, nextTracks: tracks})},
    getQueue: () => ({current}),
    async skipToNext(target) { calls.push(target); current = {...target}; },
  };
  return {api, calls, setCurrent(value) {current = value;}};
}
const request = {revision: "123", uri: "spotify:track:a", uid: "occ-2"};
test("an occurrence first observed after the confirmation deadline remains uncertain",async()=>{
 const {api,setCurrent}=runtime();let clock=0;api.skipToNext=()=>{};
 const result=await selectOccurrence(api,request,{now:()=>clock,wait:async()=>{clock=5001;setCurrent({uri:request.uri,uid:request.uid});}});
 expect(result.status).toBe("uncertain");
});

test("selects the second occurrence with uid rather than URI-only playback", async () => {
  const {api, calls} = runtime();
  expect(await selectOccurrence(api, request)).toEqual({status: "confirmed", uri: "spotify:track:a", uid: "occ-2"});
  expect(calls).toEqual([{uri: "spotify:track:a", uid: "occ-2"}]);
});

test("stale revision or removed occurrence never falls through to a plain skip", async () => {
  for (const change of [{revision: "122"}, {uid: "removed"}, {uid: ""}, {uri: "spotify:track:other"}]) {
    const {api, calls} = runtime();
    expect((await selectOccurrence(api, {...request,...change})).status).toBe("stale");
    expect(calls).toEqual([]);
  }
});

test("repeated copies of the selected occurrence still target that uid",async()=>{
  const {api,calls}=runtime();const raw=api._queue.getInternalQueue();
  raw.nextTracks.push(raw.nextTracks[2]);
  api._queue.getInternalQueue=()=>raw;
  expect(await selectOccurrence(api,request)).toEqual({status:"confirmed", uri:request.uri, uid:request.uid});
  expect(calls).toEqual([{uri:request.uri, uid:request.uid}]);
});

test("dispatch acknowledgment without observed occurrence becomes uncertain", async () => {
  const {api} = runtime(); let calls = 0; let now = 0;
  api.skipToNext = async () => {calls++;};
  const options = {now: () => now, wait: async ms => { now += ms; }};
  expect((await selectOccurrence(api, request, options)).status).toBe("uncertain");
  expect(now).toBe(5000);
  expect(calls).toBe(1);
});

test("never starts a second selection while a previous command is outstanding", async () => {
  const {api, calls} = runtime(); let finish;
  const hold = new Promise(resolve => {finish=resolve;});
  api.skipToNext = async t => {calls.push(t); await hold;};
  let clock=0;
  const first = selectOccurrence(api, request, {now:()=>clock, wait:async ms=>{clock+=ms;}});
  expect((await selectOccurrence(api, request)).status).toBe("busy");
  finish();
  expect((await first).status).toBe("uncertain");
  expect(calls.length).toBe(1);
});

test("a rejected source command is uncertain and cannot become automatic retry", async () => {
  const {api} = runtime(); let calls=0;
  api.skipToNext = async () => {calls++; throw new Error("account-private-error");};
  const result = await selectOccurrence(api, request);
  expect(result).toEqual({status:"uncertain"});
  expect(calls).toBe(1);
});

test("a selected but paused occurrence is not reported as playing", async () => {
  const {api} = runtime(); api._state.isPaused = true; let clock=0;
  expect((await selectOccurrence(api, request, {now:()=>clock, wait:async ms=>{clock+=ms;}})).status).toBe("uncertain");
});

test("a hung dispatch times out and continues excluding a second command", async () => {
  const {api} = runtime(); let clock=0; let finish;
  api.skipToNext = () => new Promise(resolve => {finish=resolve;});
  expect((await selectOccurrence(api, request, {now:()=>clock, wait:async ms=>{clock+=ms;}})).status).toBe("uncertain");
  expect((await selectOccurrence(api, request)).status).toBe("busy");
  finish();
});

test("an observation exception cannot unlock a still-outstanding source command", async () => {
  const {api} = runtime(); let finish;
  api.skipToNext = () => new Promise(resolve => {finish=resolve;});
  api.getQueue = () => {throw new Error("private-state");};
  expect((await selectOccurrence(api, request)).status).toBe("uncertain");
  expect((await selectOccurrence(api, request)).status).toBe("busy");
  finish();
});
