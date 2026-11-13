const history=new WeakMap(),pending=new WeakSet();
const unavailable=()=>({available:false,reason:"Queue unavailable for this player"});

// Revisions compare the complete supported list, not a hash or just the visible
// window: duplicate IDs and changes outside the first 64 rows must remain distinct.
export function readYouTubeQueue(player,session,describe=()=>null){
  try {
    if(!player || typeof session!=="string" || !session || session.length>128) return unavailable();
    const ids=player.getPlaylist?.(),index=player.getPlaylistIndex?.(),playlistId=player.getPlaylistId?.();
    if(!Array.isArray(ids) || !ids.length || ids.length>5000 || !Number.isInteger(index) || index<0 || index>=ids.length ||
       typeof playlistId!=="string" || !playlistId || playlistId.length>256 ||
       !ids.every(id=>typeof id==="string" && /^[\w-]{11}$/.test(id))) return unavailable();
    let previous=history.get(player);
    if(!previous || previous.session!==session || previous.playlistId!==playlistId || previous.index!==index ||
       previous.ids.length!==ids.length || ids.some((id,i)=>id!==previous.ids[i])) {
      previous={session,playlistId,index,ids:ids.slice(),revision:(previous?.revision??0)+1};
      history.set(player,previous);
    }
    const entries=[];
    for(let i=index+1;i<Math.min(ids.length,index+65);i++){
      const metadata=describe(i,ids[i])??{};
      const text=value=>typeof value==="string"?value.slice(0,512):null;
      entries.push({entryId:`${i}:${ids[i]}`,index:i,videoId:ids[i],title:text(metadata.title),
        artist:text(metadata.artist),duration:text(metadata.duration)});
    }
    return {available:true,session,revision:String(previous.revision),playlistId,currentIndex:index,
      entries,totalUpcoming:ids.length-index-1,truncated:ids.length-index-1>64};
  } catch {return unavailable();}
}

export async function selectYouTubeOccurrence(player,session,request,options={}){
  if(pending.has(player)) return {status:"busy"};
  const queue=readYouTubeQueue(player,session);
  if(!queue.available) return {status:"unavailable"};
  const target=queue.entries.find(item=>item.entryId===request?.entryId && item.index===request?.index && item.videoId===request?.videoId);
  if(request?.session!==session || request?.revision!==queue.revision || !target) return {status:"stale"};
  const expectedIds=history.get(player).ids;
  const now=options.now??(()=>performance.now()),wait=options.wait??(ms=>new Promise(resolve=>setTimeout(resolve,ms)));
  pending.add(player);
  try {
    const deadline=now()+5000;
    player.playVideoAt(target.index);
    do {
      const actualIds=player.getPlaylist?.();
      if(now()>=deadline || player.getPlaylistId()!==queue.playlistId || !Array.isArray(actualIds) ||
         actualIds.length!==expectedIds.length || actualIds.some((id,index)=>id!==expectedIds[index])) return {status:"uncertain"};
      if(player.getPlaylistIndex()===target.index && player.getVideoData()?.video_id===target.videoId && player.getPlayerState()===1) {
        return {status:"confirmed",entryId:target.entryId};
      }
      const remaining=deadline-now();if(remaining<=0)return {status:"uncertain"};
      await wait(Math.min(100,remaining));
    } while(true);
  } catch {return {status:"uncertain"};}
  finally {pending.delete(player);}
}
