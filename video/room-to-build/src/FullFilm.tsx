import React from 'react';
import {AbsoluteFill,Audio,Easing,Freeze,Img,Sequence,interpolate,spring,staticFile,useCurrentFrame} from 'remotion';
import {FontStyle,Stage,Title} from './Film';
import {DigitRoll} from './lib/DigitRoll';
import layoutData from '../public/full/layout.json';
import quotaLayoutData from '../public/textures/layout.json';
import bands from './audio-bands.json';
import {FULL_SHOTS,FULL_SFX,FULL_TOTAL} from './FullTimeline';
const C={extrapolateLeft:'clamp',extrapolateRight:'clamp'} as const;
const smooth=Easing.bezier(.4,0,.6,1),crane=Easing.bezier(.2,0,.25,1),move=Easing.bezier(.75,0,.15,1);
const r=(f:number,a:number,b:number,e=smooth)=>interpolate(f,[a,b],[0,1],{...C,easing:e});
const m=(a:number,b:number,p:number)=>a+(b-a)*p;
const layouts=layoutData as Record<string,{width:number;height:number;elements?:Record<string,{x:number;y:number;w:number;h:number}>}>;
const plateH=(src:string,w:number)=>w*(layouts[src]?.height??190)/(layouts[src]?.width??380);
const Picture:React.FC<{src:string;x:number;y:number;w:number;opacity?:number;rotY?:number;rotX?:number;rotZ?:number;depth?:number}>=({src,x,y,w,opacity=1,rotY=0,rotX=0,rotZ=0,depth=0})=>{
 const image=src.startsWith('quota:')?'textures/'+src.slice(6)+'.png':'full/'+src+'.png';
 return <div style={{position:'absolute',left:x,top:y,width:w,opacity,transform:`perspective(2200px) rotateY(${rotY}deg) rotateX(${rotX}deg) rotateZ(${rotZ}deg)`,transformStyle:'preserve-3d',transformOrigin:'50% 50%'}}>
  {depth>0?[1,2,3].map(i=><Img key={i} src={staticFile(image)} style={{position:'absolute',left:0,top:i*depth,width:w,filter:'brightness(.2)',transform:`translateZ(${-i*depth}px)`}}/>):null}
  <Img src={staticFile(image)} style={{position:'relative',display:'block',width:w,filter:'drop-shadow(0 28px 32px #0003)'}}/>
 </div>;
};
const Eyebrow:React.FC<{text:string;light?:boolean}>=({text,light=false})=><div style={{position:'absolute',left:112,top:67,fontSize:22,letterSpacing:4,color:light?'#6b696b':'#a19aa9',fontWeight:500}}>{text}</div>;
const Arrow:React.FC<{x:number;y:number;click:number;start?:number;fromX?:number;fromY?:number;scale?:number}>=({x,y,click,start=click-28,fromX=x+130,fromY=y+80,scale=1})=>{
 const f=useCurrentFrame(),p=r(f,start,click-6,Easing.bezier(.16,1,.3,1)),pf=f-click;
 const opacity=r(f,start-5,start+5)*(1-r(f,click+22,click+36));
 return <svg width={42} height={42} viewBox="0 0 24 24" style={{position:'absolute',left:m(fromX,x,p)-Math.sin(p*Math.PI)*16,top:m(fromY,y,p),opacity,transform:`scale(${scale*(pf>=0&&pf<4?.84:1)})`,transformOrigin:'0 0',filter:'drop-shadow(0 2px 3px #0006)'}}><path d="M4.5 2.5 L4.5 19.2 L8.9 15 L11.9 21.6 L14.9 20.3 L11.9 13.8 L18.2 13.4 Z" fill="#141418" stroke="#fff" strokeWidth="1.6" strokeLinejoin="round"/></svg>;
};
const Halo:React.FC<{light?:boolean;amount?:number}>=({light=false,amount=1})=><svg width="1920" height="1080" style={{position:'absolute',inset:0,opacity:amount}}><ellipse cx="1040" cy="840" rx="1000" ry="420" fill="none" stroke={light?'#d7d5cf':'#ffffff10'} strokeWidth="1"/><ellipse cx="1040" cy="840" rx="850" ry="350" fill="none" stroke={light?'#e2e0da':'#ffffff08'} strokeWidth="1"/></svg>;

