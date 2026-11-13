const wait=ms=>new Promise(resolve=>setTimeout(resolve,ms));
export class SpotifyProofFixture {
  constructor(spicetify){this.source=spicetify;this.before=null;this.ids=new Set();}
  async setup(){
    const player=this.source?.Player,api=player?.origin,queue=api?.getQueue?.();
    if(this.before || !queue?.current?.uid || !queue.current.uri?.startsWith("spotify:track:") ||
       queue.queued?.length || queue.nextUp?.length) return {status:"fixture-refused"};
    if(!api.addToQueue || !api.removeFromQueue || !api.skipTo || !api.seekTo || !api.pause) return {status:"fixture-refused"};
    this.before={uri:queue.current.uri,uid:queue.current.uid,position:player.getProgress(),playing:player.isPlaying(),
      shuffle:player.getShuffle(),repeat:player.getRepeat()};
    await api.addToQueue(Array.from({length:4},()=>({uri:this.before.uri})));
    for(let i=0;i<50;i++){
      const items=api.getQueue().queued;
      if(items.length===4 && items.every(t=>t.uri===this.before.uri && t.uid) && new Set(items.map(t=>t.uid)).size===4){
        this.ids=new Set(items.map(t=>t.uid));
        return {status:"fixture-ready",before:this.before,occurrences:items.map(t=>({uri:t.uri,uid:t.uid}))};
      }
      await wait(100);
    }
    return {status:"fixture-uncertain",before:this.before};
  }
  async restore(){
    const player=this.source.Player,api=player.origin;
    if(!this.before) return {status:"no-fixture"};
    const before=this.before;
    // Only the original occurrence and our four known IDs are eligible for restoration.
    const current=api.getQueue().current;
    if(current.uid!==before.uid && !this.ids.has(current.uid)) return {status:"restore-refused-source-changed"};
    if(current.uid!==before.uid) await api.skipTo({uri:before.uri,uid:before.uid});
    for(let i=0;api.getQueue().current?.uid!==before.uid;i++){
      if(i>=50) return {status:"restore-uncertain"};
      await wait(100);
    }
    const ours=api.getQueue().queued.filter(t=>this.ids.has(t.uid));
    if(ours.length) await api.removeFromQueue(ours.map(t=>({uri:t.uri,uid:t.uid})));
    await api.seekTo(before.position);
    if(!before.playing) await api.pause();
    if(player.getShuffle()!==before.shuffle || player.getRepeat()!==before.repeat) return {status:"restore-mode-mismatch"};
    this.before=null;
    return {status:"restored",before};
  }
}
