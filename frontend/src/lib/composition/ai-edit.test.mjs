import { test } from 'node:test';
import assert from 'node:assert/strict';
import { isPendingProposal, latestEdit, savedEditLabel, aiEditHeadline } from './ai-edit.mjs';

const plain = { revision: 'a', created_at: '2026-11-20T10:00:00Z' };
const proposal = { revision: 'b', created_at: '2026-11-21T10:00:00Z', ai_edit: { label: 'AI edit proposal', status: 'proposed', pass: 1 } };
const accepted = { revision: 'c', created_at: '2026-11-19T10:00:00Z', ai_edit: { label: 'AI edit proposal', status: 'accepted', pass: 2 } };

test('a pending AI proposal is never opened as the latest edit', () => {
  assert.equal(isPendingProposal(proposal), true);
  assert.equal(isPendingProposal(accepted), false);
  assert.equal(latestEdit([plain, proposal, accepted]).revision, 'a');
  assert.equal(latestEdit([proposal]), null);
});

test('saved AI edits say what they are and where they stand', () => {
  assert.equal(savedEditLabel(plain), '');
  assert.equal(savedEditLabel(proposal), 'AI edit proposal · waiting for you');
  assert.equal(savedEditLabel(accepted), 'AI edit proposal (refined by ear) · accepted');
});

test('every ending of an AI edit run has a plain headline', () => {
  for (const status of ['proposed', 'accepted', 'rejected', 'unchanged']) assert.ok(aiEditHeadline({ status }).length > 0);
  assert.match(aiEditHeadline({ status: 'accepted', video_url: '/x.mp4' }), /exported/);
  assert.equal(aiEditHeadline(null), '');
});
