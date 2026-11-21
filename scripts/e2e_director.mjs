import { mkdir, writeFile } from 'node:fs/promises';
import { pathToFileURL } from 'node:url';
import assert from 'node:assert/strict';

const { chromium } = await import(pathToFileURL(process.env.PLAYWRIGHT_MODULE || '/tmp/videoflow-browser-runner/node_modules/playwright/index.mjs'));
const base = process.env.E2E_BASE_URL || 'http://127.0.0.1:5194';
const out = process.env.E2E_OUTPUT || '/tmp/videoflow-director-e2e';
await mkdir(out, { recursive: true });
const browser = await chromium.launch({ executablePath: process.env.CHROME_BIN || '/usr/bin/google-chrome', headless: true, args: ['--no-sandbox'] });
const report = { checks: [], errors: [] };

try {
  for (const mode of ['desktop', 'mobile']) {
    const context = await browser.newContext({ viewport: mode === 'desktop' ? { width: 1440, height: 1000 } : { width: 390, height: 844 } });
    let sessions = [], selection = { character_ids: [], asset_ids: [] }, turns = [];
    let offline = false, selectionOffline = false;
    const mutations = [];
    await context.route('**/*', async route => {
      const request = route.request(), url = new URL(request.url());
      if (!url.pathname.startsWith('/api/') && url.port !== '8000') {
        return url.origin === new URL(base).origin ? route.continue() : route.abort();
      }
      const path = url.pathname.replace(/^\/api/, '');
      if (offline && path === '/director/status') return route.fulfill({ status: 503, json: { error: 'Fixture backend offline' } });
      if (selectionOffline && path.endsWith('/selection')) return route.fulfill({ status: 503, json: { error: 'Fixture references unavailable' } });
      if (request.method() !== 'GET') mutations.push({ path, method: request.method(), body: request.postDataJSON() });
      let body = [];
      if (path === '/director/status') body = { all_text_local: true, engines: [{ engine: 'codex', installed: true }, { engine: 'claude', installed: false, reason: 'CLI not installed' }], host_requirement: 'Fixture backend host' };
      else if (path === '/director/sessions' && request.method() === 'GET') body = sessions;
      else if (path === '/director/sessions') {
        sessions = sessions.map(s => ({ ...s, selected: false }));
        body = { id: `s${sessions.length}`, ...request.postDataJSON(), status: 'ready', native_id: '', selected: true };
        sessions.push(body);
      } else if (path.endsWith('/selection')) {
        if (request.method() === 'PUT') { selection = request.postDataJSON(); body = { warnings: ['Fixture reference has no description'] }; }
        else body = selection;
      } else if (path.endsWith('/turns')) body = turns;
      else if (path.endsWith('/select')) { sessions = sessions.map(s => ({ ...s, selected: s.id === path.split('/')[3] })); body = {}; }
      else if (path.endsWith('/cancel')) { sessions.find(s => s.selected).status = 'review_required'; body = {}; }
      else if (path.endsWith('/acknowledge')) { sessions.find(s => s.selected).status = 'ready'; body = {}; }
      else if (path === '/characters') body = [{ id: 'c1', name: 'Mimi' }];
      else if (path === '/assets') body = [{ id: 'a1', name: 'Reference photo', type: 'character_reference' }];
      else if (path === '/projects/active') body = { id: 'project', name: 'Fixture project' };
      else if (path === '/events') return route.fulfill({ contentType: 'text/event-stream', body: ': fixture\n\n' });
      return route.fulfill({ json: body });
    });
    const page = await context.newPage();
    page.setDefaultTimeout(5000);
    page.on('pageerror', error => report.errors.push({ mode, error: error.message }));
    const summary = name => page.locator('main summary').filter({ hasText: name });
    async function check(name, fn) {
      if (report.checks.some(check => check.name.startsWith(mode + ':') && check.status === 'fail')) { report.checks.push({ name: `${mode}: ${name}`, status: 'skipped' }); return; }
      try { await fn(); report.checks.push({ name: `${mode}: ${name}`, status: 'pass' }); console.error('PASS', mode, name); }
      catch (error) { report.checks.push({ name: `${mode}: ${name}`, status: 'fail', error: error.message }); console.error('FAIL', mode, name, error.message); }
    }
    await check('first session and progressive disclosure', async () => {
      await page.goto(`${base}/director`);
      await page.getByRole('heading', { name: 'Start your first session' }).waitFor();
      assert.equal(await page.locator('#ai-setup').getAttribute('open'), null);
      await page.getByLabel('Session name', { exact: false }).fill('Fixture planning');
      await page.getByRole('button', { name: 'Create session', exact: true }).click();
      await page.getByRole('link', { name: 'Continue in Studio' }).waitFor();
      assert.equal(await page.locator('main details[open]').count(), 0);
      assert.equal(await page.getByRole('heading', { name: 'Fixture planning', exact: true }).count(), 1);
      await page.getByText('AI sign-in and account access are checked when you send your first prompt.', { exact: true }).waitFor();
    });
    await check('references save, unsaved guard and discard', async () => {
      await summary('Characters & references').click();
      await page.getByRole('checkbox', { name: 'Mimi', exact: true }).check();
      await page.getByRole('checkbox', { name: 'Reference photo', exact: true }).check();
      assert.equal(await page.getByRole('combobox', { name: /Your sessions/ }).isDisabled(), true);
      assert.equal(await page.getByRole('link', { name: 'Continue in Studio' }).getAttribute('aria-disabled'), 'true');
      await page.getByRole('button', { name: 'Save references', exact: true }).click();
      await page.getByText('References saved for your next prompt.', { exact: true }).waitFor();
      assert.deepEqual(selection, { character_ids: ['c1'], asset_ids: ['a1'] });
      await page.getByRole('checkbox', { name: 'Mimi', exact: true }).uncheck();
      await page.getByRole('button', { name: 'Discard changes', exact: true }).click();
      assert.equal(await page.getByRole('checkbox', { name: 'Mimi', exact: true }).isChecked(), true);
      assert.equal(await page.getByRole('combobox', { name: /Your sessions/ }).isEnabled(), true);
    });
    await check('running cancel and interrupted review gate', async () => {
      sessions[0].status = 'running'; sessions[0].native_id = 'native-fixture';
      turns = [{ id: 't1', kind: 'script', status: 'cancelled', created_at: '2026-11-12T08:00:00Z', request_json: { prompt: 'Fixture prompt' }, result_json: {}, error: 'Fixture interrupted request' }];
      await page.getByRole('button', { name: 'Refresh sessions' }).click();
      await page.getByText('Planning in progress', { exact: true }).waitFor();
      assert.equal(await page.getByRole('checkbox', { name: 'Mimi', exact: true }).isDisabled(), true);
      await page.getByRole('button', { name: 'Stop request' }).click();
      await page.getByRole('button', { name: 'Review history' }).click();
      await summary('script · cancelled').click();
      await page.getByText('Fixture interrupted request', { exact: true }).waitFor();
      await page.getByRole('checkbox', { name: 'Mimi', exact: true }).uncheck();
      assert.equal(await page.getByRole('button', { name: 'I reviewed this request — continue' }).isDisabled(), true, 'Recovery must not discard unsaved references');
      await page.getByRole('button', { name: 'Discard changes', exact: true }).click();
      await page.getByRole('button', { name: 'I reviewed this request — continue' }).click();
      await page.getByRole('link', { name: 'Continue in Studio' }).waitFor();
      assert.equal(sessions[0].status, 'ready');
      assert(mutations.some(m => m.path.endsWith('/cancel')) && mutations.some(m => m.path.endsWith('/acknowledge')));
    });
    await check('missing engine shows setup, never ready', async () => {
      await page.getByRole('button', { name: 'New session', exact: true }).click();
      assert.equal(await page.getByLabel('Session name', { exact: false }).evaluate(e => e === document.activeElement), true);
      await page.getByRole('combobox', { name: /AI assistant/ }).selectOption('claude');
      await page.getByLabel('Session name', { exact: false }).fill('Claude fixture');
      await page.getByRole('button', { name: 'Create session', exact: true }).click();
      await page.getByText('AI setup required', { exact: true }).waitFor();
      assert.equal(await page.getByRole('link', { name: 'Continue in Studio' }).count(), 0);
      await page.getByRole('button', { name: 'Set up AI' }).click();
      assert.equal(await page.locator('#ai-setup').evaluate(e => e.open && e.querySelector('summary') === document.activeElement), true);
      await page.getByRole('combobox', { name: /Your sessions/ }).selectOption('s0');
      await page.getByRole('link', { name: 'Continue in Studio' }).waitFor();
      assert.equal(sessions[0].selected, true);
    });
    await check('failed details cannot save old references or continue', async () => {
      selectionOffline = true;
      await page.getByRole('button', { name: 'Refresh sessions' }).click();
      await page.getByRole('alert').filter({ hasText: 'Fixture references unavailable' }).waitFor();
      assert.equal(await page.getByRole('link', { name: 'Continue in Studio' }).getAttribute('aria-disabled'), 'true');
      assert.equal(await page.getByRole('button', { name: 'Save references', exact: true }).count(), 0);
      selectionOffline = false;
      await page.getByRole('button', { name: 'Refresh sessions' }).click();
      await summary('Characters & references').waitFor();
    });
    await check('long session name, compact default and no overflow', async () => {
      sessions[0].title = 'A very long session title about a forest adventure and the characters who live there';
      await page.getByRole('button', { name: 'Refresh sessions' }).click();
      await page.getByRole('heading', { name: sessions[0].title, exact: true }).waitFor();
      assert.equal(await page.locator('main details[open]').count(), 1); // AI setup remained intentionally open.
      await summary('AI setup').click();
      assert.equal(await page.locator('main details[open]').count(), 0);
      assert(await page.locator('main').evaluate(e => e.scrollWidth <= e.clientWidth + 1), 'Main content overflows');
      await page.screenshot({ path: `${out}/${mode}.png`, fullPage: true });
    });
    await check('backend error keeps refresh usable and blocks continuation', async () => {
      offline = true;
      await page.getByRole('button', { name: 'Refresh sessions' }).click();
      await page.getByRole('alert').filter({ hasText: 'Fixture backend offline' }).waitFor();
      assert.equal(await page.getByRole('button', { name: 'Refresh sessions' }).isEnabled(), true);
      assert.equal(await page.getByRole('link', { name: 'Continue in Studio' }).getAttribute('aria-disabled'), 'true');
    });
    await context.close();
  }
} finally {
  await browser.close();
  await writeFile(`${out}/report.json`, JSON.stringify(report, null, 2));
}
console.log(JSON.stringify(report, null, 2));
if (report.errors.length || report.checks.some(check => check.status === 'fail')) process.exitCode = 1;
