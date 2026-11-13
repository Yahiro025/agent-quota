import {expect,test} from "bun:test";
import {readYouTubeQueue,selectYouTubeOccurrence} from "./queue.js";
const ids=["AAAAAAAAAAA","BBBBBBBBBBB","AAAAAAAAAAA","DDDDDDDDDDD"];
function player(){let list=[...ids],index=0,state=2;const calls=[];
  return {getPlaylist:()=>list,getPlaylistId:()=>"PL-example",getPlaylistIndex:()=>index,
    getPlayerState:()=>state,getVideoData:()=>({video_id:list[index]}),
    playVideoAt(i){calls.push(i);index=i;state=1;},calls,
    setList(v){list=v;},setIndex(v){index=v;},setState(v){state=v;}};
}
test("reports only actual upcoming playlist occurrences with stable identities",()=>{
  const p=player();const a=readYouTubeQueue(p,"tab-generation");
  expect(a.entries.map(t=>[t.index,t.videoId])).toEqual([[1,"BBBBBBBBBBB"],[2,"AAAAAAAAAAA"],[3,"DDDDDDDDDDD"]]);
  expect(a.entries[1].entryId).toBe("2:AAAAAAAAAAA");
  expect(readYouTubeQueue(p,"tab-generation").revision).toBe(a.revision);
});
test("selects a duplicate by position and confirms actual playback without replacing the playlist",async()=>{
  const p=player(),q=readYouTubeQueue(p,"session");
  const target=q.entries[1];
  expect((await selectYouTubeOccurrence(p,"session",{session:"session",revision:q.revision,...target})).status).toBe("confirmed");
  expect(p.calls).toEqual([2]);expect(p.getPlaylist()).toEqual(ids);
  expect(readYouTubeQueue(p,"session").entries.map(t=>t.index)).toEqual([3]);
});
test("rejects a stale queue, old session, or removed row before calling the player",async()=>{
  for(const change of ["revision","session","removed"]){
    const p=player(),q=readYouTubeQueue(p,"s"),r={session:"s",revision:q.revision,...q.entries[1]};
    if(change==="revision")p.setList([ids[0],ids[3],ids[2],ids[1]]);
    if(change==="session")r.session="former";
    if(change==="removed")p.setList(ids.slice(0,2));
    expect((await selectYouTubeOccurrence(p,"s",r)).status).toBe("stale");expect(p.calls).toEqual([]);
  }
});
test("an autoplay-only page or unknown current index does not invent a queue",()=>{
  const p=player();p.setList(null);expect(readYouTubeQueue(p,"s").available).toBe(false);
  p.setList(ids);p.setIndex(-1);expect(readYouTubeQueue(p,"s").available).toBe(false);
});
test("retains at most 64 upcoming rows and marks the remaining count",()=>{
  const p=player();p.setList(Array(1000).fill("AAAAAAAAAAA"));
  const q=readYouTubeQueue(p,"s");expect(q.entries.length).toBe(64);expect(q.totalUpcoming).toBe(999);expect(q.truncated).toBe(true);
});
test("dispatch without observed playback times out once and never auto-retries",async()=>{
  const p=player(),q=readYouTubeQueue(p,"s");let now=0;
  p.playVideoAt=i=>{p.calls.push(i);p.setState(3);};
  const r=await selectYouTubeOccurrence(p,"s",{session:"s",revision:q.revision,...q.entries[1]},
    {now:()=>now,wait:async ms=>{now+=ms;}});
  expect(r.status).toBe("uncertain");expect(now).toBe(5000);expect(p.calls).toEqual([2]);
});

test("concurrent clicks cannot launch two selections in one tab",async()=>{
  const p=player(),q=readYouTubeQueue(p,"s");let release;
  const hold=new Promise(resolve=>{release=resolve;});
  p.playVideoAt=i=>{p.calls.push(i);p.setState(3);};
  const first=selectYouTubeOccurrence(p,"s",{session:"s",revision:q.revision,...q.entries[1]},
    {now:()=>0,wait:()=>hold});
  expect((await selectYouTubeOccurrence(p,"s",{session:"s",revision:q.revision,...q.entries[2]})).status).toBe("busy");
  p.setIndex(2);p.setState(1);release();
  expect((await first).status).toBe("confirmed");expect(p.calls).toEqual([2]);
});

test("queue edits beyond the displayed window still invalidate a selection",async()=>{
  const p=player(),ids=Array(100).fill("AAAAAAAAAAA");p.setList(ids);
  const q=readYouTubeQueue(p,"s");p.setList([...ids.slice(0,99),"BBBBBBBBBBB"]);
  expect((await selectYouTubeOccurrence(p,"s",{session:"s",revision:q.revision,...q.entries[1]})).status).toBe("stale");
  expect(p.calls).toEqual([]);
});

test("independent tabs cannot share revisions or selection state",async()=>{
  const a=player(),b=player(),q=readYouTubeQueue(a,"a");
  expect((await selectYouTubeOccurrence(b,"b",{session:"a",revision:q.revision,...q.entries[1]})).status).toBe("stale");
  expect(b.calls).toEqual([]);
});

test("current-index changes invalidate a selection even if video IDs have not changed",async()=>{
  const p=player(),q=readYouTubeQueue(p,"s");p.setIndex(1);
  expect((await selectYouTubeOccurrence(p,"s",{session:"s",revision:q.revision,...q.entries[1]})).status).toBe("stale");
  expect(p.calls).toEqual([]);
});

test("malformed or oversized source lists are explicitly unavailable",()=>{
  for(const ids of [Array(5001).fill("AAAAAAAAAAA"),["not-a-video-id"],[]]){
    const p=player();p.setList(ids);expect(readYouTubeQueue(p,"s").available).toBe(false);
  }
});
test("an in-flight list edit cannot confirm a different duplicate at the same index",async()=>{
 const p=player(),q=readYouTubeQueue(p,"s"),play=p.playVideoAt.bind(p);
 p.playVideoAt=i=>{p.setList([ids[0],ids[0],ids[0],ids[3]]);play(i);};
 const result=await selectYouTubeOccurrence(p,"s",{session:"s",revision:q.revision,...q.entries[1]});
 expect(result.status).toBe("uncertain");expect(p.calls).toEqual([2]);
});
test("playback first observed after the five-second deadline is uncertain",async()=>{
 const p=player(),q=readYouTubeQueue(p,"s");let clock=0;
 p.playVideoAt=i=>{p.calls.push(i);p.setState(3);};
 const result=await selectYouTubeOccurrence(p,"s",{session:"s",revision:q.revision,...q.entries[1]},
  {now:()=>clock,wait:async()=>{clock=5001;p.setIndex(2);p.setState(1);}});
 expect(result.status).toBe("uncertain");
});