export const NotchHook:React.FC<{title?:string}>=({title=FULL_SHOTS[0].title})=>{
 const f=useCurrentFrame(),p=r(f,24,126,crane),w=m(3100,800,p),h=w*36/140;
 const x=m(-480,560,p),y=m(300,475,p),angle=m(-26,0,p);
 return <AbsoluteFill><Stage/><Halo amount={r(f,70,145)*.4}/>
 <div style={{position:'absolute',left:x,top:y,width:w,height:h,transform:`perspective(2300px) rotateY(${angle}deg) rotateX(${m(35,0,p)}deg)`,borderRadius:80,boxShadow:'0 45px 80px #0009,0 0 0 2px #ffffff18',background:'#101012'}}>
 <Img src={staticFile('full/rest.png')} style={{width:'100%',position:'absolute',top:-6,left:0}}/>
 </div>
 <Title text={title} center top={110} size={108} delay={108}/>
 <div style={{opacity:r(f,158,190)}}>{['music','calendar','controls'].map((src,i)=><Picture key={src} src={src} x={130+i*580} y={760} w={500} rotY={(i-1)*-10} rotX={10}/>)}</div>
 <Eyebrow text="BLOOM + AGENT QUOTA"/>
 </AbsoluteFill>;
};

export const MusicWorld:React.FC<{title?:string}>=({title=FULL_SHOTS[1].title})=>{
 const f=useCurrentFrame(),arrive=r(f,0,60,crane),land=r(f,80,140),barLand=r(f,100,145),b=bands[Math.min(2699,f+210)]??[.4,.5,.6,.7,.4];
 const x=780,y=420,w=1050,s=w/340;
 return <AbsoluteFill><Stage/><Halo amount={.5}/><Title text={title} top={100} size={96} center/>
 <Picture src={f<155?'music':f<205?'music-compact':'music-paused'} x={x} y={y+m(100,0,arrive)} w={w} rotY={m(-28,0,arrive)} rotX={m(12,0,arrive)} depth={3}/>
 {f<150?<div style={{position:'absolute',left:x+12*s,top:y+11*s,width:98*s,height:98*s,background:'#000',borderRadius:20}}/>:null}
 {f<150?<Img src={staticFile('album-art.svg')} style={{position:'absolute',left:m(165,x+12*s,land),top:m(390,y+11*s,land),width:m(460,98*s,land),borderRadius:m(40,18,land),transform:`perspective(1300px) rotateY(${m(-24,0,land)}deg) rotateZ(${m(-5,0,land)}deg)`,filter:'drop-shadow(0 35px 35px #0008)'}}/>:null}
 {f<155?<div style={{position:'absolute',left:m(840,x+298*s,barLand),top:m(850,y+13*s,barLand),width:m(870,30*s,barLand),height:m(190,18*s,barLand),display:'flex',gap:m(30,4,barLand),alignItems:'center'}}>{b.map((v,i)=><div key={i} style={{flex:1,height:`${m(12+v*88,18+v*82,barLand)}%`,borderRadius:20,background:barLand>.7?'#fff':`rgb(${255-i*4},${225-i*9},${185-i*14})`,boxShadow:barLand>.7?'none':'inset 0 1px 0 #ffffff9a'}}/>)}</div>:null}
 <Arrow x={970} y={607} click={155}/>
 <Arrow x={1305} y={795} click={205} fromX={1410} fromY={820}/>
 </AbsoluteFill>;
};

