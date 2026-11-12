import test from 'node:test';
import assert from 'node:assert/strict';
import { normalizeActivity, ACTIVITY_LIMITS } from './activity.mjs';

test('rejects malformed events and never labels unknown states successful', () => {
  assert.deepEqual(normalizeActivity({kind: 'tool', title: 'x'}), []);
  const rows = normalizeActivity([null, {kind:'bogus',title:'x'}, {kind:'tool'}, {kind:'tool',title:'Supplied result',status:'success',detail:'<script>alert(1)</script>'}]);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].status, undefined);
  assert.equal(rows[0].detail, '<script>alert(1)</script>'); // Remains plain text for Svelte escaping.
});

test('source links reject script/data/local protocols and credentials, normalize and deduplicate URLs', () => {
  const [row] = normalizeActivity([{kind:'sources',title:'Sources',sources:[
    {url:'javascript:alert(1)'},{url:'data:text/html,x'},{url:'file:///etc/passwd'},
    {url:'https://user:pass@example.com'}, {url:'https://example.com',title:'Docs'},
    {url:'https://example.com/'},{url:'http://localhost:8080/docs'}, {url:123}
  ]}]);
  assert.deepEqual(row.sources.map(s => s.url), ['https://example.com/', 'http://localhost:8080/docs']);
});

test('partial advisory failures preserve real outputs and do not invent execution telemetry', () => {
  const [row] = normalizeActivity([{kind:'subagent',title:'Advisory reviews',children:[
    {kind:'subagent',title:'Editor',status:'succeeded',detail:'Shorten intro',suggestions:['Trim opening']},
    {kind:'subagent',title:'Researcher',status:'failed',detail:'Provider unavailable'}
  ]}]);
  assert.equal(row.children[0].status, 'done');
  assert.equal(row.children[1].status, 'failed');
  assert.equal(row.children[1].code, undefined);
  assert.deepEqual(row.children[0].suggestions, ['Trim opening']);
});

test('context requires actual finite integer usage and a positive limit', () => {
  assert.equal(normalizeActivity([{kind:'context',title:'Context',usage:{limit:100}}]).length, 0);
  assert.equal(normalizeActivity([{kind:'context',title:'Context',usage:{used:0,limit:0}}]).length, 0);
  const [row] = normalizeActivity([{kind:'context',title:'Context',usage:{used:12,limit:100,input:0,output:NaN}}]);
  assert.deepEqual(row.usage, {used:12,limit:100,input:0});
});

test('cyclic trees, huge lists and text stay bounded across all nested events', () => {
  const cycle = {kind:'task',title:'Work',detail:'x'.repeat(200000)};
  cycle.children = [cycle,cycle,cycle];
  const result = normalizeActivity(Array(1000).fill(cycle));
  let count = 0, chars = 0, depth = 0;
  function visit(rows, level) { depth = Math.max(depth,level); for (const row of rows) { count++; chars += row.title.length + (row.detail?.length || 0); visit(row.children,level+1); } }
  visit(result,0);
  assert.ok(count <= ACTIVITY_LIMITS.events);
  assert.ok(chars <= ACTIVITY_LIMITS.totalText);
  assert.ok(depth <= ACTIVITY_LIMITS.depth);
  assert.ok(result.every(row => !row.detail || row.detail.length <= ACTIVITY_LIMITS.text));
});
