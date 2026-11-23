import { test } from 'node:test';
import assert from 'node:assert/strict';
import { findingLines, needsAttention, readSelfCheck, selfCheckBadge, selfCheckKindFor } from './selfcheck.mjs';

const record = {
  kind: 'clip', id: 'o1', hash: 'h', checked_at: '2026-11-23T10:00:00Z', attempt: 1, engine: 'codex',
  result: { verdict: 'fix', score: 48, issues: [{ code: 'a', severity: 'low', detail: 'Small blur.' }, { code: 'b', severity: 'high', detail: 'Mia is missing.' }], suggestion: 'Re-render.', retry: { worth_it: true, change: 'Keep Mia in frame.' }, note: 'Sound was not checked.' }
};

test('reads a record from metadata, qa notes or itself', () => {
  for (const raw of [record, { self_check: record, locked_traits: {} }]) {
    const c = readSelfCheck(raw);
    assert.equal(c?.verdict, 'fix');
    assert.equal(c?.score, 48);
    assert.equal(c?.change, 'Keep Mia in frame.');
    assert.equal(c?.accepted, false);
  }
  assert.equal(readSelfCheck(null), null);
  assert.equal(readSelfCheck({ consistency_review: 'ready' }), null);
  assert.equal(readSelfCheck({ self_check: { result: { verdict: 'great' } } }), null);
});

test('badge and attention follow verdict, keep and availability', () => {
  const c = readSelfCheck(record);
  assert.deepEqual(selfCheckBadge(c), { label: 'Needs a fix', tone: 'warn' });
  assert.equal(needsAttention(c), true);
  const kept = readSelfCheck({ ...record, accepted_at: '2026-11-23T11:00:00Z' });
  assert.equal(needsAttention(kept), false);
  assert.equal(selfCheckBadge(kept).label, 'Kept despite check');
  const failed = readSelfCheck({ ...record, result: { ...record.result, verdict: 'fail' } });
  assert.equal(selfCheckBadge(failed).tone, 'bad');
  const off = readSelfCheck({ result: { verdict: 'pass', score: 0, issues: [], unavailable: true, note: 'self-check unavailable: no session' } });
  assert.equal(needsAttention(off), false);
  assert.deepEqual(selfCheckBadge(off), { label: 'Check unavailable', tone: 'muted' });
  assert.deepEqual(selfCheckBadge(readSelfCheck({ result: { verdict: 'pass', score: 91.4, issues: [] } })), { label: 'Checked · 91', tone: 'ok' });
  assert.deepEqual(selfCheckBadge(null), { label: 'Not checked', tone: 'muted' });
});

test('findings list the most serious first', () => {
  assert.deepEqual(findingLines(readSelfCheck(record)), ['Mia is missing.', 'Small blur. (low)']);
  assert.deepEqual(findingLines(null), []);
});

test('only generated pictures have a check kind', () => {
  assert.equal(selfCheckKindFor('character_reference'), 'character_sheet');
  assert.equal(selfCheckKindFor('storyboard'), 'scene_picture');
  assert.equal(selfCheckKindFor('image'), '');
});
