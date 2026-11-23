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
