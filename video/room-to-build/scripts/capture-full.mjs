import fs from 'node:fs';
import path from 'node:path';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url);
const {chromium}=require('../../../.scratch/qa/node_modules/playwright');
const root=path.resolve('.');const out=path.join(root,'public/full');fs.mkdirSync(out,{recursive:true});
const browser=await chromium.launch({headless:true,channel:'msedge'});
const layouts={};
const art=`<svg xmlns="http://www.w3.org/2000/svg" width="600" height="600" viewBox="0 0 600 600"><defs><radialGradient id="g"><stop stop-color="#ffcf86"/><stop offset=".48" stop-color="#e98440"/><stop offset="1" stop-color="#3e1816"/></radialGradient></defs><rect width="600" height="600" fill="#111014"/><circle cx="345" cy="270" r="230" fill="url(#g)"/><path d="M-70 480 Q170 110 390 400 T750 260" fill="none" stroke="#100e12" stroke-width="135"/><path d="M-70 485 Q170 115 390 405 T750 265" fill="none" stroke="#ffdb9b" stroke-opacity=".7" stroke-width="2"/></svg>`;
fs.writeFileSync(path.join(root,'public/album-art.svg'),art);
const artUrl='data:image/svg+xml;base64,'+Buffer.from(art).toString('base64');
const media={title:'House Vibez',artist:'Lily J',is_playing:true,has_media:true,artwork:[artUrl],position_ms:92500,duration_ms:111464,seek_enabled:true,position_updated_at:new Date('2026-10-25T09:41:00+08:00').getTime()};
async function open(prefs={},url='/',label='main'){
 const ctx=await browser.newContext({viewport:{width:1920,height:1080},deviceScaleFactor:4,timezoneId:'Asia/Singapore'});
 const page=await ctx.newPage();page.setDefaultTimeout(5000);await page.clock.setFixedTime(new Date('2026-10-25T09:41:00+08:00'));
 await page.addInitScript({path:path.resolve('../../bloom/e2e/tauri-mock.js')});
 await page.addInitScript(({prefs,label,artUrl})=>{
  const settings={'bloom-first-run':'done','bloom-app-version':'3.10.0','bloom-dock-enabled':'false','bloom-calendar-enabled':'true','bloom-timer-sound-enabled':'false','bloom-weather-enabled':'true','bloom-weather-lat':'1','bloom-weather-lon':'1','bloom-weather-city':'Demo City','bloom-weather-cached-temp':'24','bloom-weather-cached-condition':'Clear','bloom-notch-remember-panel':'false','bloom-notch-default-panel':'calendar','bloom-status-widgets':JSON.stringify({left:['weather'],right:['battery']}),...prefs};
  for(const [k,v] of Object.entries(settings))localStorage.setItem(k,v);
  Object.defineProperty(navigator,'getBattery',{value:async()=>({level:.86,charging:false,addEventListener(){},removeEventListener(){}})});
  const base=window.__TAURI_INTERNALS__.invoke;
  const devices=[{id:'demo-headphones',name:'Studio Headphones',kind:'audio',connected:true,battery:82,can_connect:true},{id:'demo-keyboard',name:'Wireless Keyboard',kind:'keyboard',connected:true,battery:94,can_connect:false},{id:'demo-speaker',name:'Desk Speaker',kind:'audio',connected:false,battery:null,can_connect:true}];
  const responses={load_settings:settings,get_wifi_status:{enabled:true,connected:true,ssid:'Studio'},get_bluetooth_state:true,get_battery_saver_state:false,get_volume_state:{volume:.48,is_muted:false},get_brightness:65,get_cpu_usage:18,get_ram_usage:42,get_disk_space:68,get_network_speed:[128000,2400000],bluetooth_list_devices:devices,wifi_list_networks:{networks:[{ssid:'Studio',signal:96,security:'wpa3',connected:true,saved:true},{ssid:'Workspace',signal:78,security:'wpa2',connected:false,saved:true},{ssid:'Guest',signal:55,security:'open',connected:false,saved:false}],location_denied:false},get_audio_sessions:[{pid:1001,name:'Music',process_path:null,volume:.72,is_muted:false},{pid:1002,name:'Browser',process_path:null,volume:.34,is_muted:false},{pid:1003,name:'Editor',process_path:null,volume:.50,is_muted:false}]};
  window.__TAURI_INTERNALS__.metadata.currentWindow.label=label;window.__TAURI_INTERNALS__.metadata.currentWebview.windowLabel=label;
  window.__TAURI_INTERNALS__.invoke=(cmd,args)=>{
   if(cmd==='shelf_thumbnail')return Promise.resolve({thumb:args?.path?.endsWith('.png')?artUrl:null,text:args?.path?.endsWith('.md')?'Launch notes\nMake room for the next idea.':null,folder:false});
   if(cmd==='shelf_prune')return Promise.resolve(args?.paths||[]);
   if(cmd in responses)return Promise.resolve(responses[cmd]);
   if(cmd==='save_setting'){settings[args.key]=String(args.value);return Promise.resolve();}
   if(cmd==='get_app_icon'||cmd==='shelf_drag_icon')return Promise.resolve(null);
   return base(cmd,args);
  };
  const nativeFetch=window.fetch.bind(window);
  window.fetch=(url,opts)=>String(url).includes('open-meteo')?Promise.resolve(new Response(JSON.stringify({current:{temperature_2m:24,weather_code:0,is_day:1}}),{status:200,headers:{'content-type':'application/json'}})):nativeFetch(url,opts);
 },{prefs,label,artUrl});
 await page.goto('http://127.0.0.1:5174'+url,{timeout:30000,waitUntil:'domcontentloaded'});await page.waitForTimeout(1800);await page.evaluate(()=>document.fonts.ready);
 await page.addStyleTag({content:'html,body,#root{background:transparent!important;}'});
 page.on('pageerror',e=>console.log('PAGE ERROR',e.message));
 return page;
}
async function capture(page,name,selector='div.bloom'){
 await page.waitForTimeout(650);const el=page.locator(selector).first();await el.waitFor();
 const b=await el.boundingBox();layouts[name]={...b,selector,elements:{}};
 const elements={art:'.premium-album-art',visualizer:'.header-visualizer',timer:'.timer-clock',calendar:'.calendar-column',start:'.timer-btn-play',tile0:'.shelf-tile:nth-child(1)',tile1:'.shelf-tile:nth-child(2)',tile2:'.shelf-tile:nth-child(3)'};
 const shelf=page.locator('.shelf-items-grid');if(await shelf.count())await shelf.evaluate(e=>{e.scrollLeft=0});
 for(const [key,sel] of Object.entries(elements)){const e=page.locator(sel).first();if(await e.count()&&await e.isVisible()){const eb=await e.boundingBox();if(eb)layouts[name].elements[key]={x:eb.x-b.x,y:eb.y-b.y,w:eb.width,h:eb.height};}}
 for(const [key,sel] of Object.entries(elements)){if(layouts[name].elements[key])await page.locator(sel).first().screenshot({path:path.join(out,name+'-'+key+'.png'),omitBackground:true,animations:'disabled'});}
 if(await shelf.count())await shelf.evaluate(e=>{e.scrollLeft=0});
 await el.screenshot({path:path.join(out,name+'.png'),omitBackground:true,animations:'disabled'});
 fs.writeFileSync(path.join(out,'layout.json'),JSON.stringify(layouts,null,2));
 console.log('captured',name,JSON.stringify(b));
}
const p=await open();await capture(p,'rest');await p.locator('div.bloom').click();await p.locator('.calendar-timer-content').waitFor();await capture(p,'calendar');
await p.locator('.timer-btn-play').click();await capture(p,'timer-running');await p.locator('.timer-btn-play').click();await capture(p,'timer-paused');
await p.evaluate(m=>window.__emit('media-update',m),media);await p.evaluate(()=>window.__emit('audio-visualization',{frequencies:[.45,.8,.6,.95,.5]}));
await p.waitForTimeout(700);await p.evaluate(()=>window.__emit('notch-pointer-left'));await p.waitForTimeout(700);
const musicBtn=p.locator('button[aria-label^="Open music"]');await musicBtn.first().waitFor();await musicBtn.first().click();
await capture(p,'music');
await p.locator('.premium-album-art').click().catch(()=>{});await capture(p,'music-compact');
await p.evaluate(m=>window.__emit('media-update',{...m,is_playing:false}),media);await capture(p,'music-paused');
await p.evaluate(m=>window.__emit('media-update',m),media);
await p.evaluate(()=>{window.__emit('shelf-drag-enter',{paths:['C:\\Demo\\Cover.png','C:\\Demo\\Notes.md']});});await capture(p,'shelf-dropzone');
await p.evaluate(()=>window.__emit('shelf-drag-drop',{paths:['C:\\Demo\\Cover.png','C:\\Demo\\Notes.md']}));await p.locator('div.bloom').hover();await capture(p,'shelf-files');
await p.evaluate(()=>window.__emit('shelf-drag-drop',{text:'Make room for the next idea.'}));await p.locator('div.bloom').hover();await capture(p,'shelf');
await p.evaluate(()=>window.__emit('notch-pointer-left'));await p.waitForTimeout(800);await capture(p,'activities', 'div.bloom');
const boxes=await p.locator('div.bloom,button.activity-bubble').evaluateAll(els=>els.map(e=>{const b=e.getBoundingClientRect();return{x:b.x,y:b.y,w:b.width,h:b.height}}));
const minX=Math.min(...boxes.map(b=>b.x)),maxX=Math.max(...boxes.map(b=>b.x+b.w)),maxY=Math.max(...boxes.map(b=>b.y+b.h));
await p.screenshot({path:path.join(out,'activities-all.png'),clip:{x:minX-6,y:0,width:maxX-minX+12,height:maxY+6},omitBackground:true});
await p.evaluate(()=>window.__emit('quota-snapshot',[{id:'claude',state:'ok',used:92,reset5h:'2026-10-25T13:05:00+08:00'}]));
await p.locator('div.bloom').hover();
for(let i=0;i<6;i++){const next=p.getByLabel('Next panel',{exact:true});if(await next.isVisible())await next.click();else await p.mouse.wheel(0,120);await p.waitForTimeout(550);const labels=await p.locator('.qp-row-name').allTextContents();if(labels.includes('Music')&&labels.includes('Shelf'))break;}
await capture(p,'overview');
const c=await open({'bloom-notch-default-panel':'command-center'});await c.locator('div.bloom').click();await c.locator('[aria-label="Wi-Fi networks"]').waitFor();await capture(c,'controls');
await c.evaluate(()=>window.__emit('volume-change',{volume:.76,is_muted:false}));await capture(c,'controls-volume');
await c.getByLabel('Wi-Fi networks',{exact:true}).click();await capture(c,'wifi');
await c.getByLabel('Back to controls',{exact:true}).click();await c.getByLabel('Bluetooth devices',{exact:true}).click();await capture(c,'bluetooth');
const s=await open({'bloom-status-widgets':JSON.stringify({left:['cpu','ram'],right:['disk','net']})});await s.locator('div.bloom').click();await capture(s,'hardware');
const st=await open({},'/settings.html','settings');await st.setViewportSize({width:720,height:620});await st.getByText('Notch',{exact:true}).first().click();await capture(st,'settings','.settings-container');
await st.getByText('Compact',{exact:true}).click();await capture(st,'settings-custom','.settings-container');
fs.writeFileSync(path.join(out,'fixtures.json'),JSON.stringify({media,weather:{temperature:24,condition:'Clear'},deviceNames:['Studio Headphones','Wireless Keyboard','Desk Speaker'],networkNames:['Studio','Workspace','Guest'],data:'Fictional frontend demo state; no OS settings changed'},null,2));
if(process.argv.includes('--panels')){await browser.close();process.exit(0);}
const ov=await open({},'/overlay.html','overlay');await ov.evaluate(()=>window.__emit('volume-change',{volume:.64,is_muted:false}));await capture(ov,'volume','.volume-notch-wrapper');
await ov.locator('.volume-notch-action').click();await capture(ov,'mixer','.volume-notch-wrapper');
await ov.evaluate(()=>window.__emit('brightness-change',{brightness:72}));await capture(ov,'brightness','.brightness-notch-wrapper');
fs.writeFileSync(path.join(out,'layout.json'),JSON.stringify(layouts,null,2));
fs.writeFileSync(path.join(out,'fixtures.json'),JSON.stringify({media,weather:{temperature:24,condition:'Clear'},deviceNames:['Studio Headphones','Wireless Keyboard','Desk Speaker'],networkNames:['Studio','Workspace','Guest'],data:'Fictional frontend demo state; no OS settings changed'},null,2));
await browser.close();
