import subprocess,json,re,hashlib
from pathlib import Path
from PIL import Image,ImageDraw
ROOT=Path('out'); Q=ROOT/'qa'/'final';Q.mkdir(parents=True,exist_ok=True)
files=[ROOT/'Bloom-One-Notch-Away.mp4',ROOT/'Bloom-One-Notch-Away-SFX.mp4']
def call(args):
    p=subprocess.run(args,capture_output=True,text=True)
    if p.returncode: raise RuntimeError(p.stderr)
    return p.stdout,p.stderr
report={'files':[]}
for path in files:
    meta=json.loads(call(['ffprobe','-v','error','-show_streams','-show_format','-of','json',str(path)])[0]);v=next(s for s in meta['streams'] if s['codec_type']=='video');a=next(s for s in meta['streams'] if s['codec_type']=='audio')
    h=call(['ffmpeg','-v','error','-i',str(path),'-map','0:v:0','-c','copy','-f','hash','-hash','sha256','-'])[0].strip()
    loud=call(['ffmpeg','-hide_banner','-i',str(path),'-af','loudnorm=I=-14:TP=-1:LRA=11:print_format=json','-f','null','NUL'])[1]
    measured=json.loads(re.findall(r'\{[^{}]+\}',loud)[-1]);item={'file':str(path),'sha256':hashlib.sha256(path.read_bytes()).hexdigest(),'pictureHash':h,'video':v,'audio':a,'duration':float(meta['format']['duration']),'loudness':measured}
    assert v['width']==1920 and v['height']==1080 and v['r_frame_rate']=='30/1' and int(v['nb_frames'])==2700 and v['codec_name']=='h264'
    assert v['pix_fmt']=='yuv420p' and v['color_space']=='bt709'
    assert a['codec_name']=='aac' and a['channels']==2 and a['sample_rate']=='48000' and abs(item['duration']-90)<.1
    assert float(measured['input_tp']) < -1
    if 'SFX' not in path.name: assert abs(float(measured['input_i'])+14)<.6
    report['files'].append(item)
assert report['files'][0]['pictureHash']==report['files'][1]['pictureHash'];report['identicalPicture']=True
call(['ffmpeg','-v','error','-y','-i',str(files[0]),'-vf','fps=1/2',str(Q/'frame-%02d.png')])
frames=sorted(Q.glob('frame-*.png')); assert len(frames)==45
for page in range(3):
    canvas=Image.new('RGB',(1440,1500),(30,30,30));d=ImageDraw.Draw(canvas)
    for i,f in enumerate(frames[page*15:(page+1)*15]):
        x=(i%3)*480;y=(i//3)*300;canvas.paste(Image.open(f).convert('RGB').resize((480,270)),(x,y));d.text((x+10,y+276),f'{(page*15+i)*2:02d}s / frame {(page*15+i)*60}',fill='white')
    canvas.save(Q/f'contact-{page+1}.jpg',quality=94)
events=call(['ffmpeg','-hide_banner','-i',str(files[0]),'-vf','blackdetect=d=0.033:pix_th=0.02:pic_th=0.999,freezedetect=n=-60dB:d=4','-an','-f','null','NUL'])[1]
report['blackEvents']=re.findall(r'black_start:[^\r\n]+',events);report['freezeEvents']=re.findall(r'freeze_(?:start|duration|end):[^\r\n]+',events)
report['sampledFrames']=len(frames);report['visualReview']='Contact sheets and individual interaction boundaries require human/model visual inspection; detection alone is not acceptance.'
(ROOT/'qa-report.json').write_text(json.dumps(report,indent=2));print(json.dumps({k:v for k,v in report.items() if k!='files'},indent=2));print([(p['file'],p['loudness']['input_i'],p['loudness']['input_tp']) for p in report['files']])
