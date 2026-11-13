import {FULL_SHOTS,FULL_SFX,FULL_TOTAL} from './FullTimeline';
import {FULL_UNITS,FullFilm} from './FullFilm';
export const WORKBENCH={
 name:'Bloom + Agent Quota · One notch away',fps:30,width:1920,height:1080,total:FULL_TOTAL,background:'#070708',revision:'3',
 shots:FULL_SHOTS.map((s,i)=>({...s,component:FULL_UNITS[i],props:{title:s.title},schema:[{type:'textarea',key:'title',label:'Title',default:s.title}]})),
 transitions:[],captions:[],overlays:[],
 sfx:FULL_SFX.map(s=>({...s,src:'audio/'+s.src})),
 bgm:[{from:0,duration:FULL_TOTAL,src:'audio/music-workbench.wav',volume:.32}],
 original:FullFilm,
};
