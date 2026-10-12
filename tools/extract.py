#!/usr/bin/env python3
"""Convert FFDec's XML/resource export into the Go runtime's lossless timeline data.
Usage: python extract.py input.xml exported-assets exported-texts output-assets
Requires Pillow and CairoSVG for static text rasterization only.
"""
import base64, copy, hashlib, json, math, shutil, sys
from pathlib import Path
import xml.etree.ElementTree as ET
import cairosvg
from PIL import Image
from fontTools.ttLib import TTFont

xml, exported, texts, out = map(Path, sys.argv[1:5])
out.mkdir(parents=True, exist_ok=True)
root = ET.parse(xml).getroot()
hires=Path(sys.argv[5]) if len(sys.argv)>5 else None

def integer(x, k, d=0): return int(x.attrib.get(k, d))
def flag(x,k): return x.attrib.get(k)=='true'
def rect(x):
    if x is None:return {'X':0,'Y':0,'W':0,'H':0}
    return {'X':integer(x,'Xmin')/20,'Y':integer(x,'Ymin')/20,'W':(integer(x,'Xmax')-integer(x,'Xmin'))/20,'H':(integer(x,'Ymax')-integer(x,'Ymin'))/20}
def matrix(x):
    m={'A':1.,'B':0.,'C':0.,'D':1.,'TX':0.,'TY':0.}
    if x is not None:
        if flag(x,'hasScale'):m.update(A=float(x.attrib.get('scaleX',1)),D=float(x.attrib.get('scaleY',1)))
        if flag(x,'hasRotate'):m.update(B=float(x.attrib.get('rotateSkew0',0)),C=float(x.attrib.get('rotateSkew1',0)))
        m.update(TX=integer(x,'translateX')/20,TY=integer(x,'translateY')/20)
    return m
def color(x):
    c={'Mul':[1.,1.,1.,1.],'Add':[0.,0.,0.,0.]}
    if x is not None:
        if flag(x,'hasMultTerms'):c['Mul']=[integer(x,v+'MultTerm',256)/256 for v in ['red','green','blue','alpha']]
        if flag(x,'hasAddTerms'):c['Add']=[integer(x,v+'AddTerm')/255 for v in ['red','green','blue','alpha']]
    return c
def code(x):return base64.b64encode(bytes.fromhex(x.attrib['actionBytes'])).decode()
def clips(x):
    result=[]
    for y in x.findall('./clipActions/clipActionRecords/item'):
        f=y.find('eventFlags')
        for k,event in [('clipEventLoad','load'),('clipEventEnterFrame','enterFrame')]:
            if flag(f,k):result.append({'event':event,'code':code(y)})
    return result

variants={}
variant_keys={}
def sound_event(source, info):
    if info is None:return source
    envelope=[{'position':integer(e,'pos44'),'left':integer(e,'leftLevel'),'right':integer(e,'rightLevel')} for e in info.findall('./envelopeRecords/item')]
    variant={'source':source,'in_point':integer(info,'inPoint'),'out_point':integer(info,'outPoint',-1),'loops':integer(info,'loopCount',1),'no_multiple':flag(info,'syncNoMultiple'),'envelope':envelope}
    if variant=={'source':source,'in_point':0,'out_point':-1,'loops':1,'no_multiple':False,'envelope':[]}:return source
    key=json.dumps(variant,sort_keys=True)
    if key not in variant_keys:
        vid=1000+len(variants);variants[vid]=variant;variant_keys[key]=vid
    return variant_keys[key]

def timeline(tags):
    frames=[];labels={};display={};actions=[];sounds=[];frame=1
    for x in tags:
        kind=x.attrib['type']
        if kind=='PlaceObject2Tag':
            d=integer(x,'depth');old=display.get(d)
            if not flag(x,'placeFlagMove') or old is None:
                p={'depth':d,'character':0,'name':'','matrix':matrix(None),'color':color(None),'ratio':0,'clip_depth':0,'born':frame}
            else:p=copy.deepcopy(old)
            if flag(x,'placeFlagHasCharacter'):
                if old and integer(x,'characterId') !=old['character']:p['born']=frame
                p['character']=integer(x,'characterId')
            if flag(x,'placeFlagHasName'):p['name']=x.attrib.get('name','')
            if flag(x,'placeFlagHasMatrix'):p['matrix']=matrix(x.find('matrix'))
            if flag(x,'placeFlagHasColorTransform'):p['color']=color(x.find('colorTransform'))
            if flag(x,'placeFlagHasRatio'):p['ratio']=integer(x,'ratio')
            if flag(x,'placeFlagHasClipDepth'):p['clip_depth']=integer(x,'clipDepth')
            if flag(x,'placeFlagHasClipActions'):p['clip_actions']=clips(x)
            display[d]=p
        elif kind=='RemoveObject2Tag':display.pop(integer(x,'depth'),None)
        elif kind=='DoActionTag':actions.append(code(x))
        elif kind=='StartSoundTag':sounds.append(sound_event(integer(x,'soundId'),x.find('soundInfo')))
        elif kind=='FrameLabelTag':labels[x.attrib['name']]=frame
        elif kind=='ShowFrameTag':
            frames.append({'display':[copy.deepcopy(display[d]) for d in sorted(display)],'actions':actions,'sounds':sounds})
            actions=[];sounds=[];frame+=1
    return frames,labels

