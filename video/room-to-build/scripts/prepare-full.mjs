import fs from 'node:fs';
import {execFileSync} from 'node:child_process';
const crops={
 'weather-rail':['calendar',0,0,480,36],
 'hardware-rail':['hardware',0,0,480,36],
 'settings-notch':['settings',160,60,550,480],
 'settings-notch-custom':['settings-custom',160,60,550,480],
};
for(const [name,[source,x,y,w,h]] of Object.entries(crops))execFileSync('ffmpeg',['-v','error','-y','-i',`public/full/${source}.png`,'-vf',`crop=${w*4}:${h*4}:${x*4}:${y*4}`,'-frames:v','1',`public/full/${name}.png`]);
fs.writeFileSync('public/bloom.svg',fs.readFileSync('../../bloom/public/bloom.svg','utf8').replace('<svg ','<svg xmlns="http://www.w3.org/2000/svg" ').replaceAll('fill="white"','fill="#141416"'));
const s=fs.readFileSync('scripts/capture.mjs','utf8').replaceAll('2026-10-25T04:00:00+08:00','2026-10-25T09:41:00+08:00').replaceAll('2026-10-25T07:24:00+08:00','2026-10-25T13:05:00+08:00').replaceAll('2026-10-25T10:18:00+08:00','2026-10-25T15:59:00+08:00').replaceAll('2026-10-27T10:18:00+08:00','2026-10-27T15:59:00+08:00').replaceAll('2026-10-25T03:57:00+08:00','2026-10-25T09:38:00+08:00');
fs.writeFileSync('scripts/capture.mjs',s);
