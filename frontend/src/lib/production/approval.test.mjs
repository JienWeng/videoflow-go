import { test } from 'node:test';
import assert from 'node:assert/strict';
import { clipApproved } from './approval.mjs';

test('a clip is approved only by an approval note', () => {
  assert.equal(clipApproved({ approval: { status: 'approved', by: 'user' } }), true);
  assert.equal(clipApproved('{"approval":{"status":"approved"}}'), true);
  assert.equal(clipApproved({ self_check: { result: { verdict: 'pass' } } }), false);
  assert.equal(clipApproved(undefined), false);
  assert.equal(clipApproved('not json'), false);
});
