import assert from 'node:assert/strict';
import test from 'node:test';
import { sessionState } from './session-state.mjs';

const saved = { engine: 'codex', status: 'ready' };
const available = { all_text_local: true, engines: [{ engine: 'codex', installed: true }] };

test('a saved session with an unavailable engine requires setup', () => {
  const state = sessionState(saved, { all_text_local: true, engines: [{ engine: 'codex', installed: false }] });
  assert.equal(state.label, 'AI setup required');
  assert.equal(state.canContinue, false);
  assert.equal(state.needsSetup, true);
});
test('unknown engine status never implies readiness', () => {
  assert.equal(sessionState(saved, null).canContinue, false);
  assert.equal(sessionState(saved, null).label, 'Checking AI setup');
});
test('detected executable allows first use without claiming authenticated access', () => {
  const state = sessionState(saved, available);
  assert.equal(state.label, 'Session saved');
  assert.equal(state.canContinue, true);
  assert.match(state.help, /sign-in.*first prompt/i);
});
test('an existing native conversation is described as resumable', () => {
  assert.equal(sessionState({ ...saved, native_id: 'conversation' }, available).label, 'Session selected');
});
test('running and interrupted sessions keep their recovery gates', () => {
  assert.equal(sessionState({ ...saved, status: 'running' }, available).label, 'Planning in progress');
  assert.equal(sessionState({ ...saved, status: 'running' }, available).canContinue, false);
  assert.equal(sessionState({ ...saved, status: 'review_required' }, available).label, 'Review needed');
  assert.equal(sessionState({ ...saved, status: 'review_required' }, available).canContinue, false);
});
test('disabled local routing cannot be presented as usable', () => {
  const state = sessionState(saved, { ...available, all_text_local: false });
  assert.equal(state.canContinue, false);
  assert.equal(state.needsSetup, true);
});
