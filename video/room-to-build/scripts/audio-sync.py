import subprocess,json,wave,numpy as np
from pathlib import Path
def decode(path):
    raw=subprocess.check_output(['ffmpeg','-v','error','-i',str(path),'-f','f32le','-ac','1','-ar','48000','-'])
    return np.frombuffer(raw,dtype='<f4')
source=decode('public/audio/switch-click-quick.mp3')
mix=decode('out/audio-sfx.wav')
probe=source[:2400]; expected=[365,415,610,675,1102,1804,2032,2095,2135,2160,2538]
report=[]
for frame in expected:
    start=round(frame/30*48000); lo=max(0,start-2400); window=mix[lo:start+probe.size+2400]
    scores=np.correlate(window,probe,mode='valid'); actual=lo+int(np.argmax(scores)); offset=actual-start
    report.append({'cueFrame':frame,'measuredStartSeconds':actual/48000,'offsetSamples':offset,'offsetFrames':offset/1600,'pass':abs(offset)<=1600})
Path('out/audio-sync.json').write_text(json.dumps({'method':'Cross-correlation of downloaded click waveform against the rendered SFX-only PCM track; 48kHz mono, 50ms probe, +/-50ms search. MP3 decoder delay is represented in the decoded source.','date':'2026-10-26','renderPipeline':'Remotion 4.0.484 audio render, Chrome on Windows, 30 fps, PCM 48kHz stereo; no applied global offset.','probes':report},indent=2))
print(json.dumps(report,indent=2))
assert all(p['pass'] for p in report)
