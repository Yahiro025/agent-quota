import {readYouTubeQueue,selectYouTubeOccurrence} from "../queue.js";

// Page-world access is needed for YouTube's own player API. No privileged extension
// APIs or credentials are exposed in this object; it only operates on this page.
if(!globalThis.__bloomQueueCheck) {
  const session=crypto.randomUUID();
  const player=()=>document.getElementById("movie_player");
  function describe(index,id){
    const rows=document.querySelectorAll("ytd-playlist-panel-video-renderer");
    for(const row of rows){
      const data=row.data;
      const endpoint=data?.navigationEndpoint?.watchEndpoint;
      if(endpoint?.index===index && data?.videoId===id){
        return {title:data.title?.simpleText??data.title?.runs?.map(r=>r.text).join(""),
          artist:data.shortBylineText?.runs?.map(r=>r.text).join(""),duration:data.lengthText?.simpleText};
      }
    }
    return null;
  }
  globalThis.__bloomQueueCheck={
    read:()=>readYouTubeQueue(player(),session,describe),
    select:request=>selectYouTubeOccurrence(player(),session,request),
  };
}
