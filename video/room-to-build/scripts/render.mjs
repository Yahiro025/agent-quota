import {bundle} from '@remotion/bundler';
import {selectComposition,renderMedia,renderStill} from '@remotion/renderer';
import path from 'node:path';
import fs from 'node:fs';
const mode=process.argv[2]||'stills';
const browserExecutable='C:/Program Files/Google/Chrome/Application/chrome.exe';
const serveUrl=await bundle({entryPoint:path.resolve('src/index.ts'),publicDir:path.resolve('public')});
const inputProps={bgm:process.argv[3]!=='nobgm',sound:mode==='full'||mode==='audio'};
const composition=await selectComposition({serveUrl,id:'RoomToBuild',browserExecutable,inputProps});
if(mode==='stills'){
 for(const frame of (process.argv[3]?.match(/^\d+(,\d+)*$/)?process.argv[3].split(',').map(Number):[150,360,650,900,1150,1430,1670,1900,2180,2275,2440,2540,2685])){
  await renderStill({serveUrl,composition,output:`out/qa/f${frame}.png`,frame,imageFormat:'png',browserExecutable,inputProps});console.log('still',frame);
 }
}else if(mode==='audio'){
 await renderMedia({serveUrl,composition,outputLocation:inputProps.bgm?'out/audio-music.wav':'out/audio-sfx.wav',codec:'wav',browserExecutable,inputProps});console.log('audio complete',inputProps.bgm?'music':'sfx');
}else{
 let last=-1;
  await renderMedia({serveUrl,composition,outputLocation:mode==='preview'?'out/preview.mp4':mode==='picture'?'out/picture.mp4':inputProps.bgm?'out/render-music.mp4':'out/render-sfx.mp4',codec:'h264',crf:mode==='preview'?25:17,pixelFormat:'yuv420p',audioCodec:'aac',audioBitrate:'320k',scale:mode==='preview'?.5:1,concurrency:4,browserExecutable,inputProps,onProgress:p=>{const n=Math.floor(p.progress*10);if(n!==last){last=n;console.log(mode,n*10+'%')}}});
}
