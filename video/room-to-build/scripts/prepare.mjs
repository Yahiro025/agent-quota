import fs from 'node:fs';
import {execFileSync} from 'node:child_process';
const s=fs.readFileSync('../../bloom/src/quota/marks/index.tsx','utf8');
const p=s.match(/const markCodex =\s*"([^"]+)/)[1];
const g=s.match(/const markCodexGlyph =\s*"([^"]+)/)[1];
fs.writeFileSync('public/codex-mark.svg',`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect width="24" height="24" rx="12" fill="white"/><defs><linearGradient id="c" x1="0" y1="0" x2="0" y2="1"><stop stop-color="#B1A7FF"/><stop offset=".5" stop-color="#7A9DFF"/><stop offset="1" stop-color="#3941FF"/></linearGradient></defs><path d="${p}" fill="url(#c)"/><path d="${g}" fill="white"/></svg>`);
fs.copyFileSync('../../internal/marks/assets/LOBEHUB-MIT-LICENSE.txt','public/LOBEHUB-MIT-LICENSE.txt');
const licenses=fs.readdirSync('../../internal/text/assets').filter(f=>/license|ofl/i.test(f));
for(const file of licenses)fs.copyFileSync('../../internal/text/assets/'+file,'public/fonts/'+file);
const crops={
 'overview-row':['overview',12,60,462,72],
 'codex-row':['codex',12,60,462,72],
 'codex-detail':['codex',24,132,370,94],
 'overview-detail':['overview',24,132,370,94],
 'codex-windows':['codex',24,132,370,61],
 'clock-windows':['clock',24,132,370,61],
 'codex-reset':['codex',30,191,110,28],
 'clock-reset':['clock',30,191,110,28],
 'models-closed':['groups-closed',24,132,438,74],
 'models-open':['groups',24,132,438,140],
 'stale-detail':['stale',24,132,370,123],
 'fresh-detail':['fresh',24,132,370,89],
 'refresh-button':['stale',390,260,73,28],
 'urgent-cell':['urgent-open',28,60,62,72],
};
for(const [name,[source,x,y,w,h]] of Object.entries(crops)){
 execFileSync('ffmpeg',['-v','error','-y','-i',`public/textures/${source}.png`,'-vf',`crop=${w*4}:${h*4}:${x*4}:${y*4}`,'-frames:v','1',`public/textures/${name}.png`]);
}
