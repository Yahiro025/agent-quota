import React from 'react';
import {AbsoluteFill, Audio, Easing, Freeze, Img, Sequence, interpolate, staticFile, useCurrentFrame} from 'remotion';
import {PageCam, type CamKey} from './lib/PageCam';
import {SHOTS,SFX,TOTAL} from './timeline';
const WHITE='#f4f2f6',BLACK='#070708',STAGE='#efeee9',GREEN='#32d74b';
const CLAMP={extrapolateLeft:'clamp',extrapolateRight:'clamp'} as const;
const ease=Easing.bezier(0.4,0,0.6,1);
const swift=Easing.bezier(0.16,1,0.3,1);
const ramp=(f:number,a:number,b:number)=>interpolate(f,[a,b],[0,1],{...CLAMP,easing:ease});
const mix=(a:number,b:number,t:number)=>a+(b-a)*t;

export const FontStyle=()=> <style>{`@font-face{font-family:Inter;src:url('${staticFile('fonts/Inter-Regular.ttf')}');font-weight:400}@font-face{font-family:Inter;src:url('${staticFile('fonts/Inter-Medium.ttf')}');font-weight:500}@font-face{font-family:Inter;src:url('${staticFile('fonts/Inter-SemiBold.ttf')}');font-weight:600}`}</style>;

// An original sculpted stage; the reading surface itself stays matte black.
export const Stage:React.FC<{light?:boolean;horizon?:number}>=({light=false,horizon=0})=><AbsoluteFill style={{background:light?STAGE:BLACK,overflow:'hidden'}}>
 <svg width="1920" height="1080" viewBox="0 0 1920 1080" style={{position:'absolute',inset:0,opacity:horizon}}>
  <defs><linearGradient id={light?'silver':'graphite'} x1="0" y1="0" x2="0.7" y2="1"><stop stopColor={light?'#f8f7f3':'#252527'}/><stop offset=".44" stopColor={light?'#deddd7':'#111113'}/><stop offset="1" stopColor={light?'#efeee9':'#070708'}/></linearGradient></defs>
  <path d="M-200 1000 C280 620 590 1050 1050 750 C1450 480 1950 680 2160 1180 L2160 1400 L-200 1400Z" fill={`url(#${light?'silver':'graphite'})`}/>
  <path d="M-200 1000 C280 620 590 1050 1050 750 C1450 480 1950 680 2160 1180" fill="none" stroke={light?'#ffffff':'#ffffff16'} strokeWidth="1.5"/>
 </svg>
</AbsoluteFill>;

export const Title:React.FC<{text:string;light?:boolean;top?:number;left?:number;size?:number;center?:boolean;delay?:number}>=({text,light=false,top=102,left=112,size=100,center=false,delay=8})=>{
 const f=useCurrentFrame();const p=ramp(f,delay,delay+36);
 return <div style={{position:'absolute',left:center?0:left,top,width:center?1920:1000,fontFamily:'Inter',fontSize:size,fontWeight:600,letterSpacing:-size*.055,lineHeight:1.06,color:light?'#141416':WHITE,whiteSpace:'pre-line',textAlign:center?'center':'left',opacity:p,transform:`translateY(${(1-p)*28}px)`,filter:`blur(${(1-p)*5}px)`}}>{text}</div>;
};

