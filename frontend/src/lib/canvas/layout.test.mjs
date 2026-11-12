import assert from 'node:assert/strict';
import test from 'node:test';
import {layout,nodeSize} from './layout.ts';
test('orphan image rows reserve their card height plus spacing',()=>{
 const nodes=Array.from({length:12},(_,i)=>({id:'image-'+i,type:'entity',position:{x:0,y:0},data:{kind:'asset',label:'Reference '+i,file_path:'reference.png'}}));
 const placed=layout(nodes,[]);
 const first=placed.find(n=>n.id==='image-0'),next=placed.find(n=>n.id==='image-6');
 assert(next.position.y-first.position.y>=nodeSize(first).h+24,'Reference image rows overlap');
});
