import {bundle} from '@remotion/bundler';
import {selectComposition,renderStill} from '@remotion/renderer';
import {spawnSync} from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
const wb='C:/Users/Bennett/.agents/skills/video-shotcraft/workbench';
const python='C:/Users/Bennett/.cache/codex-runtimes/codex-primary-runtime/dependencies/python/python.exe';
const browserExecutable='C:/Program Files/Google/Chrome/Application/chrome.exe';
fs.mkdirSync('out/parity',{recursive:true});
const serveUrl=await bundle({entryPoint:path.join(wb,'src/remotion/index.ts'),publicDir:path.join(wb,'public'),webpackOverride:c=>({...c,resolve:{...c.resolve,symlinks:false,alias:{...c.resolve?.alias,'@proj':path.join(wb,'proj'),'@demos':path.join(wb,'demosrc')}}})});
const comps={};for(const id of ['ProjImported','ProjOriginal'])comps[id]=await selectComposition({serveUrl,id,browserExecutable});
const results=[];
for(const frame of [150,365,722,1150,1970,2275,2440,2685]){
 for(const id of Object.keys(comps))await renderStill({serveUrl,composition:comps[id],output:`out/parity/${id}-${frame}.png`,frame,imageFormat:'png',browserExecutable});
 const r=spawnSync(python,['-c',`import sys,json,numpy as np
from PIL import Image
a=np.asarray(Image.open(sys.argv[1]).convert('RGB')).astype('int16');b=np.asarray(Image.open(sys.argv[2]).convert('RGB')).astype('int16')
d=np.abs(a-b); print(json.dumps({'badPixels':int((d.max(axis=2)>2).sum()),'pixels':int(a.shape[0]*a.shape[1]),'maxChannelDifference':int(d.max())}))`, `out/parity/ProjImported-${frame}.png`,`out/parity/ProjOriginal-${frame}.png`],{encoding:'utf8'});
 if(r.status!==0)throw Error(r.stderr);const result={frame,...JSON.parse(r.stdout)};result.pass=result.badPixels/result.pixels<.001;results.push(result);console.log(result);
}
fs.writeFileSync('out/parity/results.json',JSON.stringify(results,null,2));if(results.some(r=>!r.pass))process.exitCode=1;
