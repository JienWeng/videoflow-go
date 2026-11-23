import { test } from 'node:test';
import assert from 'node:assert/strict';
import { checkKey, checkName, engineName, findingLines, heardLines, needsAttention, readSelfCheck, selfCheckBadge, selfCheckKindFor } from './selfcheck.mjs';

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

test('the speech check reads from audio_check and lists each line as heard', () => {
  const qa = {
    self_check: record,
    audio_check: {
      kind: 'clip_audio', id: 'o1', hash: 'h', engine: 'openai', attempt: 1,
      result: {
        verdict: 'fix', score: 62, issues: [{ code: 'speech_cut_off', severity: 'high', detail: 'Speech cut off at 7.9 s: line 2 stops before its end.' }],
        retry: { worth_it: true, change: 'Give line 2 fewer words.' },
        audio: {
          lines: [
            { id: 'l1', text: '他从一万年后回来，', heard: '他從一萬年後回來', similarity: 1, start: 0.6, end: 2.4, status: 'ok' },
            { id: 'l2', text: '守住这台机器，不要让任何人靠近。', heard: '守住这台机器不要让任', similarity: 0.67, start: 4.2, end: 7.9, status: 'cut_off' },
            { id: 'l3', text: '走！', heard: '', similarity: 0, status: 'weird' }
          ],
          heard_language: 'Chinese', expected_language: 'Chinese', cost: 0.0008
        }
      }
    }
  };
  assert.equal(checkKey('clip_audio'), 'audio_check');
  assert.equal(checkKey('clip'), 'self_check');
  assert.equal(checkName('clip_audio'), 'Speech check');
  assert.equal(engineName('openai'), 'OpenAI');
  assert.equal(engineName('claude'), 'Claude Code');
  const look = readSelfCheck(qa);
  assert.equal(look?.score, 48, 'the frames check is still read by default');
  assert.equal(look?.audio, null);
  const c = readSelfCheck(qa, checkKey('clip_audio'));
  assert.equal(c?.score, 62);
  assert.equal(needsAttention(c), true);
  assert.deepEqual(heardLines(c), [
    'Line 1: said at 0.6–2.4 s (100% as written).',
    'Line 2: cut off at 4.2–7.9 s, heard “守住这台机器不要让任”.',
    'Line 3: not heard (“走！”).'
  ]);
  assert.equal(readSelfCheck({ self_check: record }, 'audio_check'), null);
  assert.deepEqual(heardLines(look), []);
});
