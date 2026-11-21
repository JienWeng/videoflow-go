import assert from 'node:assert/strict';
import {editChanges,applyEditProposal,moveClip} from './proposals.mjs';
const draft={duration:10,fps:30,clips:[{id:'one',start:0,duration:2},{id:'two',start:4,duration:2}],audio:[],captions:[]};
const moved=moveClip(draft,'clips','one',2.08);assert.equal(moved.clips[0].start,2.066666666666667);assert.equal(draft.clips[0].start,0);
assert.equal(moveClip(draft,'clips','one',3.9).clips[0].start,4);assert.equal(moveClip(draft,'clips','one',20).clips[0].start,8);
assert.equal(editChanges(draft,moved).length,1);assert.deepEqual(applyEditProposal(draft,{base:JSON.stringify(draft),manifest:moved}),moved);assert.throws(()=>applyEditProposal(moved,{base:JSON.stringify(draft),manifest:moved}),/changed/);

assert.equal(moveClip(draft,"clips","one",1/30,false).clips[0].start,1/30);
