import {mkdir,copyFile} from "node:fs/promises";
import {join} from "node:path";
const source=join(import.meta.dir,"proof-extension"),out=join(import.meta.dir,"proof-build");
await mkdir(out,{recursive:true});
const result=await Bun.build({entrypoints:[join(source,"page.js")],target:"browser",format:"iife"});
if(!result.success)throw new Error("YouTube proof connector build failed");
await Bun.write(join(out,"page.bundle.js"),await result.outputs[0].text());
for(const file of ["manifest.json","popup.html","popup.css","popup.js"])await copyFile(join(source,file),join(out,file));
console.log(out);
