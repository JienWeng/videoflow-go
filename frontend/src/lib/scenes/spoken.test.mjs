import test from 'node:test';
import assert from 'node:assert/strict';
import { moveLine, sceneSpokenLines, speakerLabel, spokenSummary } from './spoken.mjs';

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
