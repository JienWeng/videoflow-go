import { test } from 'node:test';
import assert from 'node:assert/strict';
import { normalizeDecision, assumptionLine, isAsking, answerChips } from './decision.mjs';

test('a missing or broken decision reads as inform with nothing to show', () => {
  for (const raw of [undefined, null, 'act', { mode: 'maybe' }, { mode: 'ask' }, { mode: 'ask', question: '  ' }]) {
    const d = normalizeDecision(raw);
    assert.equal(d.mode, 'inform');
    assert.deepEqual(d.assumptions, []);
    assert.deepEqual(d.options, []);
    assert.equal(assumptionLine(raw), '');
    assert.equal(isAsking(raw), false);
    assert.deepEqual(answerChips(raw), []);
  }
});

test('assumptions become one muted line', () => {
  assert.equal(assumptionLine({ mode: 'act', assumptions: ['9:16 for TikTok', ' ', 'narration, slow piano', 3] }), 'Assumed: 9:16 for TikTok · narration, slow piano');
});

test('a question becomes quick replies that remember the answer', () => {
  const raw = { mode: 'ask', question: 'Which format?', key: 'aspect_ratio', options: [{ label: 'Vertical', value: '9:16' }, { label: 'Wide' }, { label: '' }, null] };
  assert.equal(isAsking(raw), true);
  assert.deepEqual(answerChips(raw), [
    { label: 'Vertical', send: '9:16', answer: { key: 'aspect_ratio', question: 'Which format?', label: 'Vertical', value: '9:16' } },
    { label: 'Wide', send: 'Wide', answer: { key: 'aspect_ratio', question: 'Which format?', label: 'Wide', value: 'Wide' } }
  ]);
});

test('options of a reply that does not ask are ignored', () => {
  assert.deepEqual(answerChips({ mode: 'act', question: 'Which?', options: [{ label: 'A' }] }), []);
  assert.equal(normalizeDecision({ mode: 'act', overruled: 'full_auto' }).overruled, 'full_auto');
});

import { rejectionKey, decisionLabel } from './decision.mjs';

test('a dismissed proposal is kept under its action and target', () => {
  assert.equal(rejectionKey({ action: 'render_scene', scene_id: 'scene_1a2b' }), 'rejected:render_scene:scene_1a2b');
  assert.equal(rejectionKey({ action: 'update_character', character_id: 'char 9' }), 'rejected:update_character:char_9');
  assert.equal(rejectionKey({ action: 'render_scenes', scene_ids: ['a', 'b'] }), 'rejected:render_scenes:project');
  assert.equal(rejectionKey({ action: 'generate_script' }), 'rejected:generate_script:project');
  for (const intent of [undefined, null, { action: 'unknown' }, { action: 'Bad Action' }]) assert.equal(rejectionKey(intent), '');
});

test('decision keys read as plain words', () => {
  const named = [{ id: 'char_1', name: 'Satoshi' }, { id: 'scene_1', title: 'Opening' }];
  assert.equal(decisionLabel('aspect_ratio'), 'Format');
  assert.equal(decisionLabel('voice:char_1', named), 'Voice · Satoshi');
  assert.equal(decisionLabel('voice:char_gone', named), 'Voice');
  assert.equal(decisionLabel('rejected:render_scene:scene_1', named), 'Turned down: render scene for Opening');
  assert.equal(decisionLabel('choice:which_scene_first'), 'Chose: which scene first');
  assert.equal(decisionLabel('something_else'), 'something else');
});