export const FocusWorld:React.FC<{title?:string}>=({title=FULL_SHOTS[2].title})=>{
 const f=useCurrentFrame(),p=r(f,30,112,crane),digits=r(f,85,115),src=f<160?'calendar':f<225?'timer-running':'timer-paused';
 return <AbsoluteFill><Stage light horizon={.6}/><Title text={title} light top={70} size={96} center/>
 <Picture src={src} x={360} y={m(680,320,p)} w={1200} rotX={m(30,0,p)} rotY={m(-12,0,p)} opacity={r(f,45,100)} depth={2}/>
 <div style={{position:'absolute',left:m(730,1080,p),top:m(390,535,p),fontFamily:'Inter',fontWeight:500,letterSpacing:-10,opacity:1-digits,transform:`scale(${m(1,.45,p)})`,transformOrigin:'0 0'}}><DigitRoll value="25:00" delay={15} fontSize={220} color="#141416"/></div>
 <Arrow x={1225} y={768} click={160}/><Arrow x={1225} y={768} click={225}/>
 </AbsoluteFill>;
};

export const ShelfWorld:React.FC<{title?:string}>=({title=FULL_SHOTS[3].title})=>{
 const f=useCurrentFrame(),p=r(f,5,42),s=2.65,x=800,y=375,collapse=r(f,165,190);const slots=layouts.shelf?.elements??{};
 return <AbsoluteFill><Stage/><Halo amount={.4}/><Title text={title} top={345} size={100}/>
 <Picture src="shelf-dropzone" x={x} y={y} w={380*s} opacity={p*(1-r(f,32,50))}/>
 <Picture src="shelf" x={x} y={y} w={380*s} opacity={r(f,32,50)*(1-collapse)}/>
 {f<125?<div style={{position:'absolute',left:x+24*s,top:y+46*s,width:332*s,height:126*s,background:'#000',opacity:r(f,32,50)}}/>:null}
 {[0,1,2].map(i=>{const at=56+i*6,u=spring({fps:30,frame:f-at,config:{damping:17,stiffness:95,mass:.9}}),q=Math.max(0,Math.min(1,u)),box=slots['tile'+i]??{x:26+i*124,y:48,w:116,h:120};const ex=x+box.x*s,ey=y+box.y*s;const sx=250+i*320,sy=810;const px=(1-q)**2*sx+2*(1-q)*q*((sx+ex)/2)+q*q*ex,py=(1-q)**2*sy+2*(1-q)*q*(Math.min(sy,ey)-220)+q*q*ey,iw=m(240,box.w*s,q),clip=Math.max(0,px+iw-(x+354*s))*r(f,80,120);return f<125?<Img key={i} src={staticFile(`full/shelf-tile${i}.png`)} style={{position:'absolute',left:px,top:py,width:iw,opacity:r(f,20+i*6,38+i*6),borderRadius:20,clipPath:`inset(0 ${clip}px 0 0)`,transform:`rotate(${m([-8,4,-3][i],0,q)}deg) scaleY(${f>at+43&&f<at+49?.97:1})`,filter:`drop-shadow(0 ${m(45,6,q)}px ${m(30,7,q)}px #0007)`}}/>:null;})}
 <Img src={staticFile('full/activities-all.png')} style={{position:'absolute',left:800,top:m(550,480,collapse),width:1000,opacity:collapse,filter:'drop-shadow(0 28px 32px #0003)'}}/>
 <div style={{position:'absolute',left:840,top:710,fontSize:32,color:'#b9b3c1',opacity:collapse}}>Music. Timer. Shelf. Still within reach.</div>
 </AbsoluteFill>;
};

export const ControlsWorld:React.FC<{title?:string}>=({title=FULL_SHOTS[4].title})=>{
 const f=useCurrentFrame(),p=r(f,0,68,crane),magnify=r(f,95,140),back=r(f,215,260);const zoom=m(1,1.15,magnify)*(1-.13*back);
 return <AbsoluteFill><Stage light horizon={.45}/><Title text={title} light top={330} size={88}/>
 <Picture src={f<142?'controls':'controls-volume'} x={m(1040,800,p)-(zoom-1)*400} y={250-(zoom-1)*200} w={1000*zoom} rotY={m(-48,0,p)} rotX={m(10,0,p)} rotZ={m(5,0,p)} depth={4}/>
 <Arrow x={1543} y={822} click={142} fromX={1370} fromY={822}/>
 </AbsoluteFill>;
};

