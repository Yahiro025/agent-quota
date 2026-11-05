const ORIGIN="https://xpui.app.spotify.com";
const LIMIT=1024*1024;

export function createProofHandler({key,onObservation,takeCommand}) {
  return async request=>{
    const path=new URL(request.url).pathname;
    if(request.headers.get("Origin")!==ORIGIN || ![`/${key}/observe`,`/${key}/command`].includes(path)) {
      return new Response(null,{status:403});
    }
    const headers={"Access-Control-Allow-Origin":ORIGIN,"Vary":"Origin","Cache-Control":"no-store",
      "Access-Control-Allow-Methods":"GET, POST, OPTIONS","Access-Control-Allow-Headers":"content-type"};
    if(request.method==="OPTIONS") return new Response(null,{status:204,headers});
    if(path.endsWith("/command") && request.method==="GET") {
      return Response.json(await takeCommand(),{headers});
    }
    if(!path.endsWith("/observe") || request.method!=="POST") return new Response(null,{status:405,headers});
    if(Number(request.headers.get("content-length"))>LIMIT) return new Response(null,{status:413,headers});
    const reader=request.body?.getReader();
    if(!reader) return new Response(null,{status:400,headers});
    const chunks=[];let size=0;
    try {
      while(true){
        const {done,value}=await reader.read(); if(done) break;
        size+=value.byteLength;
        if(size>LIMIT){await reader.cancel();return new Response(null,{status:413,headers});}
        chunks.push(value);
      }
      const bytes=new Uint8Array(size);let offset=0;
      for(const chunk of chunks){bytes.set(chunk,offset);offset+=chunk.length;}
      const value=JSON.parse(new TextDecoder().decode(bytes));
      await onObservation(value);
      return new Response(null,{status:204,headers});
    } catch {
      return new Response(null,{status:400,headers});
    } finally {reader.releaseLock();}
  };
}