const Camera:React.FC<{src:string;keys:CamKey[];children?:React.ReactNode}>=({src,keys,children})=><PageCam src={`textures/${src}-page.png`} pageH={1080} keys={keys} ease={ease}>{children}</PageCam>;
const key=(frame:number,cx:number,cy:number,zoom:number,rotY=0,rotX=0):CamKey=>({frame,cx,cy,zoom,rotY,rotX,persp:1800});
const Cutout:React.FC<{src:string;x:number;y:number;w:number;opacity?:number;radius?:number}>=({src,x,y,w,opacity=1,radius=0})=><Img src={staticFile(`textures/${src}.png`)} style={{position:'absolute',left:x,top:y,width:w,opacity,borderRadius:radius}}/>;
// Cursor timing and arrow geometry adapted from Shotcraft CursorFlyover.
const Pointer:React.FC<{x:number;y:number;click:number;fromX?:number;fromY?:number;start?:number}>=({x,y,click,fromX=x+80,fromY=y+80,start=click-24})=>{
 const f=useCurrentFrame();const p=interpolate(f,[start,click-6],[0,1],{...CLAMP,easing:swift});const pf=f-click;
 const press=pf>-2&&pf<8?(pf<1?ramp(pf,-2,1):1-ramp(pf,1,7)):0;
 const vis=ramp(f,start-6,start+3)*(1-ramp(f,click+20,click+32));
 const bow=Math.sin(Math.PI*p)*18;
 return <div style={{position:'absolute',left:mix(fromX,x,p)-bow,top:mix(fromY,y,p),opacity:vis,transform:`scale(${1-.16*press})`,transformOrigin:'4px 2px'}}>
  {pf>=0&&pf<18?<div style={{position:'absolute',width:22,height:22,left:-11,top:-11,border:'1px solid #f4f2f6',borderRadius:'50%',opacity:1-pf/18,transform:`scale(${.3+pf/18*1.2})`}}/>:null}
  <svg viewBox="0 0 24 24" width={16} height={16} style={{overflow:'visible',filter:'drop-shadow(0 1px 1px #0008)'}}><path d="M4.5 2.5 L4.5 19.2 L8.9 15 L11.9 21.6 L14.9 20.3 L11.9 13.8 L18.2 13.4 Z" fill="#0d1324" stroke="#fff" strokeWidth={1.6} strokeLinejoin="round"/></svg>
 </div>;
};

export const Question:React.FC<{title?:string}>=({title=SHOTS[0].title})=>{
 const f=useCurrentFrame();const arrival=ramp(f,0,86);const exit=ramp(f,145,180);
 const scale=mix(1.75,1,arrival)*(1-.90*exit);const x=mix(1770,1410,arrival)-631*exit;
 return <AbsoluteFill style={{overflow:'hidden'}}><Stage/>
  <div style={{position:'absolute',left:112,top:84,fontFamily:'Inter',fontSize:24,letterSpacing:4,color:'#a19aa9',opacity:ramp(f,10,40)*(1-exit)}}>AGENT QUOTA · INSIDE BLOOM</div>
  <div style={{opacity:1-exit}}><Title text={title} top={332} size={106} delay={30}/></div>
  <div style={{position:'absolute',left:x-450,top:90+exit*111,width:900,height:900,transform:`perspective(2000px) rotateY(${mix(mix(-55,-16,arrival),0,exit)}deg) rotateX(${mix(mix(24,7,arrival),0,exit)}deg) scale(${scale})`,transformOrigin:'center',filter:`drop-shadow(0 55px 70px #0008)`}}>
   <svg width="900" height="900" viewBox="0 0 900 900">
    <defs><linearGradient id="ring-edge" x1="0" y1="0" x2="1" y2="1"><stop stopColor="#55555b"/><stop offset=".25" stopColor="#222226"/><stop offset=".65" stopColor="#101012"/><stop offset="1" stopColor="#38383d"/></linearGradient></defs>
    <circle cx="450" cy="458" r="384" fill="none" stroke="#020203" strokeWidth="62"/>
    <circle cx="450" cy="450" r="384" fill="none" stroke="url(#ring-edge)" strokeWidth="46"/>
    <circle cx="450" cy="450" r="405.5" fill="none" stroke="#ffffff25" strokeWidth="1.5"/>
    <circle cx="450" cy="450" r="384" fill="none" stroke={GREEN} strokeWidth="46" strokeLinecap="round" strokeDasharray={`${2*Math.PI*384*.18} 3000`} transform="rotate(-90 450 450)"/>
   </svg>
   <Img src={staticFile('codex-mark.svg')} style={{position:'absolute',left:290,top:290,width:320,height:320,opacity:ramp(f,52,92)}}/>
  </div>
 </AbsoluteFill>;
};

export const Reveal:React.FC<{title?:string}>=({title=SHOTS[1].title})=>{
 const f=useCurrentFrame();return <AbsoluteFill><Stage light horizon={.55}/>
 <Camera src="overview" keys={[key(0,960,40,2.7),key(20,960,40,2.7),key(76,960,88,2.24),key(210,960,88,2.24)]}/>
 <Title text={title} light center size={96} top={108}/>
 </AbsoluteFill>;
};

