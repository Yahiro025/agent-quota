import {execFileSync} from 'node:child_process';
// Remotion's JPEG pipeline tags full-range BT.601. Deliver conventional HD Rec.709.
execFileSync('ffmpeg',['-v','error','-y','-i','out/picture.mp4','-an','-vf','scale=in_range=pc:out_range=tv:in_color_matrix=bt601:out_color_matrix=bt709,format=yuv420p','-c:v','libx264','-preset','medium','-crf','16','-color_range','tv','-colorspace','bt709','-color_primaries','bt709','-color_trc','bt709','-movflags','+faststart','out/picture-delivery.mp4']);
for(const edition of ['music','sfx']){
 const output=edition==='music'?'Bloom-One-Notch-Away.mp4':'Bloom-One-Notch-Away-SFX.mp4';
 execFileSync('ffmpeg',['-v','error','-y','-i','out/picture-delivery.mp4','-i',`out/master-${edition}.wav`,'-map','0:v:0','-map','1:a:0','-c:v','copy','-c:a','aac','-b:a','320k','-ar','48000','-ac','2','-t','90','-movflags','+faststart','-metadata','title=Bloom + Agent Quota — One notch away',`out/${output}`]);
 console.log(output);
}
