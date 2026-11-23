import test from 'node:test';
import assert from 'node:assert/strict';
import { budgetText, fitBar, moveLine, sceneSpokenLines, secondsText, speakerLabel, spokenSummary } from './spoken.mjs';

const cast = [{ id: 'char_5b644ab6', name: 'Mei' }];

test('reads spoken lines from scene_json and ignores unreadable data', () => {
  assert.deepEqual(sceneSpokenLines({ scene_json: null }), []);
  assert.deepEqual(sceneSpokenLines({ scene_json: { spoken_lines: 'nope' } }), []);
  assert.deepEqual(
    sceneSpokenLines({
      scene_json: {
        spoken_lines: [{ id: 's1_l1', character_id: 'char_5b644ab6', language: 'Chinese', text: '如果钱不需要靠银行来信任呢？', visible: true }, null]
      }
    }),
    [{ id: 's1_l1', character_id: 'char_5b644ab6', language: 'Chinese', text: '如果钱不需要靠银行来信任呢？', visible: true }]
  );
});

test('labels the speaker as a character, the narrator or an unknown character', () => {
  assert.equal(speakerLabel({ character_id: 'char_5b644ab6' }, cast), 'Mei');
  assert.equal(speakerLabel({ character_id: '' }, cast), 'Narrator');
  assert.equal(speakerLabel({ character_id: 'char_gone' }, cast), 'Unknown character');
});

test('summarizes an empty, one-line and multi-line script', () => {
  assert.equal(spokenSummary([], cast), '');
  const line = { id: 'a', character_id: 'char_5b644ab6', language: 'English', text: 'Hi', visible: true };
  assert.equal(spokenSummary([line], cast), 'Mei: Hi');
  assert.equal(spokenSummary([line, { ...line, character_id: '' }], cast), '2 lines · Mei: Hi');
});

test('moves a line up or down and ignores moves past the ends', () => {
  assert.deepEqual(moveLine(['a', 'b', 'c'], 2, -1), ['a', 'c', 'b']);
  assert.deepEqual(moveLine(['a', 'b', 'c'], 0, 1), ['b', 'a', 'c']);
  assert.deepEqual(moveLine(['a', 'b'], 0, -1), ['a', 'b']);
});

test('keeps a line\'s shot and start hint when they are saved', () => {
  const [line] = sceneSpokenLines({
    scene_json: { spoken_lines: [{ id: 'l1', character_id: '', language: 'Chinese', text: '你好', visible: false, shot_id: 'shot_1', start_hint: 0.5 }] }
  });
  assert.equal(line.shot_id, 'shot_1');
  assert.equal(line.start_hint, 0.5);
});

test('fit bars say how full a shot is and turn amber when tight and red when too long', () => {
  assert.deepEqual(fitBar(7.2, 8, 'ok'), { percent: 90, tone: 'ok', label: '7.2 s of 8 s' });
  assert.equal(fitBar(7.6, 8, 'tight').tone, 'tight');
  assert.deepEqual(fitBar(11.4, 8, 'too_long'), { percent: 100, tone: 'too_long', label: '11.4 s of 8 s' });
  assert.equal(fitBar(0, 3).tone, 'empty');
  assert.equal(fitBar(3.2, 3).tone, 'too_long');
  assert.equal(secondsText(3), '3 s');
});

test('writes a shot\'s target in its language\'s unit', () => {
  assert.equal(budgetText({ unit: 'characters', low: 23, high: 30 }), '23–30 characters');
  assert.equal(budgetText({ unit: 'words', low: 0, high: 1 }), 'about 1 words');
  assert.equal(budgetText(null), '');
});
