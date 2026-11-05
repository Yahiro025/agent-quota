import {mkdir} from "node:fs/promises";
import {resolve,join} from "node:path";
import {createProofHandler} from "./proof-transport.js";

// Disposable instrumentation, not a production service. No account API requests.
const directory=resolve(process.argv[2]??".superpowers/sdd/2026-10-29-bloom-media-queue/live");
await mkdir(directory,{recursive:true});
const key=crypto.randomUUID().replaceAll("-","");
const expiresAt=Date.now()+10*60*1000;
const observations=[];
const delivered=new Set();
let write=Promise.resolve();
const handler=createProofHandler({key,
  async onObservation(value){
    observations.push(value);
    if(observations.length>20) observations.shift();
    // Serialize writes so older requests cannot overwrite newer diagnostic snapshots.
    write=write.then(async()=>{
      await Bun.write(join(directory,"latest.json"),JSON.stringify(value,null,2));
      await Bun.write(join(directory,"observations.json"),JSON.stringify(observations,null,2));
      if(value.result) await Bun.write(join(directory,"result.json"),JSON.stringify(value,null,2));
    });
    await write;
  },
  async takeCommand(){
    const file=Bun.file(join(directory,"command.json"));
    if(!(await file.exists())) return null;
    try {
      if(file.size>4096 || delivered.size>=16) return null;
      const command=await file.json();
      if(typeof command.id!=="string" || typeof command.session!=="string" ||
         typeof command.uid!=="string" || typeof command.uri!=="string" || typeof command.revision!=="string" ||
         delivered.has(command.id)) return null;
      delivered.add(command.id); // Lost delivery is uncertain; never replay automatically.
      return command;
    } catch {return null;}
  }});
const server=Bun.serve({hostname:"127.0.0.1",port:0,maxRequestBodySize:1024*1024,fetch:handler});
const config={endpoint:`http://127.0.0.1:${server.port}/${key}`,expiresAt};
await Bun.write(join(directory,"config.json"),JSON.stringify(config));
const result=await Bun.build({entrypoints:[join(import.meta.dir,"spotify-probe.js")],target:"browser",minify:false,
  define:{"globalThis.__BLOOM_QUEUE_PROOF_CONFIG__":JSON.stringify(config)}});
if(!result.success){server.stop(true);throw new Error("Probe build failed");}
await Bun.write(join(directory,"bloom-queue-proof.js"),await result.outputs[0].text());
console.log(`Proof collector listening on 127.0.0.1:${server.port}; expires in 10 minutes. Private endpoint not logged.`);
setTimeout(()=>{server.stop(true);process.exit(0);},10*60*1000);
