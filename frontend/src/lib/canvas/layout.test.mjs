import assert from 'node:assert/strict';
import test from 'node:test';
import {layout,nodeSize} from './layout.ts';
test('orphan image rows reserve their card height plus spacing',()=>{
 const nodes=Array.from({length:12},(_,i)=>({id:'image-'+i,type:'entity',position:{x:0,y:0},data:{kind:'asset',label:'Reference '+i,file_path:'reference.png'}}));
 const placed=layout(nodes,[]);
 const first=placed.find(n=>n.id==='image-0'),next=placed.find(n=>n.id==='image-6');
 assert(next.position.y-first.position.y>=nodeSize(first).h+24,'Reference image rows overlap');
});

test('character reference cards reserve preview and action height',()=>{
 const nodes=Array.from({length:12},(_,i)=>({id:'character-'+i,type:'entity',position:{x:0,y:0},data:{kind:'character',label:'Character '+i,reference_preview_path:'reference.png'}}));
 const placed=layout(nodes,[]),first=placed.find(n=>n.id==='character-0'),next=placed.find(n=>n.id==='character-6');
 assert(nodeSize(first).h>=180,'Character controls/preview not reserved');
 assert(next.position.y-first.position.y>=nodeSize(first).h+24,'Character reference rows overlap');
});

test('story stages and character references use separate ordered bands',()=>{
 const node=(id,kind,data={})=>({id,type:'entity',position:{x:0,y:0},data:{kind,label:id,...data}});
 const nodes=[node('scene','scene',{scene_order:1}),node('last','shot',{shot_order:3}),node('first','shot',{shot_order:1}),node('middle','shot',{shot_order:2}),node('Pip','character',{reference_preview_path:'pip.png',reference_asset_ids_json:['ref']}),node('ref','asset',{file_path:'pip.png'}),node('board','asset',{file_path:'board.png'})];
 const edges=[...['last','first','middle','Pip','board'].map(target=>({source:'scene',target})),{source:'Pip',target:'ref'}];
 const placed=layout(nodes,edges),p=Object.fromEntries(placed.map(n=>[n.id,n.position]));
 assert(p.first.y<p.middle.y&&p.middle.y<p.last.y,'shots must follow shot_order');assert.equal(p.first.x,p.last.x);assert(p.Pip.y>p.last.y+nodeSize(nodes[1]).h);assert.equal(p.Pip.y,p.ref.y);assert(p.ref.x>p.Pip.x);assert(p.board.x>p.first.x);
 const shuffled=layout(nodes.slice().reverse(),edges.slice().reverse());assert.deepEqual(Object.fromEntries(shuffled.map(n=>[n.id,n.position])),p);
});

test('fresh cards avoid pinned and other fresh cards using full card heights',async()=>{
 const {layoutFresh}=await import('./layout.ts');
 const nodes=Array.from({length:5},(_,i)=>({id:String(i),type:'entity',position:{x:0,y:0},data:{kind:'character',label:String(i),reference_preview_path:'ref.png'}}));
 const saved={'0':{x:0,y:250}},placed=layoutFresh(nodes,[],saved);assert.deepEqual(placed[0].position,saved['0']);
 for(let i=0;i<placed.length;i++)for(let j=i+1;j<placed.length;j++){const a=placed[i],b=placed[j],as=nodeSize(a),bs=nodeSize(b);assert(!(a.position.x<b.position.x+bs.w&&a.position.x+as.w>b.position.x&&a.position.y<b.position.y+bs.h&&a.position.y+as.h>b.position.y),'cards overlap')}
});


test('render and output descendants remain in their scene stage columns',()=>{
 const nodes=[['s','scene',{scene_order:1}],['t','shot',{scene_id:'s',shot_order:1}],['r','render_job',{}],['o','output',{}]].map(([id,kind,data])=>({id,type:'entity',position:{x:0,y:0},data:{kind,...data}}));
 const p=Object.fromEntries(layout(nodes,[{source:'s',target:'t'},{source:'t',target:'r'},{source:'r',target:'o'}]).map(n=>[n.id,n.position]));assert(p.s.x<p.t.x&&p.t.x<p.r.x&&p.r.x<p.o.x);assert.equal(p.s.y,p.o.y);
});
