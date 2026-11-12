import assert from 'node:assert/strict';
import test from 'node:test';
import { makeOverview } from './overview.mjs';

const now = Date.parse('2026-11-12T10:00:00Z');
const snapshot = (extra = {}) => ({
  graph: { nodes: [], edges: [] }, assets: [], ops: [], jobs: [], sessions: [],
  aiHealth: { all_text_local: true, engines: [{ engine: 'codex', installed: true }] },
  appHealth: { missing_keys: [] }, ...extra
});
const session = { id: 'session', title: 'Story', engine: 'codex', status: 'ready', selected: true };

test('counts saved project records and actual outputs, not successful job totals', () => {
  const model = makeOverview(snapshot({
    graph: { nodes: [{ type: 'scene' }, { type: 'character' }, { type: 'output', data: { video_path: 'storage/clip.mp4' } }, { type: 'output' }] },
    assets: [{ id: 'a' }], jobs: [{ id: 'j', status: 'succeeded' }, { id: 'j2', status: 'succeeded' }]
  }), now);
  assert.deepEqual(model.metrics, { scenes: 1, characters: 1, assets: 1, videos: 1 });
  assert.equal(model.storyEmpty, false);
});
test('empty story with saved characters still gets a first action', () => {
  const model = makeOverview(snapshot({ graph: { nodes: [{ type: 'character' }] } }), now);
  assert.equal(model.storyEmpty, true);
  assert.equal(model.primary.href, '/director');
});
test('missing engine and missing credentials are not described as ready', () => {
  const model = makeOverview(snapshot({ sessions: [session], aiHealth: { engines: [{ engine: 'codex', installed: false }] }, appHealth: { missing_keys: ['FAL_KEY'] } }), now);
  assert.equal(model.primary.label, 'Set up AI');
  assert(model.attention.some(item => item.id === 'media-setup'));
});
test('unknown optional setup status is different from missing setup', () => {
  const model = makeOverview(snapshot({ sessions: null, aiHealth: null, appHealth: null }), now);
  assert.equal(model.primary.label, 'Check AI setup');
  assert(model.attention.some(item => item.id === 'ai-unknown'));
  assert.equal(model.attention.some(item => item.id === 'ai-setup'), false);
});
test('recent activity is sorted and deduplicated without inventing timestamps', () => {
  const job = { id: 'j', status: 'running', created_at: '2026-11-12T09:50:00Z' };
  const model = makeOverview(snapshot({ jobs: [job, job], ops: [{ id: 'op', kind: 'video_generation', status: 'succeeded', created_at: '2026-11-12T09:30:00Z' }], assets: [{ id: 'a', name: 'Photo', created_at: 'invalid' }] }), now);
  assert.deepEqual(model.recent.map(item => item.id), ['job:j', 'op:op']);
  assert.equal(model.runningCount, 1);
});
test('uncertain work and pending reference reviews have recovery links, never retries', () => {
  const model = makeOverview(snapshot({ jobs: [{ id: 'j', status: 'reconciliation_required' }], assets: [{ id: 'a', metadata_json: { consistency_review: 'pending' } }] }), now);
  assert.equal(model.primary.href, '/render');
  assert(model.attention.some(item => item.id === 'reference-review' && item.href === '/assets'));
  assert.equal(model.attention.some(item => /retry|resubmit/i.test(item.label)), false);
});
test('old failures and failures followed by successful renders do not create alert fatigue', () => {
  const model = makeOverview(snapshot({ jobs: [
    { id: 'old', status: 'failed', created_at: '2026-10-01T00:00:00Z' },
    { id: 'failed', scene_id: 'scene', status: 'failed', created_at: '2026-11-12T08:00:00Z' },
    { id: 'success', scene_id: 'scene', status: 'succeeded', created_at: '2026-11-12T09:00:00Z' }
  ] }), now);
  assert.equal(model.attention.some(item => item.id === 'failed-renders'), false);
});
test('configured empty project can create a story; a populated project resumes Studio', () => {
  assert.equal(makeOverview(snapshot({ sessions: [session] }), now).primary.href, '/create');
  assert.equal(makeOverview(snapshot({ sessions: [session], graph: { nodes: [{ type: 'scene' }] } }), now).primary.href, '/?view=studio');
});
test('interrupted story generation links to its saved workflow', () => {
  const model = makeOverview(snapshot({ ops: [{ id: 'op', kind: 'video_generation', status: 'reconciliation_required' }] }), now);
  assert.equal(model.primary.href, '/create?op_id=op');
});
test('an older running story opens its own progress instead of the newest story', () => {
  const model = makeOverview(snapshot({ ops: [{ id: 'running-story', kind: 'video_generation', status: 'running' }] }), now);
  assert.equal(model.primary.href, '/create?op_id=running-story');
});
test('missing required graph data fails instead of showing false zero totals', () => {
  assert.throws(() => makeOverview(snapshot({ graph: null }), now), /project data/i);
});
