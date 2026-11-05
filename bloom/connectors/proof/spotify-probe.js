import {captureSpotify} from "./spotify-snapshot.js";
import {selectOccurrence} from "./spotify-selection.js";
import {SpotifyProofFixture} from "./spotify-fixture.js";

// Generated config is ephemeral, local-only, and never committed with its private path.
const config=globalThis.__BLOOM_QUEUE_PROOF_CONFIG__;
if(config && Date.now()<config.expiresAt) {
  const session=crypto.randomUUID();
  let lastCommand=null;
  let fixture=null;
  const observe=async value=>{
    const response=await fetch(`${config.endpoint}/observe`,{method:"POST",headers:{"Content-Type":"application/json"},
      body:JSON.stringify({session,at:new Date().toISOString(),...value}),signal:AbortSignal.timeout(2000)});
    if(!response.ok) throw new Error("Probe transport unavailable");
  };
  async function tick(){
    if(Date.now()>config.expiresAt) return;
    try {
      await observe({snapshot:captureSpotify(globalThis.Spicetify)});
      const response=await fetch(`${config.endpoint}/command`,{signal:AbortSignal.timeout(2000)});
      if(response.ok){
        const command=await response.json();
        if(command && command.id!==lastCommand){
          lastCommand=command.id;
          const api=globalThis.Spicetify?.Player?.origin;
          let result={status:"session-mismatch"};
          if(command.session===session){
            if(command.action==="setup-fixture"){
              fixture??=new SpotifyProofFixture(globalThis.Spicetify);
              result=await fixture.setup();
            } else if(command.action==="restore-fixture"){
              result=fixture?await fixture.restore():{status:"no-fixture"};
            } else if(command.action==="resume-fixture" && fixture?.before){
              await api.resume();result={status:"resume-dispatched"};
            } else {result=await selectOccurrence(api,command);}
          }
          await observe({result:{id:command.id,...result},snapshot:captureSpotify(globalThis.Spicetify)});
        }
      }
    } catch {
      // No source errors or credentials are logged. A missing heartbeat is visible at the collector.
    }
    if(Date.now()<config.expiresAt) setTimeout(tick,2000);
  }
  tick();
}
