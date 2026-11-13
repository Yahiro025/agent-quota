import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {createRequire} from 'node:module';
const require = createRequire(import.meta.url);
const {chromium} = require('../../../.scratch/qa/node_modules/playwright');
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const repo = path.resolve(root, '../..');
const out = path.join(root, 'public/textures');
fs.mkdirSync(out, {recursive:true});
const browser = await chromium.launch({headless:true,channel:'msedge'});
const context = await browser.newContext({viewport:{width:1920,height:1080},deviceScaleFactor:4,timezoneId:'Asia/Singapore'});
const page = await context.newPage();
await page.clock.setFixedTime(new Date('2026-10-25T09:41:00+08:00'));
await page.addInitScript({path:path.join(repo,'bloom/e2e/tauri-mock.js')});
await page.addInitScript(()=>Object.defineProperty(navigator,'getBattery',{value:async()=>({level:.86,charging:false,addEventListener(){},removeEventListener(){}})}));
await page.goto('http://127.0.0.1:5174');
await page.locator('div.bloom').waitFor();
await page.waitForTimeout(1300);
await page.evaluate(()=>document.fonts.ready);
const reset='2026-10-25T13:05:00+08:00';
const readings=[
 {id:'claude',state:'ok',used:42,plan:'Pro',reset5h:reset,windows:[{label:'5h',used:42},{label:'week',used:28}],details:[{label:'5h',value:'42% used',resetAt:reset},{label:'Week',value:'28% used',resetAt:'2026-10-27T15:59:00+08:00'}]},
 {id:'kiro',state:'ok',used:62,details:[{label:'Credits',value:'620 of 1,000 used'}]},
 {id:'codex',state:'ok',used:18,plan:'Plus',reset5h:reset,windows:[{label:'5h',used:18},{label:'week',used:36}],details:[{label:'5h',value:'18% used',resetAt:reset},{label:'Week',value:'36% used',resetAt:'2026-10-27T15:59:00+08:00'}]},
 {id:'cursor',state:'ok',used:71,plan:'Pro'},
 {id:'copilot',state:'ok',used:35,details:[{label:'Premium requests',value:'35% used'}]},
 {id:'devin',state:'ok',used:56,plan:'Pro',windows:[{label:'day',used:56},{label:'week',used:28}],details:[{label:'Day',value:'56% used',resetAt:'2026-10-25T15:59:00+08:00'},{label:'Week',value:'28% used',resetAt:'2026-10-27T15:59:00+08:00'}]},
 {id:'antigravity',state:'ok',used:24,reset5h:reset,groups:[{name:'Gemini 3 Pro',windows:[{label:'5h',used:24}]},{name:'Claude Sonnet 4.5',windows:[{label:'5h',used:12}]},{name:'GPT-OSS 120B',windows:[{label:'5h',used:8}]}]}
];
const emit=async(data)=>page.evaluate(data=>window.__emit('quota-snapshot',data),data);
await emit(readings);
await page.locator('div.bloom').click();
for(let i=0;i<5&&await page.getByTestId('quota-space').count()===0;i++){
 await page.locator('button.panel-arrow.next').click(); await page.waitForTimeout(400);
}
await page.getByTestId('quota-space').waitFor();
await page.getByTestId('quota-pin').click();
await page.mouse.move(20,900);
await page.waitForTimeout(1600);
await page.addStyleTag({content:'html,body,#root {background:transparent !important;}'});
const layouts={};
const capture=async(name)=>{
 await page.waitForTimeout(500);
 const island=page.locator('div.bloom');
 const b=await island.boundingBox();
 layouts[name]={island:b,elements:{}};
 for(const [key,sel] of Object.entries({panel:'[data-testid="quota-space"]',detail:'[data-testid="quota-detail"]',reset:'[data-testid="quota-reset"]',groups:'[data-testid="quota-groups"]',groupsToggle:'[data-testid="quota-groups-toggle"]',refresh:'[data-testid="quota-refresh"]',codex:'[data-testid="quota-cell-codex"]',claude:'[data-testid="quota-cell-claude"]',antigravity:'[data-testid="quota-cell-antigravity"]'})){
  const el=page.locator(sel);if(await el.count()){const eb=await el.first().boundingBox();if(eb)layouts[name].elements[key]={x:eb.x-b.x,y:eb.y-b.y,w:eb.width,h:eb.height};}
 }
 await island.screenshot({path:path.join(out,`${name}.png`),omitBackground:true,animations:'disabled'});
 await page.screenshot({path:path.join(out,`${name}-page.png`),omitBackground:true,animations:'disabled'});
 console.log(name,JSON.stringify(layouts[name]));
};
await page.getByTestId('quota-cell-claude').click(); await capture('overview');
await page.getByTestId('quota-cell-codex').click(); await capture('codex');
if(await page.getByTestId('quota-reset').count())await page.getByTestId('quota-reset').click(); await capture('clock');
await page.getByTestId('quota-cell-antigravity').click(); await capture('groups-closed');
await page.getByTestId('quota-groups-toggle').click(); await capture('groups');
const stale=structuredClone(readings);stale[2]={...stale[2],state:'stale',err:'Connection timed out.',lastGoodAt:'2026-10-25T09:38:00+08:00'};
await emit(stale);await page.getByTestId('quota-cell-codex').click();await capture('stale');
await page.getByTestId('quota-refresh').click();await capture('refresh');
await emit(readings);await capture('fresh');
await page.getByTestId('quota-close').click();await page.waitForTimeout(1100);await capture('rest');
const urgent=structuredClone(readings);urgent[0]={...urgent[0],used:92,windows:[{label:'5h',used:92},{label:'week',used:28}],details:[{label:'5h',value:'92% used',resetAt:reset},{label:'Week',value:'28% used',resetAt:'2026-10-27T15:59:00+08:00'}]};
await emit(urgent);await page.mouse.move(20,1000);await page.waitForTimeout(1300);await capture('urgent');
await page.locator('div.bloom').hover();await page.waitForTimeout(1000);await capture('urgent-hover');
await page.locator('div.bloom').click();for(let i=0;i<5&&await page.getByTestId('quota-space').count()===0;i++){await page.locator('button.panel-arrow.next').click();await page.waitForTimeout(300)};
await page.getByTestId('quota-cell-claude').click();await page.getByTestId('quota-pin').click();await capture('urgent-open');
fs.writeFileSync(path.join(out,'layout.json'),JSON.stringify(layouts,null,2));
fs.writeFileSync(path.join(out,'demo-data.json'),JSON.stringify({clock:'2026-10-25T09:41:00+08:00',readings},null,2));
await browser.close();