export const ConnectionsWorld:React.FC<{title?:string}>=({title=FULL_SHOTS[5].title})=>{
 const f=useCurrentFrame(),p=r(f,100,145,move),v=(r(f+1,100,145,move)-r(f-1,100,145,move))*1920;
 return <AbsoluteFill><Stage light horizon={.4}/><Title text={title} light top={70} size={88} center/>
 <div style={{position:'absolute',inset:0,transform:`translateX(${-1920*p}px)`,filter:`blur(${Math.min(10,Math.abs(v)*.035)}px)`}}>
 <Picture src="wifi" x={505} y={290} w={910}/><Picture src="bluetooth" x={2425} y={290} w={910}/>
 </div>
 </AbsoluteFill>;
};

export const StatusWorld:React.FC<{title?:string}>=({title=FULL_SHOTS[6].title})=>{
 const f=useCurrentFrame(),scan=r(f,18,90,crane),swap=r(f,105,145,move),pull=r(f,175,220);const z=m(1.7,1,scan)*m(1,.7,pull);
 return <AbsoluteFill><Stage/><Halo amount={.6}/><Title text={title} top={80} size={96} center/>
 <div style={{position:'absolute',left:960-720*z,top:m(495,470,pull),width:1440*z,height:240*z,overflow:'hidden',borderRadius:28,transform:`perspective(1500px) rotateY(${m(-32,0,scan)}deg)`}}>
 <Img src={staticFile('full/weather-rail.png')} style={{position:'absolute',width:1440*z,left:-1440*swap*z,top:0}}/>
 <Img src={staticFile('full/hardware-rail.png')} style={{position:'absolute',width:1440*z,left:1440*(1-swap)*z,top:0}}/>
 </div>
 <div style={{position:'absolute',top:730,width:'100%',textAlign:'center',fontSize:38,color:'#b9b3c1',opacity:r(f,60,86)}}>{f<125?'Weather. Battery. At a glance.':'CPU. Memory. Storage. Network.'}</div>
 </AbsoluteFill>;
};

export const EdgesWorld:React.FC<{title?:string}>=({title=FULL_SHOTS[7].title})=>{
 const f=useCurrentFrame(),entry=r(f,0,42,crane),expand=r(f,64,100),bright=r(f,110,150,crane);
 return <AbsoluteFill><Stage light horizon={.5}/><Title text={title} light top={70} size={96} center/>
 <div style={{position:'absolute',left:m(-180,350,entry),top:350,width:m(126,594,expand),height:588,overflow:'hidden',borderRadius:30}}><Img src={staticFile(f<64?'full/volume.png':'full/mixer.png')} style={{width:f<64?126:594,position:'absolute',left:0,top:0}}/></div>
 <Picture src="brightness" x={m(1930,1440,bright)} y={350} w={126}/>
 <Arrow x={385} y={862} click={64}/>
 <div style={{position:'absolute',left:1060,top:560,width:320,fontSize:38,lineHeight:1.25,color:'#4f4d52',opacity:r(f,136,162)}}>Volume.<br/>Per-app mixing.<br/>Brightness.</div>
 </AbsoluteFill>;
};

