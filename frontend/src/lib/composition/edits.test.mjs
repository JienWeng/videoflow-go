import {test} from 'node:test';import assert from 'node:assert/strict';import {splitClip,duplicateClip,defaults,clipStyle} from './edits.mjs';
test('split conserves timing and source trim without changing original',()=>{const clip={id:'a',asset_id:'asset',start:2,trim:3,duration:6,speed:1};const result=splitClip(clip,4,'b');assert.deepEqual(result.map(c=>[c.start,c.trim,c.duration]),[[2,3,2],[4,5,4]]);assert.equal(clip.duration,6);assert.throws(()=>splitClip(clip,2,'b'));assert.throws(()=>splitClip(clip,8,'b'))});
test('duplicate deep clones and offsets clip',()=>{const clip={id:'a',start:1,duration:2,adjustments:defaults()};const c=duplicateClip(clip,'b');c.adjustments.scale=2;assert.equal(c.start,3);assert.equal(clip.adjustments.scale,1);assert.equal(c.id,'b')});
test('preview and renderer adjustment contract',()=>{assert.match(clipStyle({adjustments:{...defaults(),fit:'cover',scale:2}}),/object-fit:cover/);assert.match(clipStyle({}),/brightness\(1\)/)});
import {trimStart,trimEnd,contentEnd} from './edits.mjs';
{const clip={id:'a',start:2,trim:1,duration:4,speed:1};
 const left=trimStart(clip,3,24);assert.equal(left.start,3);assert.equal(left.trim,2);assert.equal(left.duration,3);
 const back=trimStart(clip,0,24);assert.equal(back.start,1);assert.equal(back.trim,0);assert.equal(back.duration,5);
 const tiny=trimStart(clip,10,24);assert.ok(tiny.duration>=1/24-1e-9&&tiny.start<6);
 assert.equal(trimEnd(clip,8,24).duration,6);assert.equal(trimEnd(clip,8,24,5).duration,5);assert.ok(trimEnd(clip,1,24).duration>0);
 assert.equal(contentEnd({clips:[clip],audio:[{start:0,duration:9}],captions:[{end:3}]}),9);
 console.log('Trim checks passed');}