export const Windows:React.FC<{title?:string}>=({title=SHOTS[2].title})=>{
 const f=useCurrentFrame();const p=ramp(f,2,48);return <AbsoluteFill><Stage horizon={.25}/>
 <div style={{position:'absolute',inset:0,transform:`translateX(${(1-p)*110}px)`,opacity:p}}>
 <Cutout src={f<70?'overview-row':'codex-row'} x={800} y={250} w={1016} radius={24}/>
 <div style={{position:'absolute',left:800,top:475,width:1065,height:335,background:'#000',borderRadius:24,overflow:'hidden'}}><Cutout src={f<70?'overview-detail':'codex-detail'} x={12} y={25} w={1073}/></div>
 <div style={{position:'absolute',left:0,top:0,transform:'translate(800px,250px) scale(2.2)'}}><Pointer x={166} y={26} click={70} fromX={95} fromY={74}/></div>
 </div><Title text={title} top={350} size={100}/>
 </AbsoluteFill>;
};

export const Resets:React.FC<{title?:string}>=({title=SHOTS[3].title})=>{
 const f=useCurrentFrame();const p=ramp(f,0,46);return <AbsoluteFill><Stage light horizon={.5}/>
 <div style={{position:'absolute',left:810,top:330+(1-p)*50,width:1000,height:455,background:'#000',borderRadius:32,overflow:'hidden',opacity:p,boxShadow:'0 35px 60px #0002'}}>
 <Cutout src={f>=70&&f<144?'clock-windows':'codex-windows'} x={15} y={28} w={1073}/>
 <div style={{position:'absolute',left:40,top:260,width:920,height:1,background:'#262628'}}/>
 <Cutout src={f>=70&&f<144?'clock-reset':'codex-reset'} x={72} y={302} w={660}/>
 <div style={{position:'absolute',left:72,top:302,transform:'scale(6)'}}><Pointer x={65} y={16} click={70} fromX={90} fromY={28}/><Pointer x={65} y={16} click={144} fromX={95} fromY={29}/></div>
 </div><Title text={title} light top={350} size={100}/>
 </AbsoluteFill>;
};

export const Models:React.FC<{title?:string}>=({title=SHOTS[4].title})=>{
 const f=useCurrentFrame();const opening=ramp(f,66,96);const p=ramp(f,0,48);
 return <AbsoluteFill><Stage horizon={.3}/>
 <div style={{position:'absolute',left:260,top:400+(1-p)*65,width:1400,height:mix(268,480,opening),borderRadius:28,background:'#000',overflow:'hidden',opacity:p,boxShadow:'0 30px 90px #0008'}}>
 <Cutout src={f<66?'models-closed':'models-open'} x={0} y={20} w={1400}/>
 <div style={{position:'absolute',left:0,top:20,transform:'scale(3.196)'}}><Pointer x={56} y={58} click={66} fromX={180} fromY={80}/></div>
 </div><Title text={title} top={90} size={96} center/>
 </AbsoluteFill>;
};

export const Attention:React.FC<{title?:string}>=({title=SHOTS[5].title})=>{
 const f=useCurrentFrame();const open=ramp(f,95,135);const move=ramp(f,10,36);const pf=f-95;
 return <AbsoluteFill><Stage/>
 <div style={{opacity:1-open}}>{f<36?<Cutout src="rest" x={1084} y={400} w={462}/>:<Cutout src="urgent-hover" x={820} y={400} w={990}/>}</div>
 <div style={{opacity:open,transform:`translateY(${(1-open)*40}px)`}}><Cutout src="urgent-open" x={810} y={270} w={1060}/></div>
 {f<125?<svg viewBox="0 0 24 24" width={42} height={42} style={{position:'absolute',left:mix(1750,1260,move),top:mix(790,510,move),opacity:ramp(f,5,12)*(1-ramp(f,108,125)),transform:`scale(${pf>-1&&pf<5?.84:1})`,filter:'drop-shadow(0 3px 3px #0008)'}}><path d="M4.5 2.5 L4.5 19.2 L8.9 15 L11.9 21.6 L14.9 20.3 L11.9 13.8 L18.2 13.4 Z" fill="#0d1324" stroke="#fff" strokeWidth="1.6" strokeLinejoin="round"/></svg>:null}
 <Title text={title} top={350} size={100}/>
 <div style={{position:'absolute',left:800,right:80,bottom:150,fontFamily:'Inter',fontSize:44,textAlign:'center',color:'#b9b3c1',opacity:ramp(f,36,60)*(1-open)}}>Critical usage, revealed on hover.</div>
 </AbsoluteFill>;
};

