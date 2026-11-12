import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { pathToFileURL } from 'node:url';

const { chromium } = await import(pathToFileURL(process.env.PLAYWRIGHT_MODULE || '/tmp/videoflow-browser-runner/node_modules/playwright/index.mjs'));
const base = process.env.E2E_BASE_URL || 'http://127.0.0.1:8080';
const out = process.env.E2E_OUTPUT || '/tmp/videoflow-dashboard-e2e';
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ executablePath: process.env.CHROME_BIN || '/usr/bin/google-chrome', headless: true, args: ['--no-sandbox'] });
const report = { checks: [], errors: [] };
try {
  for (const mode of ['desktop', 'mobile']) {
    const context = await browser.newContext({ viewport: mode === 'desktop' ? { width: 1440, height: 1000 } : { width: 390, height: 844 } });
    await context.addInitScript(() => { localStorage.setItem('videoflow.onboarded', '1'); localStorage.setItem('videoflow.canvas.coach', '1'); });
    let graph = { nodes: [], edges: [] }, assets = [], jobs = [], ops = [], sessions = [];
    let graphOffline = false, aiOffline = false;
    const mutations = [];
    const now = Date.now(), stamp = minutes => new Date(now - minutes * 60000).toISOString();
    await context.route('**/*', async route => {
      const request = route.request(), url = new URL(request.url());
      if (!url.pathname.startsWith('/api/')) return url.origin === new URL(base).origin ? route.continue() : route.abort();
      const path = url.pathname.slice(4);
      if (request.method() !== 'GET') mutations.push(path);
      if (path === '/graph' && graphOffline) return route.fulfill({ status: 503, json: { error: 'Fixture graph unavailable' } });
      if (path === '/director/status' && aiOffline) return route.fulfill({ status: 503, json: { error: 'Fixture AI status unavailable' } });
      let body = [];
      if (path === '/projects/active') body = { id: 'project', name: 'Forest film', description: 'A short story about a fox.' };
      else if (path === '/graph') body = graph;
      else if (path === '/assets') body = assets;
      else if (path === '/render-jobs') body = jobs;
      else if (path === '/ops') body = url.searchParams.get('limit') === 'all' ? ops : ops.slice(0, 15);
      else if (path.startsWith('/ops/')) body = ops.find(op => op.id === path.slice(5)) ?? {};
      else if (path === '/videos/preflight') body = { ready: false, effective_style: 'Ink illustration', effective_routes: { image_provider: 'fixture', image_model: 'fixture', video_provider: 'fixture', video_model: 'fixture' }, missing: [], video_durations: [] };
      else if (path === '/director/sessions') body = sessions;
      else if (path === '/director/status') body = { all_text_local: true, engines: [{ engine: 'codex', installed: true }] };
      else if (path.endsWith('/turns')) body = [];
      else if (path === '/health') body = { status: 'ok', missing_keys: [], media_ready: true };
      else if (path === '/style') body = { style_prompt: 'Illustration' };
      else if (path === '/characters') body = [{ id: 'fox', name: 'Mimi' }];
      else if (path === '/characters/fox') body = { id: 'fox', name: 'Mimi', description: 'A forest fox', appearance: '', personality: '', visual_rules_json: [], voice_rules_json: [], sample_dialogue: '' };
      else if (path === '/scenes') body = [{ id: 'scene', title: 'Forest' }];
      else if (path === '/events') return route.fulfill({ contentType: 'text/event-stream', body: ': fixture\n\n' });
      return route.fulfill({ json: body });
    });
    const page = await context.newPage();
    page.setDefaultTimeout(5000);
    page.on('pageerror', error => report.errors.push({ mode, error: error.message }));
    async function check(name, fn) {
      if (report.checks.some(item => item.name.startsWith(mode + ':') && item.status === 'fail')) { report.checks.push({ name: `${mode}: ${name}`, status: 'skipped' }); return; }
      try { await fn(); report.checks.push({ name: `${mode}: ${name}`, status: 'pass' }); console.error('PASS', mode, name); }
      catch (error) { report.checks.push({ name: `${mode}: ${name}`, status: 'fail', error: error.message }); console.error('FAIL', mode, name, error.message); }
    }
    const refresh = () => page.getByRole('button', { name: 'Refresh overview', exact: true }).click();
    await check('empty project is oriented with true totals and a first action', async () => {
      await page.goto(base);
      await page.getByRole('heading', { name: 'Project overview', exact: true }).waitFor();
      await page.getByRole('heading', { name: 'Forest film', exact: true }).waitFor();
      await page.getByRole('link', { name: 'Scenes: 0', exact: true }).waitFor();
      await page.getByRole('link', { name: 'Choose AI session', exact: true }).waitFor();
      await page.getByText('No activity yet. Start a story or add your first reference to begin.', { exact: true }).waitFor();
      assert.equal(await page.getByLabel('AI session', { exact: true }).count(), 0, 'Hidden editing workspace should not be mounted on first arrival');
    });
    await check('populated project has correct counts, recent work and resume action', async () => {
      graph = { nodes: [{ id: 'scene', type: 'scene', label: 'Forest', data: { duration: 5, aspect_ratio: '16:9' } }, { id: 'fox', type: 'character', label: 'Mimi' }, { id: 'out', type: 'output', label: 'Take one', data: { video_path: 'storage/take.mp4' } }], edges: [] };
      sessions = [{ id: 'session', title: 'Forest planning', engine: 'codex', status: 'ready', selected: true, created_at: stamp(10) }];
      assets = [{ id: 'asset', name: 'Forest reference', type: 'character_reference', created_at: stamp(5), metadata_json: {} }];
      jobs = [{ id: 'job', scene_id: 'scene', status: 'succeeded', created_at: stamp(2) }];
      await refresh();
      await page.getByRole('link', { name: 'Scenes: 1', exact: true }).waitFor();
      await page.getByRole('link', { name: 'Finished videos: 1', exact: true }).waitFor();
      await page.getByRole('button', { name: 'Continue in Studio', exact: true }).waitFor();
      const items = page.getByRole('region', { name: 'Recent activity', exact: true }).getByRole('listitem');
      assert.equal(await items.count(), 3);
      assert.match(await items.first().innerText(), /Render · Forest/);
      assert.match(await page.getByRole('region', { name: 'Project totals', exact: true }).innerText(), /Updated/);
    });
    await check('recovery and pending references are actionable without any resubmission', async () => {
      jobs[0].status = 'reconciliation_required'; assets[0].metadata_json.consistency_review = 'pending';
      await refresh();
      await page.getByRole('link', { name: 'Review renders', exact: true }).waitFor();
      await page.getByRole('link', { name: 'Review references', exact: true }).waitFor();
      assert.equal(await page.getByRole('link', { name: 'Review renders', exact: true }).getAttribute('href'), '/render');
      assert.equal(mutations.length, 0);
      jobs = []; assets[0].metadata_json = {};
      await refresh(); await page.getByRole('button', { name: 'Continue in Studio', exact: true }).waitFor();
    });
    await check('older interrupted generation opens the exact saved request', async () => {
      const old = { id: 'older-story', kind: 'video_generation', status: 'reconciliation_required', project_id: 'project', created_at: stamp(100), updated_at: stamp(1), error: 'Fixture old generation needs review', result_json: {} };
      ops = [...Array.from({ length: 20 }, (_, i) => ({ id: `new-${i}`, kind: 'assets', status: 'succeeded', created_at: stamp(i + 2) })), old];
      await refresh();
      const link = page.getByRole('link', { name: 'Review request', exact: true });
      await link.waitFor();
      assert.equal(await link.getAttribute('href'), '/create?op_id=older-story');
      await link.click();
      await page.getByText('Fixture old generation needs review', { exact: true }).first().waitFor();
      assert.equal(mutations.length, 0, 'Review must not replay generation');
      ops = []; await page.goto(base);
      await page.getByRole('button', { name: 'Continue in Studio', exact: true }).waitFor();
    });
    await check('required failure retains stale data and retries without false zeros', async () => {
      graphOffline = true; await refresh();
      await page.getByRole('alert').filter({ hasText: 'Fixture graph unavailable' }).waitFor();
      await page.getByText('Showing the last loaded project data.', { exact: true }).waitFor();
      assert.equal(await page.getByRole('link', { name: 'Scenes: 1', exact: true }).count(), 1);
      graphOffline = false; await page.getByRole('button', { name: 'Try again', exact: true }).click();
      await page.getByRole('alert').waitFor({ state: 'hidden' });
    });
    await check('unknown setup is not presented as unconfigured or ready', async () => {
      aiOffline = true; await refresh();
      await page.getByText('AI setup status is unavailable', { exact: true }).waitFor();
      assert.equal(await page.getByText('Your AI assistant needs setup', { exact: true }).count(), 0);
      aiOffline = false; await refresh();
      await page.getByText('AI setup status is unavailable', { exact: true }).waitFor({ state: 'hidden' });
    });
    await check('canvas and chat drafts survive overview switching', async () => {
      await page.getByRole('button', { name: 'Canvas & chat', exact: true }).click();
      await page.getByRole('heading', { name: 'Story studio', exact: true }).waitFor();
      if (mode === 'mobile') await page.getByRole('button', { name: 'Workbench', exact: true }).click();
      const composer = page.getByPlaceholder('Describe your story or select an item…', { exact: true });
      await composer.fill('Keep this unsent story draft.');
      graph.nodes.push({ id: 'river', type: 'scene', label: 'River', data: { duration: 5, aspect_ratio: '16:9' } });
      await page.getByRole('button', { name: 'Overview', exact: true }).click();
      await page.getByRole('heading', { name: 'Project overview', exact: true }).waitFor();
      await page.getByRole('link', { name: 'Scenes: 2', exact: true }).waitFor();
      await page.getByRole('button', { name: 'Canvas & chat', exact: true }).click();
      assert.equal(await composer.inputValue(), 'Keep this unsent story draft.');
      await page.getByRole('button', { name: 'Overview', exact: true }).click();
      assert.equal(mutations.length, 0, 'Switching views must not send AI requests');
    });
    await check('node editing draft survives refresh on returning to canvas', async () => {
      await page.getByRole('button', { name: 'Canvas & chat', exact: true }).click();
      if (mode === 'mobile') await page.getByRole('button', { name: 'Canvas', exact: true }).click();
      await page.getByRole('button', { name: 'Fit View', exact: true }).click();
      await page.locator('.svelte-flow__node[data-id="fox"]').click();
      const name = page.locator('#np-character-name');
      await name.fill('Mimi draft');
      await page.getByRole('button', { name: 'Overview', exact: true }).click();
      await page.getByRole('heading', { name: 'Project overview', exact: true }).waitFor();
      await page.getByRole('button', { name: 'Canvas & chat', exact: true }).click();
      await page.getByText('Your editing draft is kept. Save or discard it before refreshing the canvas.', { exact: true }).waitFor();
      assert.equal(await name.inputValue(), 'Mimi draft');
      page.once('dialog', dialog => dialog.accept());
      await page.getByRole('button', { name: 'Clear selection', exact: true }).click();
      await page.getByRole('button', { name: 'Refresh canvas', exact: true }).click();
      await page.getByRole('button', { name: 'Overview', exact: true }).click();
      assert.equal(mutations.length, 0);
    });
    await check('recent links, quick actions and responsive layout', async () => {
      assert.equal(await page.getByRole('link', { name: 'New story', exact: true }).getAttribute('href'), '/create');
      assert.equal(await page.getByRole('link', { name: 'Add references', exact: true }).getAttribute('href'), '/assets');
      const overview = page.getByRole('region', { name: 'Project overview', exact: true });
      assert(await overview.evaluate(e => e.scrollWidth <= e.clientWidth + 1), 'Overview overflows');
      await page.screenshot({ path: `${out}/${mode}.png`, fullPage: true });
    });
    await context.close();
  }
} finally {
  await browser.close(); await writeFile(`${out}/report.json`, JSON.stringify(report, null, 2));
}
console.log(JSON.stringify(report, null, 2));
if (report.errors.length || report.checks.some(item => item.status === 'fail')) process.exitCode = 1;
