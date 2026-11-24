import {test} from 'node:test';import assert from 'node:assert/strict';import {normalizeChatIntent} from './intent.mjs';
test('legacy story actions become generate_script',()=>{assert.equal(normalizeChatIntent({action:'add_scene'}).action,'generate_script');assert.equal(normalizeChatIntent({action:'create_script'}).action,'generate_script')});
test('current actions are kept with their details',()=>{assert.deepEqual(normalizeChatIntent({action:'render_scene',scene_id:'s1'}),{action:'render_scene',scene_id:'s1'})});
test('removed actions restored from an older session become help, not a card',()=>{for(const action of['make_clip','caption','invented'])assert.equal(normalizeChatIntent({action,reply:'Old card',clip:{prompt:'dance'}}).action,'unknown')});
test('the all-characters picture action is a card',()=>{assert.equal(normalizeChatIntent({action:'generate_character_references',character_ids:['a','b']}).action,'generate_character_references')});
test('fit script to clips is a supported action',()=>{assert.equal(normalizeChatIntent({action:'fit_script',scene_id:'s1'}).action,'fit_script')});
test('repairing a clip\'s speech is a supported action',()=>{assert.equal(normalizeChatIntent({action:'repair_clip_speech',output_id:'o1'}).action,'repair_clip_speech')});
test('the AI edit is a supported action',()=>{assert.equal(normalizeChatIntent({action:'ai_edit',script_id:'s1'}).action,'ai_edit')});
