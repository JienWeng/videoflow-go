import { sessionState } from '../director/session-state.mjs';

const timestamp = item => {
  const updated = Date.parse(item.updated_at ?? '');
  return Number.isFinite(updated) ? updated : Date.parse(item.created_at ?? '');
};
const unique = items => [...new Map(items.map(item => [item.id, item])).values()];
const statusLabel = value => ({ pending: 'Queued', running: 'In progress', succeeded: 'Completed', failed: 'Failed', reconciliation_required: 'Review needed' })[value] ?? 'Saved';
const kindLabel = value => ({ video_generation: 'Story generation', storyboard: 'Storyboard images', assets: 'Asset generation', shots: 'Shot planning', caption: 'Captions', style_ingest: 'Project style' })[value] ?? String(value || 'Project task').replaceAll('_', ' ');
const operationHref = op => op.kind === 'video_generation' ? `/create?op_id=${encodeURIComponent(op.id)}` : op.scene_id ? `/scenes?scene_id=${encodeURIComponent(op.scene_id)}` : ['assets', 'style_ingest'].includes(op.kind) ? '/assets' : op.kind === 'caption' ? '/render' : '/scenes';

export function makeOverview(snapshot, now = Date.now()) {
  if (!Array.isArray(snapshot.graph?.nodes) || !['assets', 'ops', 'jobs'].every(key => Array.isArray(snapshot[key]))) {
    throw new Error('Project data is incomplete. Refresh to try again.');
  }
  const nodes = snapshot.graph.nodes;
  const assets = unique(snapshot.assets), ops = unique(snapshot.ops), jobs = unique(snapshot.jobs);
  const sessions = snapshot.sessions ?? [];
  const selected = sessions.find(item => item.selected);
  const state = sessionState(selected, snapshot.aiHealth);
  const nodeByID = new Map(nodes.map(node => [node.id, node]));
  const metrics = {
    scenes: nodes.filter(node => node.type === 'scene').length,
    characters: nodes.filter(node => node.type === 'character').length,
    assets: assets.length,
    videos: nodes.filter(node => node.type === 'output' && node.data?.video_path).length
  };
  const storyEmpty = metrics.scenes === 0 && metrics.videos === 0;
  const attention = [];
  const issue = (id, label, detail, href, action, urgent = false) => ({ id, label, detail, href, action, urgent });
  const uncertainJobs = jobs.filter(job => job.status === 'reconciliation_required');
  const uncertainOps = ops.filter(op => op.status === 'reconciliation_required');
  if (uncertainJobs.length) attention.push(issue('uncertain-renders', 'Review an unfinished render request', 'Check existing provider work before starting another render.', '/render', 'Review renders', true));
  if (uncertainOps.length) attention.push(issue('uncertain-requests', 'Review an interrupted generation request', 'Open the saved workflow and review its status before starting again.', operationHref(uncertainOps[0]), 'Review request', true));
  if (selected?.status === 'review_required') attention.push(issue('session-review', 'Your AI session needs review', 'Review the interrupted request before continuing.', '/director', 'Review session', true));
  const pendingReferences = assets.filter(asset => asset.metadata_json?.consistency_review === 'pending');
  if (pendingReferences.length) attention.push(issue('reference-review', `${pendingReferences.length} ${pendingReferences.length === 1 ? 'reference needs' : 'references need'} approval`, 'Check the generated images before rendering the scene.', '/assets', 'Review references', true));
  const target = job => job.shot_id ? `shot:${job.shot_id}` : job.scene_id ? `scene:${job.scene_id}` : null;
  const failed = jobs.filter(job => job.status === 'failed' && now - timestamp(job) >= 0 && now - timestamp(job) < 86400000 && !jobs.some(later => later.status === 'succeeded' && target(job) && target(later) === target(job) && timestamp(later) > timestamp(job)));
  if (failed.length) attention.push(issue('failed-renders', `${failed.length} ${failed.length === 1 ? 'render failed' : 'renders failed'} in the last 24 hours`, 'Open the saved error and review the request before running again.', '/render', 'View renders', true));

  if (!snapshot.aiHealth || snapshot.sessions === null) {
    attention.push(issue('ai-unknown', 'AI setup status is unavailable', 'Check your session setup before planning a story.', '/director', 'Check AI setup'));
  } else if (!selected || state.needsSetup) {
    attention.push(issue('ai-setup', selected ? 'Your AI assistant needs setup' : 'Choose an AI session', selected ? state.help : 'Choose Codex or Claude Code for this project.', '/director', selected ? 'Set up AI' : 'Choose AI session'));
  }
  if (!snapshot.appHealth) attention.push(issue('media-unknown', 'Media setup status is unavailable', 'Check provider settings before generating images or video.', '/settings#connections', 'Check media setup'));
  else if (snapshot.appHealth.missing_keys?.length) attention.push(issue('media-setup', 'Provider credentials need setup', 'Add the required provider keys before generating media.', '/settings#connections', 'Set up providers'));

  let primary;
  const blocker = attention.find(item => item.urgent);
  const activeJob = jobs.find(job => ['running', 'pending'].includes(job.status));
  const activeOp = ops.find(op => ['running', 'pending'].includes(op.status));
  if (blocker) primary = { id: blocker.id, label: blocker.action, href: blocker.href, detail: blocker.detail };
  else if (activeJob || activeOp) primary = { id: 'progress', label: 'View progress', href: activeJob ? '/render' : operationHref(activeOp), detail: 'Your project has work in progress. You can check its current status.' };
  else if (selected?.status === 'running') primary = { id: 'session-progress', label: 'View AI session', href: '/director', detail: 'Your assistant is working on a request.' };
  else if (storyEmpty && attention.length) {
    const next = attention[0];
    primary = { id: next.id, label: next.action, href: next.href, detail: next.detail };
  } else primary = storyEmpty
    ? { id: 'new-story', label: 'Create a story', href: '/create', detail: 'Describe an idea, review the storyboard, then choose what to render.' }
    : { id: 'studio', label: 'Continue in Studio', href: '/?view=studio', detail: 'Open your canvas and chat to keep shaping this project.' };

  const recent = [];
  const addRecent = (source, item, title, kind, href, status = statusLabel(item.status)) => {
    const time = timestamp(item);
    if (!Number.isFinite(time)) return;
    recent.push({ id: `${source}:${item.id}`, title, kind, href, status, time, date: new Date(time).toISOString() });
  };
  for (const job of jobs) {
    const scene = nodeByID.get(job.scene_id)?.label;
    addRecent('job', job, scene ? `Render · ${scene}` : 'Video render', 'Render', '/render');
  }
  for (const op of ops) addRecent('op', op, kindLabel(op.kind), 'Project task', operationHref(op));
  for (const asset of assets.filter(item => item.type !== 'output_video')) addRecent('asset', asset, asset.name || 'Untitled asset', 'Asset', '/assets', 'Saved');
  for (const session of sessions) addRecent('session', session, session.title || 'Untitled session', 'AI session', '/director', sessionState(session, snapshot.aiHealth).label);
  for (const edit of Array.isArray(snapshot.compositions) ? snapshot.compositions : []) addRecent('composition', {...edit,id:edit.revision}, `Saved composition · ${edit.duration}s`, 'Composition', `/compositions?revision=${encodeURIComponent(edit.revision)}`, 'Saved');
  recent.sort((a, b) => b.time - a.time || a.id.localeCompare(b.id));
  return { metrics, storyEmpty, primary, attention, recent: recent.slice(0, 5), runningCount: [...jobs, ...ops].filter(item => ['running', 'pending'].includes(item.status)).length };
}
