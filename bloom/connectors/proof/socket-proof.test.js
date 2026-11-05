import {expect,test} from "bun:test";
import {authorizeSocket} from "./socket-proof.js";
test("authenticates the source and loopback host as well as origin",()=>{
  const request=(change={})=>new Request("http://127.0.0.1:39641/spotify",{headers:{
    host:"127.0.0.1:39641",origin:"https://xpui.app.spotify.com",upgrade:"websocket",
    "sec-websocket-protocol":"bloom.v1, test-private-nonce",...change}});
  expect(authorizeSocket(request(),39641,"test-private-nonce")).toBe(true);
  for(const change of [{host:"attacker.example"},{origin:"https://www.youtube.com"},{"sec-websocket-protocol":"bloom.v1, wrong"},{upgrade:"http"}]){
    expect(authorizeSocket(request(change),39641,"test-private-nonce")).toBe(false);
  }
});
