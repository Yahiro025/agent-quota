export function authorizeSocket(request,port,nonce){
  const protocols=request.headers.get("sec-websocket-protocol")?.split(",").map(s=>s.trim());
  return new URL(request.url).pathname==="/spotify" && request.headers.get("host")===`127.0.0.1:${port}` &&
    request.headers.get("origin")==="https://xpui.app.spotify.com" && request.headers.get("upgrade")?.toLowerCase()==="websocket" &&
    protocols?.length===2 && protocols[0]==="bloom.v1" && protocols[1]===nonce;
}

if(import.meta.main){
  const directory=process.argv[2];if(!directory)throw new Error("Supply a private proof directory");
  const nonce=crypto.randomUUID().replaceAll("-","");
  const server=Bun.serve({hostname:"127.0.0.1",port:0,
    fetch(request,server){
      if(!authorizeSocket(request,server.port,nonce))return new Response(null,{status:403});
      if(server.upgrade(request,{headers:{"Sec-WebSocket-Protocol":"bloom.v1"}}))return;
      return new Response(null,{status:400});
    },
    websocket:{maxPayloadLength:256,message(socket,message){
      if(message==="spotify-ready"){
        Bun.write(directory+"/websocket-result.json",JSON.stringify({confirmed:true,at:new Date().toISOString(),
          source:"Installed Spotify/Spicetify",transport:"Authenticated loopback WebSocket",origin:"https://xpui.app.spotify.com"}));
        socket.send("confirmed");
      }
      socket.close();
    }}
  });
  const code=`(()=>{const s=new WebSocket(${JSON.stringify(`ws://127.0.0.1:${server.port}/spotify`)},["bloom.v1",${JSON.stringify(nonce)}]);s.onopen=()=>s.send("spotify-ready");s.onmessage=()=>s.close();s.onerror=()=>s.close();setTimeout(()=>s.close(),15000);})();`;
  await Bun.write(directory+"/bloom-ws-proof.js",code);
  console.log("Temporary WebSocket proof ready; private endpoint not logged.");
  setTimeout(()=>{server.stop(true);process.exit(0);},120000);
}
