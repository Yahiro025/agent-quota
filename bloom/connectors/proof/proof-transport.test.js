import {expect,test} from "bun:test";
import {createProofHandler} from "./proof-transport.js";
const key="0123456789abcdef0123456789abcdef";
const origin="https://xpui.app.spotify.com";
function req(path,options={}) {return new Request(`http://127.0.0.1/${path}`,{headers:{origin},...options});}

test("requires the private path and Spotify origin before invoking callbacks", async()=>{
  let called=0;
  const handle=createProofHandler({key,onObservation:()=>{called++;},takeCommand:()=>{called++;}});
  expect((await handle(req("wrong/observe",{method:"POST",body:"{}"}))).status).toBe(403);
  expect((await handle(req(`${key}/command`,{headers:{origin:"https://example.com"}}))).status).toBe(403);
  expect(called).toBe(0);
});

test("delivers observations and supports only known methods",async()=>{
  let result;
  const handle=createProofHandler({key,onObservation:value=>{result=value;},takeCommand:()=>null});
  expect((await handle(req(`${key}/observe`,{method:"POST",body:'{"available":false}'}))).status).toBe(204);
  expect(result).toEqual({available:false});
  expect((await handle(req(`${key}/observe`))).status).toBe(405);
  expect((await handle(req(`${key}/command`,{method:"OPTIONS"}))).headers.get("Access-Control-Allow-Origin")).toBe(origin);
});

test("rejects oversized chunked bodies before an observation is accepted",async()=>{
  let called=false;
  const handle=createProofHandler({key,onObservation:()=>{called=true;},takeCommand:()=>null});
  const body=new ReadableStream({start(c){c.enqueue(new Uint8Array(1048577));c.close();}});
  expect((await handle(req(`${key}/observe`,{method:"POST",body,duplex:"half"}))).status).toBe(413);
  expect(called).toBe(false);
});

test("returns an empty poll and rejects malformed observations",async()=>{
  const handle=createProofHandler({key,onObservation:()=>{},takeCommand:()=>null});
  expect(await (await handle(req(`${key}/command`))).json()).toBe(null);
  expect((await handle(req(`${key}/observe`,{method:"POST",body:"not json"}))).status).toBe(400);
});
