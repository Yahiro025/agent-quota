import json, subprocess
from pathlib import Path
import numpy as np

root=Path(__file__).resolve().parent.parent
def decode(p, filters=None, sr=22050):
    cmd=['ffmpeg','-v','error','-i',str(p),'-ac','1','-ar',str(sr)]
    if filters: cmd+=['-af',filters]
    return np.frombuffer(subprocess.check_output(cmd+['-f','f32le','-']),dtype=np.float32)

sr=22050
y=decode(root/'public/audio/house-vibez.mp3','highpass=f=40,lowpass=f=160')
hop=110
n=len(y)//hop
rms=np.sqrt(np.mean(y[:n*hop].reshape(n,hop)**2,axis=1))
env=np.maximum(0,rms-np.roll(rms,2))
times=np.arange(n)*hop/sr
mask=(times>2)&(times<65)
# Fit an equally spaced grid to the low-frequency attack envelope, then
# refit its matched physical transients. Analysis edits downloaded audio only.
best=None
for bpm in np.arange(120,126,.01):
    period=60/bpm
    for phase in np.arange(0,period,.005):
        ts=np.arange(phase,65,period)
        ts=ts[(ts>2)&(ts<65)]
        ix=np.clip(np.rint(ts*sr/hop).astype(int),0,n-1)
        score=float(np.mean(env[ix]))
        if best is None or score>best[0]:best=(score,float(bpm),float(phase))
_,bpm,phase=best
period=60/bpm
grid=np.arange(phase,65,period)
matches=[]
for k,t in enumerate(grid):
    if t<2:continue
    ix=np.where(np.abs(times-t)<.045)[0]
    j=ix[np.argmax(env[ix])]
    matches.append((k,float(times[j])))
ks=np.array([x[0] for x in matches]); ts=np.array([x[1] for x in matches])
period,phase=np.polyfit(ks,ts,1);bpm=60/period
residual=ts-(phase+ks*period)
ratio=120/bpm
out=root/'public/audio/music-bed.wav'
subprocess.check_call(['ffmpeg','-v','error','-y','-i',str(root/'public/audio/house-vibez.mp3'),'-af',f'atrim=start={phase:.8f},asetpts=PTS-STARTPTS,atempo={ratio:.9f}', '-t','90','-ar','48000','-ac','2',str(out)])
raw=decode(out,sr=48000)
bands=[]
for f in range(2700):
    chunk=raw[f*1600:f*1600+2048]
    chunk=np.pad(chunk,(0,max(0,2048-len(chunk))))
    spec=np.abs(np.fft.rfft(chunk*np.hanning(2048)))
    hz=np.fft.rfftfreq(2048,1/48000)
    bands.append([float(np.sqrt(np.mean(spec[(hz>=lo)&(hz<hi)]**2))) for lo,hi in [(40,160),(160,500),(500,1800),(1800,5000),(5000,14000)]])
ba=np.array(bands);normal=np.percentile(ba,95,axis=0)
ba=np.clip(ba/np.maximum(normal,1e-6),0,1)
(root/'src/audio-bands.json').write_text(json.dumps(np.round(ba,4).tolist()))
peaks={}
for p in (root/'public/audio').glob('*.mp3'):
    if p.name=='house-vibez.mp3':continue
    a=decode(p,sr=48000)
    peaks[p.name]={'peak_seconds':float(np.argmax(np.abs(a))/48000),'peak_frame_30fps':int(round(np.argmax(np.abs(a))/48000*30)),'peak_dbfs':float(20*np.log10(max(1e-10,np.max(np.abs(a)))))}
report={'source_bpm':float(bpm),'source_phase_seconds':float(phase),'target_bpm':120,'atempo':float(ratio),'fit_mean_abs_ms':float(np.mean(np.abs(residual))*1000),'fit_max_abs_ms':float(np.max(np.abs(residual))*1000),'grid_transients_within_20ms':float(np.mean(np.abs(residual)<.02)),'method':'Low-band 40-160Hz attack-grid search and least-squares transient refit; 22050Hz, 110-sample hop. Downloaded music trimmed and tempo-adjusted without synthesis.','sfx':peaks}
(root/'audio-analysis.json').write_text(json.dumps(report,indent=2))
print(json.dumps(report,indent=2))
