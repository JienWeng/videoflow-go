import {test} from 'node:test';import assert from 'node:assert/strict';import {nodeTools} from './tools.mjs';
test('all six node types have canonical editing routes',()=>{for(const kind of ['character','scene','shot','asset','output','render_job'])assert(nodeTools(kind,'id',{scene_id:'scene',file_path:'storage/test.mp4'}).length>0)});
test('media tools seed the selected source and preserve IDs safely',()=>{assert.equal(nodeTools('output','clip/x')[0].href,'/?view=edit&source=output-clip%2Fx');assert.equal(nodeTools('asset','prop',{file_path:'a.png'})[1].href,'/?view=edit&source=prop');assert.equal(nodeTools('asset','blob',{file_path:'a.pdf'}).length,1)});

test('character tools open the dedicated character page',()=>{assert.equal(nodeTools('character','hero/x')[0].href,'/characters/hero%2Fx')});
test('a finished clip opens in the video editor or the Videos page',()=>{assert.deepEqual(nodeTools('output','o').map(t=>t.href),['/?view=edit&source=output-o','/render'])});