export const QuotasWorld:React.FC<{title?:string}>=({title=FULL_SHOTS[8].title})=>{
 const f=useCurrentFrame(),p=r(f,0,62,crane),src=f<82?'overview':f<145?'codex':f<185?'clock':f<210?'groups-closed':'groups';const w=m(1200,1060,r(f,185,210)),alert=r(f,270,290),hover=f>=300;
 return <AbsoluteFill><Stage/><Title text={title} top={70} size={96} center/>
 <Picture src={'quota:'+src} x={960-w/2} y={340} w={w} rotY={m(30,0,p)} rotX={m(14,0,p)} depth={3} opacity={1-alert}/>
 {(()=>{const ql=quotaLayoutData['groups-closed'].elements.groupsToggle;const tx=430+(ql.x+ql.w*.5)*1060/486,ty=340+(ql.y+ql.h*.5)*1060/486;const frames=[54,76,120,139,156,179,190,204],cx=interpolate(f,frames,[1010,808,808,561,561,1414,1414,tx],{...C,easing:smooth}),cy=interpolate(f,frames,[700,577,577,846,846,577,577,ty],{...C,easing:smooth});return <svg width={42} height={42} viewBox="0 0 24 24" style={{position:'absolute',left:cx-8,top:cy-4,opacity:r(f,49,59)*(1-r(f,235,249)),transform:`scale(${[82,145,185,210].some(c=>f>=c&&f<c+4)?.84:1})`,transformOrigin:'0 0'}}><path d="M4.5 2.5 L4.5 19.2 L8.9 15 L11.9 21.6 L14.9 20.3 L11.9 13.8 L18.2 13.4 Z" fill="#141418" stroke="#fff" strokeWidth="1.6" strokeLinejoin="round"/></svg>})()}
 <Picture src={hover?'quota:urgent-hover':'quota:rest'} x={hover?510:750} y={470} w={hover?900:420} opacity={alert}/>
 {f>=270?<svg width={42} height={42} viewBox="0 0 24 24" style={{position:'absolute',left:m(1480,1000,r(f,270,299)),top:m(780,565,r(f,270,299)),opacity:alert}}><path d="M4.5 2.5 L4.5 19.2 L8.9 15 L11.9 21.6 L14.9 20.3 L11.9 13.8 L18.2 13.4 Z" fill="#141418" stroke="#fff" strokeWidth="1.6" strokeLinejoin="round"/></svg>:null}
 <div style={{position:'absolute',top:790,width:'100%',textAlign:'center',fontSize:40,color:'#b9b3c1',opacity:r(f,300,316)}}>Critical usage, revealed on hover.</div>
 </AbsoluteFill>;
};

const ensemble=['music','timer-running','shelf','controls','wifi','bluetooth','quota:overview','overview'];
export const OverviewWorld:React.FC<{title?:string}>=({title=FULL_SHOTS[9].title})=>{
 const f=useCurrentFrame();return <AbsoluteFill><Stage/><Title text={title} top={70} size={88} center/>
 {ensemble.map((src,i)=>{const p=r(f,16+i,42+i,move),out=r(f,60+i,88+i);const sx=120+(i%4)*450,sy=360+Math.floor(i/4)*315,sw=390,ex=790+(i%2)*20,ey=400+i*6;return <Picture key={src} src={src} x={m(sx,ex,p)} y={m(sy,ey,p)} w={m(sw,340,p)} rotY={m((i%4-1.5)*12,0,p)} opacity={1-out} depth={2}/>})}
 <Picture src="overview" x={575} y={365} w={770} opacity={r(f,78,104)}/>
 </AbsoluteFill>;
};

export const PersonalWorld:React.FC<{title?:string}>=({title=FULL_SHOTS[10].title})=>{
 const f=useCurrentFrame(),p=r(f,0,58,crane);return <AbsoluteFill><Stage light horizon={.5}/><Title text={title} light top={365} size={102}/>
 <Picture src={f<78?'settings-notch':'settings-notch-custom'} x={m(1220,900,p)} y={165} w={850} rotY={m(-35,0,p)} depth={3}/>
 <Arrow x={1660} y={855} click={78}/>
 <Picture src="music-compact" x={150} y={730} w={550} opacity={r(f,80,98)}/>
 <div style={{position:'absolute',left:112,top:560,width:550,fontSize:38,lineHeight:1.3,color:'#6a676e',opacity:r(f,48,74)}}>Notch modes. Media layouts.<br/>Status widgets. Edge controls.</div>
 </AbsoluteFill>;
};