def asset(path,bounds,dest=None):
    if dest is None:dest=Path(*path.parts[-2:])
    target=out/dest;target.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(path,target)
    return {'path':str(dest),'bounds':bounds,'scale':2}

movie={'sha256':'0c3d410285d69a36022f652b0c28e5c5f6d56b6112d6c77500c9b09387122eeb','Width':550,'Height':400,'fps':18,'characters':{},'fonts':{},'sounds':{},'opcodes':[]}
frames,labels=timeline(root.find('tags'))
movie['characters'][0]={'id':0,'kind':'sprite','frames':frames,'labels':labels,'bounds':{'X':0,'Y':0,'W':550,'H':400}}
for x in root.iter('item'):
    kind=x.attrib.get('type','')
    if kind=='DefineSpriteTag':
        cid=integer(x,'spriteId');frames,labels=timeline(x.find('subTags'))
        movie['characters'][cid]={'id':cid,'kind':'sprite','frames':frames,'labels':labels}
    elif kind in ['DefineShapeTag','DefineShape2Tag','DefineShape3Tag']:
        cid=integer(x,'shapeId');bounds=rect(x.find('shapeBounds'))
        a=asset(exported/'shapes'/f'{cid}.png',bounds)
        if hires and max(bounds['W'],bounds['H'])<180:
            a=asset(hires/f'{cid}.png',bounds,Path('shapes')/f'{cid}.png');a['scale']=8
        elif hires and cid != 1:
            # Keep large scene artwork sharp at the default Retina window size.
            # The oversized preloader fill (shape 1) needs no higher resolution.
            im=Image.open(hires/f'{cid}.png')
            im.resize((math.ceil(im.width/2),math.ceil(im.height/2)),Image.Resampling.LANCZOS).save(out/a['path'])
            a['scale']=4
        movie['characters'][cid]={'id':cid,'kind':'image','asset':a,'bounds':bounds}
    elif kind=='DefineTextTag':
        cid=integer(x,'characterID');bounds=rect(x.find('textBounds'))
        target=out/'texts'/f'{cid}.png';target.parent.mkdir(parents=True,exist_ok=True)
        svg=(texts/f'{cid}.svg').read_bytes()
        cairosvg.svg2png(bytestring=svg,write_to=str(target),scale=4)
        a={'path':f'texts/{cid}.png','bounds':bounds,'scale':4}
        movie['characters'][cid]={'id':cid,'kind':'image','asset':a,'bounds':bounds}
    elif kind=='DefineEditTextTag':
        cid=integer(x,'characterID');bounds=rect(x.find('bounds'));c=x.find('textColor')
        t={'variable':x.attrib.get('variableName',''),'initial':x.attrib.get('initialText',''),'font':integer(x,'fontId'),'height':integer(x,'fontHeight')/20,'color':[integer(c,k,255) for k in ['red','green','blue','alpha']],'align':integer(x,'align'),'bounds':bounds,'multiline':flag(x,'multiline'),'word_wrap':flag(x,'wordWrap'),'leading':integer(x,'leading')/20,'margin_left':integer(x,'leftMargin')/20,'margin_right':integer(x,'rightMargin')/20,'indent':integer(x,'indent')/20}
        movie['characters'][cid]={'id':cid,'kind':'text','text':t,'bounds':bounds}
    elif kind=='DefineButton2Tag':
        cid=integer(x,'buttonId');states={}
        for state,label in [(1,'Up'),(2,'Over'),(4,'Down'),(8,'HitTest')]:
            states[state]=[]
            for y in x.findall('./characters/item'):
                if not flag(y,'buttonState'+label):continue
                states[state].append({'depth':integer(y,'placeDepth'),'character':integer(y,'characterId'),'matrix':matrix(y.find('placeMatrix')),'color':color(y.find('colorTransform')),'born':1})
        acts=[]
        for y in x.findall('./actions/item'):
            k=integer(y,'condKeyPress')
            if k:acts.append({'event':'key','key':k,'code':code(y)})
            for tag,event in [('condOverDownToOverUp','release'),('condOverUpToOverDown','press'),('condIdleToOverUp','rollOver'),('condOverUpToIddle','rollOut')]:
                if flag(y,tag):acts.append({'event':event,'key':0,'code':code(y)})
        movie['characters'][cid]={'id':cid,'kind':'button','states':states,'button_actions':acts}
    elif kind=='DefineBitsJPEG2Tag':
        cid=integer(x,'characterID');p=exported/'images'/f'{cid}.jpg'
        w,h=Image.open(p).size;bounds={'X':0,'Y':0,'W':w,'H':h};a=asset(p,bounds);a['scale']=1
        movie['characters'][cid]={'id':cid,'kind':'image','asset':a,'bounds':bounds}
    elif kind=='DefineMorphShapeTag':
        cid=integer(x,'characterId');start=rect(x.find('startBounds'));end=rect(x.find('endBounds'))
        ratios=[]
        for p in root.iter('item'):
            if p.attrib.get('type')=='PlaceObject2Tag' and integer(p,'characterId')==cid:ratios.append(integer(p,'ratio'))
        # FFDec exports occurrence order, one unique ratio per root/sprite frame.
        # This movie's sole morph shape appears in preloader frames 1..10.
        active=False
        for p in root.find('tags'):
            if p.attrib.get('type')=='PlaceObject2Tag' and integer(p,'depth')==2:
                if flag(p,'placeFlagHasCharacter'):active=integer(p,'characterId')==cid
                if active and flag(p,'placeFlagHasRatio'):ratios.append(integer(p,'ratio'))
        ratios=sorted(set(ratios));m={}
        paths=sorted((exported/'morphshapes'/str(cid)).glob('*.png'),key=lambda p:int(p.name.split('_')[0]))
        for ratio,p in zip(ratios,paths):
            t=ratio/65535;bounds={k:start[k]*(1-t)+end[k]*t for k in start}
            m[ratio]=asset(p,bounds,Path('morphshapes')/str(cid)/p.name)
        movie['characters'][cid]={'id':cid,'kind':'morph','morph':m,'bounds':start}