export const Refresh:React.FC<{title?:string}>=({title=SHOTS[6].title})=>{
 const f=useCurrentFrame();const update=ramp(f,104,126);
 return <AbsoluteFill><Stage light horizon={.5}/>
 <div style={{position:'absolute',left:800,top:330,width:1050,height:470,background:'#000',borderRadius:30,overflow:'hidden',boxShadow:'0 35px 60px #0002'}}>
 <Cutout src={f<104?'stale-detail':'fresh-detail'} x={12} y={20} w={1073}/>
 <div style={{position:'absolute',left:35,right:35,top:365,height:1,background:'#262628'}}/>
 <Cutout src="refresh-button" x={740} y={380} w={292}/>
 <div style={{position:'absolute',left:740,top:380,transform:'scale(4)'}}><Pointer x={48} y={15} click={76} fromX={10} fromY={-20}/></div>
 </div><Title text={title} light top={350} size={96}/>
 </AbsoluteFill>;
};

export const Space:React.FC<{title?:string}>=({title=SHOTS[7].title})=>{
 const f=useCurrentFrame();const close=ramp(f,62,108);const pull=ramp(f,62,128);const content=1-ramp(f,46,62);
 const w=mix(486,140,close),h=mix(266,36,close),s=mix(2.4,1.5,pull);
 return <AbsoluteFill><Stage light horizon={1}/>
 <div style={{position:'absolute',left:960-w*s/2,top:mix(110,0,pull),width:w*s,height:h*s,overflow:'hidden',borderRadius:`0 0 ${28*close}px ${28*close}px`,background:'#000',boxShadow:'0 14px 32px #0002'}}>
 <Img src={staticFile('textures/fresh.png')} style={{position:'absolute',left:(w-486)*s/2,top:0,width:486*s,opacity:content}}/>
 <Img src={staticFile('textures/rest.png')} style={{position:'absolute',left:(w-140)*s/2,top:0,width:140*s}}/>
 </div>
 {f<63?<div style={{position:'absolute',left:376.8,top:110,transform:'scale(2.4)'}}><Pointer x={449} y={48} click={46} fromX={405} fromY={98}/></div>:null}
 <div style={{opacity:ramp(f,60,96)}}><Title text={title} light center top={480} size={102} delay={50}/></div>
 </AbsoluteFill>;
};

export const End:React.FC<{title?:string}>=({title=SHOTS[8].title})=>{
 const f=useCurrentFrame();return <AbsoluteFill><Stage horizon={.5}/>
 <Img src={staticFile('textures/rest.png')} style={{position:'absolute',width:280,left:820,top:0}}/>
 <Title text="Agent Quota" center top={290} size={126} delay={12}/>
 <div style={{position:'absolute',top:455,width:'100%',textAlign:'center',fontFamily:'Inter',fontSize:44,color:'#a19aa9',opacity:ramp(f,24,58)}}>Inside Bloom.</div>
 <Title text={title} center top={662} size={88} delay={45}/>
 </AbsoluteFill>;
};

export const SCENES=[Question,Reveal,Windows,Resets,Models,Attention,Refresh,Space,End];
const Dissolve:React.FC<{children:React.ReactNode;previous:React.ReactNode;first:boolean}>=({children,previous,first})=>{const f=useCurrentFrame();return <AbsoluteFill>{!first&&f<16?<AbsoluteFill>{previous}</AbsoluteFill>:null}<AbsoluteFill style={{opacity:first?1:ramp(f,0,16)}}>{children}</AbsoluteFill></AbsoluteFill>};
export const Film:React.FC<{bgm?:boolean;sound?:boolean}>=({bgm=true,sound=false})=><AbsoluteFill style={{background:BLACK,fontFamily:'Inter'}}>
 <FontStyle/>
 {SHOTS.map((shot,i)=>{const Scene=SCENES[i],Previous=SCENES[Math.max(0,i-1)];return <Sequence key={shot.id} from={shot.from} durationInFrames={shot.duration} name={shot.label}><Dissolve first={i===0} previous={<Freeze frame={i?SHOTS[i-1].duration-1:0}><Previous/></Freeze>}><Scene title={shot.title}/></Dissolve></Sequence>})}
 {sound&&bgm?<Audio src={staticFile('audio/music-bed.wav')} volume={f=>interpolate(f,[0,45,1440,1605,1770,1799],[0,.32,.32,.25,.25,0],CLAMP)}/>:null}
 {sound?SFX.map((cue,i)=><Sequence key={i} from={cue.from} durationInFrames={cue.duration} name={cue.label}><Audio src={staticFile('audio/'+cue.src)} volume={cue.volume}/></Sequence>):null}
</AbsoluteFill>;