export const BloomEnd:React.FC<{title?:string}>=({title=FULL_SHOTS[11].title})=>{
 const f=useCurrentFrame();return <AbsoluteFill><Stage light horizon={.5}/>
 {['music','calendar','shelf','controls'].map((src,i)=>{const p=r(f,0+i*2,40+i*2,move);return <Picture key={src} src={src} x={m([-450,350,1150,1850][i],880,p)} y={m(790,0,p)} w={m(450,160,p)} rotZ={m([-15,-6,6,15][i],0,p)} opacity={1-r(f,25,50)}/>})}
 <Picture src="rest" x={820} y={0} w={280} opacity={r(f,25,48)}/>
 <Img src={staticFile('bloom.svg')} style={{position:'absolute',left:889,top:250,width:142,height:142,opacity:r(f,8,32)}}/>
 <Title text="Bloom" light center top={430} size={128} delay={18}/>
 <Title text={title} light center top={620} size={78} delay={30}/>
 <div style={{position:'absolute',top:790,width:'100%',textAlign:'center',fontSize:38,color:'#6a676e',opacity:r(f,32,58)}}>With Agent Quota.</div>
 </AbsoluteFill>;
};

export const FULL_SCENES=[NotchHook,MusicWorld,FocusWorld,ShelfWorld,ControlsWorld,ConnectionsWorld,StatusWorld,EdgesWorld,QuotasWorld,OverviewWorld,PersonalWorld,BloomEnd];
const Transition:React.FC<{i:number;children:React.ReactNode}>=({i,children})=>{
 const f=useCurrentFrame(),p=r(f,0,20,move);const Previous=FULL_SCENES[Math.max(0,i-1)];let clip:string|undefined;
 if([1,8].includes(i))clip=`inset(${(1-p)*45}% ${(1-p)*35}% ${(1-p)*45}% ${(1-p)*35}% round ${30*(1-p)}px)`;
 if([3,6,10].includes(i))clip=`inset(0 ${(1-p)*100}% 0 0)`;
 if([4,7].includes(i))clip=`inset(0 0 0 ${(1-p)*100}%)`;
 return <AbsoluteFill>{i>0&&f<20?<Freeze frame={FULL_SHOTS[i-1].duration-1}><Previous/></Freeze>:null}<AbsoluteFill style={{clipPath:clip,opacity:clip||i===0?1:r(f,0,12)}}>{children}</AbsoluteFill></AbsoluteFill>;
};
export const FULL_UNITS=FULL_SCENES.map((Scene,i)=>{const Unit:React.FC<{title?:string}>=({title=FULL_SHOTS[i].title})=><AbsoluteFill style={{fontFamily:'Inter',background:'#070708'}}><FontStyle/><Transition i={i}><Scene title={title}/></Transition></AbsoluteFill>;Unit.displayName='FullShot_'+FULL_SHOTS[i].id;return Unit;});
export const FullAudio:React.FC<{bgm?:boolean}>=({bgm=true})=><>{bgm?<Audio src={staticFile('audio/music-bed.wav')} volume={f=>interpolate(f,[0,45,2400,2550,2650,2699],[0,.32,.32,.25,.25,0],C)}/>:null}{FULL_SFX.map((s,i)=><Sequence key={i} from={s.from} durationInFrames={s.duration} name={s.label}><Audio src={staticFile('audio/'+s.src)} volume={s.volume}/></Sequence>)}</>;
export const FullFilm:React.FC<{bgm?:boolean;sound?:boolean}>=({bgm=true,sound=false})=><AbsoluteFill style={{fontFamily:'Inter',background:'#070708'}}><FontStyle/>{FULL_SHOTS.map((s,i)=>{const Unit=FULL_UNITS[i];return <Sequence key={s.id} from={s.from} durationInFrames={s.duration} name={s.label}><Unit title={s.title}/></Sequence>})}{sound?<FullAudio bgm={bgm}/>:null}</AbsoluteFill>;