# The shop buttons have native press sounds, independent of release scripts.
for x in root.iter('item'):
    if x.attrib.get('type')!='DefineButtonSoundTag':continue
    d=movie['characters'][integer(x,'buttonId')];d['button_sounds']={}
    for i,event in enumerate(['rollOut','rollOver','press','release']):
        source=integer(x,f'buttonSoundChar{i}')
        if source:d['button_sounds'][event]=sound_event(source,x.find(f'buttonSoundInfo{i}'))
movie['sound_variants']=variants
# Fonts export filenames start with character ID.
for p in (exported/'fonts').glob('*.ttf'):
    cid=int(p.name.split('_')[0].split('.')[0]);dest=out/'fonts'/p.name;dest.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(p,dest);f=TTFont(dest)
    for tag in f.keys():
        if tag!='GlyphOrder':f[tag]
    f.save(dest);movie['fonts'][cid]=str(Path('fonts')/p.name)
for p in (exported/'sounds').glob('*.wav'):
    cid=int(p.stem)
    if cid<0:continue # empty stream header; all actual audio uses StartSound
    dest=out/'sounds'/p.name;dest.parent.mkdir(parents=True,exist_ok=True);shutil.copy2(p,dest);movie['sounds'][cid]=str(Path('sounds')/p.name)
opcodes=set()
for x in root.iter():
    if 'actionBytes' not in x.attrib:continue
    b=bytes.fromhex(x.attrib['actionBytes']);i=0
    while i<len(b):
        op=b[i];opcodes.add(op);i+=1
        if op>=128:n=int.from_bytes(b[i:i+2],'little');i+=2+n
movie['opcodes']=sorted(opcodes)
(out/'movie.json').write_text(json.dumps(movie,ensure_ascii=False,separators=(',',':')))
print('Characters',len(movie['characters']),'root frames',len(movie['characters'][0]['frames']),'layouts',sum(k.startswith('L') for k in movie['characters'][0]['labels']),'fonts',len(movie['fonts']),'sounds',len(movie['sounds']),'opcodes',len(opcodes))
