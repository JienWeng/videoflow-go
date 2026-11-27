import { test } from 'node:test';
import assert from 'node:assert/strict';
import { applyVoiceSound, canSpeedUp, captionTimingNote, voiceoverSummary } from './speech.mjs';

test('the caption note says which timing each caption uses', () => {
  assert.equal(captionTimingNote({ heard: 2, estimated: 0, voice: 0, changed: 0 }), 'Timing: 2 follow the speech heard in the clips.');
  assert.equal(
    captionTimingNote({ heard: 0, estimated: 3 }),
    'Timing: 3 use estimated shot timing (clips without their own sound, or no Listening key in AI setup).'
  );
  assert.equal(
    captionTimingNote({ heard: 1, estimated: 1, changed: 1 }),
    'Timing: 1 follows the speech heard in the clips, 1 uses estimated shot timing. 1 caption shows what was actually said, because it differs from the script.'
  );
  assert.equal(captionTimingNote({ voice: 2 }), 'Timing: 2 follow the voice lines.');
  assert.equal(captionTimingNote(undefined), '');
  assert.equal(
    captionTimingNote({ heard: 1, dropped: 1 }),
    "Timing: 1 follows the speech heard in the clips. 1 script line was left without a caption because it isn't heard in the video."
  );
});

test('replacing mutes the speaking clips, keeping restores them, music is lowered only when asked', () => {
  const clips = [
    { id: 'c1', asset_id: 'output-1', volume: 1 },
    { id: 'c2', asset_id: 'output-2', muted: true },
    { id: 'c3', asset_id: 'asset-3' }
  ];
  const audio = [
    { id: 'm', asset_id: 'music-1', volume: 0.8 },
    { id: 'v', asset_id: 'voiceover_abc', volume: 1 }
  ];
  const replaced = applyVoiceSound(clips, audio, { mode: 'replace', native: ['c1'], mute: ['c1'], lowerMusic: true });
  assert.equal(replaced.clips[0].muted, true);
  assert.equal(replaced.clips[0].volume, 1, 'muted, never just lowered');
  assert.equal(replaced.clips[2], clips[2]);
  assert.equal(replaced.audio[0].volume, 0.25);
  assert.equal(replaced.audio[1].volume, 1, 'voice lines keep their volume');
  const kept = applyVoiceSound(clips, audio, { mode: 'keep', native: ['c2'], mute: [] });
  assert.equal(kept.clips[1].muted, false);
  assert.equal(kept.audio, audio);
});

test('the voice-over note explains muting, speed-ups and lines that do not fit', () => {
  const note = voiceoverSummary({
    items: [{}, {}], generated: 1, reused: 1, mode: 'replace', mute_clip_ids: ['c1'], too_long: [{ fits_with_speed_up: true }], shifted: 1
  });
  assert.match(note, /Added 2 voice lines \(1 new, 1 reused\)\./);
  assert.match(note, /Muted the own sound of 1 clip so only one voice speaks/);
  assert.match(note, /1 line is longer than the time the character speaks, so the next line starts later\./);
  assert.equal(canSpeedUp({ too_long: [{ fits_with_speed_up: true }] }), true);
  assert.equal(canSpeedUp({ too_long: [{ fits_with_speed_up: false }] }), false);
  assert.match(voiceoverSummary({ items: [], mode: 'keep', kept_native: 3 }), /Kept the clips' own voices for 3 lines\./);
  assert.match(voiceoverSummary({ items: [{}], generated: 0, sped_up: 1 }), /all reused, no charge.*Sped up 1 line slightly/);
});
