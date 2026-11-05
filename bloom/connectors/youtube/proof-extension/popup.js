const read=document.getElementById("read"),status=document.getElementById("status"),list=document.getElementById("queue");
let snapshot=null,targetTab=null,pending=false;
async function run(action,request){
  const results=await chrome.scripting.executeScript({target:{tabId:targetTab},world:"MAIN",
    func:async(action,request)=>action==="read"?globalThis.__bloomQueueCheck?.read():await globalThis.__bloomQueueCheck?.select(request),
    args:[action,request??null]});
  return results[0]?.result;
}
function render(){
  list.replaceChildren();
  for(const item of snapshot?.entries??[]){
    const row=document.createElement("li"),button=document.createElement("button");
    const title=item.title??`Video ${item.index+1} · ${item.videoId}`;
    button.textContent=title;button.title=title;button.disabled=pending;
    button.setAttribute("aria-label",`Play ${title} now`);
    button.onclick=async()=>{
      if(pending)return;pending=true;read.disabled=true;render();status.textContent="Waiting for playback confirmation…";
      try{
        const result=await run("select",{session:snapshot.session,revision:snapshot.revision,...item});
        status.textContent=result?.status==="confirmed"?"Confirmed: selected occurrence is playing.":
          `Selection ${result?.status??"unavailable"}. Read the queue again before retrying.`;
        snapshot=await run("read");
      }catch{snapshot=null;status.textContent="Connection changed. Read the queue again before retrying.";}
      finally{pending=false;read.disabled=false;render();}
    };
    row.append(button);list.append(row);
  }
}
read.onclick=async()=>{
  if(pending)return;read.disabled=true;status.textContent="Reading queue…";
  try{
    const [tab]=await chrome.tabs.query({active:true,currentWindow:true});
    if(!tab?.id || new URL(tab.url??"").origin!=="https://www.youtube.com")throw new Error("unsupported");
    targetTab=tab.id;
    await chrome.scripting.executeScript({target:{tabId:targetTab},world:"MAIN",files:["page.bundle.js"]});
    snapshot=await run("read");
    status.textContent=snapshot?.available?`${snapshot.totalUpcoming} upcoming. Select a row to test immediate playback.`:"Queue unavailable for this player.";
    render();
  }catch{snapshot=null;render();status.textContent="Open a YouTube playlist or explicit queue, then try again.";}
  finally{read.disabled=false;}
};
