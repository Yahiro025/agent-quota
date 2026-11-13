import {expect,test} from "bun:test";
import {SpotifyProofFixture} from "./spotify-fixture.js";

test("refuses fixture setup when a user queue already exists",async()=>{
  let added=false;
  const s={Player:{origin:{getQueue:()=>({queued:[{uid:"user"}],nextUp:[],current:{uri:"spotify:track:a",uid:"a"}}),
    addToQueue:()=>{added=true;}}}};
  expect((await new SpotifyProofFixture(s).setup()).status).toBe("fixture-refused");
  expect(added).toBe(false);
});

test("uses temporary occurrences without changing context or playback modes and restores paused position",async()=>{
  let current={uri:"spotify:track:a",uid:"original"},queued=[];const calls=[];
  const api={getQueue:()=>({current,queued,nextUp:[]}),
    addToQueue:async tracks=>{queued=tracks.map((t,i)=>({...t,uid:`new-${i}`}));},
    removeFromQueue:async tracks=>{calls.push(["remove",tracks.map(t=>t.uid)]);queued=[];},
    skipTo:async track=>{calls.push(["restore",track.uid]);current=track;},
    seekTo:async ms=>{calls.push(["seek",ms]);},pause:async()=>{calls.push(["pause"]);},
  };
  const s={Player:{origin:api,getProgress:()=>12345,isPlaying:()=>false,getShuffle:()=>false,getRepeat:()=>2}};
  const fixture=new SpotifyProofFixture(s);
  const result=await fixture.setup();
  expect(result.status).toBe("fixture-ready");
  expect(result.occurrences.map(t=>t.uid)).toEqual(["new-0","new-1","new-2","new-3"]);
  current=queued[2]; queued=queued.slice(3);
  expect((await fixture.restore()).status).toBe("restored");
  expect(calls).toEqual([["restore","original"],["remove",["new-3"]],["seek",12345],["pause"]]);
  expect(queued).toEqual([]);
});
