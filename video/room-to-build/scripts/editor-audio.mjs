import {execFileSync} from 'node:child_process';
// Flatten the authored bed envelope for the editor's constant-gain audio card.
const envelope="if(lt(t,1.5),t/1.5,if(lt(t,80),1,if(lt(t,85),1-(t-80)*0.04375,if(lt(t,88.333333),0.78125,max(0,0.78125*(89.966667-t)/1.633334)))))";
execFileSync('ffmpeg',['-v','error','-y','-i','public/audio/music-bed.wav','-af',`volume='${envelope}':eval=frame`,'-t','90','-ar','48000','-c:a','pcm_s24le','public/audio/music-workbench.wav']);
