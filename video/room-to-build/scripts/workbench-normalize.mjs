import fs from 'node:fs';
const wb='C:/Users/Bennett/.agents/skills/video-shotcraft/workbench';
for(const name of ['demo-index.ts','legacy-index.ts']){
 const p=wb+'/src/cards/'+name;if(!fs.existsSync(p))continue;
 const text=fs.readFileSync(p,'utf8').replace(/from "(@demos[^"]+)"/g,(_,v)=>'from "'+v.replaceAll('\\','/')+'"');
 fs.writeFileSync(p,text);
}
console.log('Normalized generated workbench import paths for Windows.');
