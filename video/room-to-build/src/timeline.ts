export const SHOTS = [
 {id:'question',from:0,duration:180,title:'How much room\ndo you have left?',label:'The question'},
 {id:'reveal',from:180,duration:210,title:'Seven agents.\nOne clear view.',label:'Seven agents'},
 {id:'windows',from:390,duration:210,title:'5-hour. Weekly.\nAll in view.',label:'Select and inspect'},
 {id:'resets',from:600,duration:210,title:"Know when\nyou’re back.",label:'Countdown and clock'},
 {id:'models',from:810,duration:210,title:'The detail\nbehind the number.',label:'Model breakdown'},
 {id:'attention',from:1020,duration:210,title:'A heads-up.\nBefore the limit.',label:'Critical usage'},
 {id:'refresh',from:1230,duration:210,title:'Refresh.\nStay in flow.',label:'Stale to fresh'},
 {id:'space',from:1440,duration:180,title:'Always there.\nOut of your way.',label:'Return to focus'},
 {id:'end',from:1620,duration:180,title:'Room to build.',label:'Agent Quota inside Bloom'},
] as const;
export const TOTAL=1800;
export const SFX=[
 {from:SHOTS[0].from+12,src:'swoosh-slow.mp3',volume:0.21,duration:70,label:'Ring emergence'},
 {from:SHOTS[1].from+12,src:'swoosh-slow.mp3',volume:0.30,duration:70,label:'Pullback to seven agents'},
 {from:SHOTS[2].from+70,src:'switch-click-quick.mp3',volume:0.65,duration:16,label:'Select Codex'},
 {from:SHOTS[3].from+70,src:'switch-click-quick.mp3',volume:0.55,duration:16,label:'Countdown to clock'},
 {from:SHOTS[3].from+144,src:'switch-click-quick.mp3',volume:0.45,duration:16,label:'Clock to countdown'},
 {from:SHOTS[4].from+66,src:'switch-click-quick.mp3',volume:0.55,duration:16,label:'Expand model groups'},
 {from:SHOTS[4].from+67,src:'swoosh-quick.mp3',volume:0.16,duration:24,label:'Model rows reveal'},
 {from:SHOTS[5].from+12,src:'impact-deep-whoosh.mp3',volume:0.24,duration:120,label:'Critical reading'},
 {from:SHOTS[5].from+95,src:'switch-click-quick.mp3',volume:0.55,duration:16,label:'Open urgent agent'},
 {from:SHOTS[6].from+76,src:'switch-click-quick.mp3',volume:0.55,duration:16,label:'Refresh readings'},
 {from:SHOTS[6].from+103,src:'swoosh-quick.mp3',volume:0.13,duration:24,label:'Fresh snapshot'},
 {from:SHOTS[7].from+46,src:'switch-click-quick.mp3',volume:0.42,duration:16,label:'Close panel'},
 {from:SHOTS[8].from+12,src:'swoosh-slow.mp3',volume:0.18,duration:70,label:'Brand resolution'},
];
