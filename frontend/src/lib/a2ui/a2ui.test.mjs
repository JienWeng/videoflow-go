import { test } from 'node:test';
import assert from 'node:assert/strict';
import {
  A2UIRunError,
  A2UIUnavailable,
  actionEvent,
  applyMessage,
  childList,
  createSSEParser,
  getAt,
  localSurface,
  resolve,
  runA2UI,
  setAt,
  stepsActivity
} from './a2ui.mjs';

const v = '1.0';
/** @param {any[]} messages */
const sse = messages => messages.map(m => `data: ${JSON.stringify(m)}\n\n`).join('');

/** A fetch answering with body, sent in pieces of size bytes. */
function fakeFetch(body, { status = 200, type = 'text/event-stream', size = 7 } = {}) {
  /** @type {any} */
  let sent;
  const fn = async (url, init) => {
    sent = { url, init };
    const bytes = new TextEncoder().encode(body);
    let at = 0;
    const stream = new ReadableStream({
      pull(controller) {
        if (at >= bytes.length) return controller.close();
        controller.enqueue(bytes.slice(at, (at += size)));
      }
    });
    return new Response(stream, { status, headers: { 'Content-Type': type } });
  };
  return { fn, sent: () => sent };
}

const create = (id = 's1') => ({
  version: v,
  createSurface: {
    surfaceId: id,
    catalogId: 'c',
    components: [{ id: 'root', component: 'Column', children: ['reply'] }, { id: 'reply', component: 'Text', text: { '@path': '/reply' } }],
    dataModel: { run: { status: 'running' }, reply: '', steps: [] }
  }
});
const data = (path, value, id = 's1') => ({ version: v, updateDataModel: { surfaceId: id, path, value } });

test('the parser reassembles messages split across chunks and skips comments', () => {
  const got = [];
  const parser = createSSEParser(m => got.push(m));
  const text = ': heartbeat\n\n' + sse([create(), data('/reply', 'a\nb')]) + 'data: {"version":"1.0",\r\ndata: "deleteSurface":{"surfaceId":"s1"}}\r\n\r\n';
  for (let i = 0; i < text.length; i += 5) parser.push(text.slice(i, i + 5));
  parser.end();
  assert.equal(got.length, 3);
  assert.equal(got[1].updateDataModel.value, 'a\nb');
  assert.equal(got[2].deleteSurface.surfaceId, 's1');
});

test('JSON Pointer reads and writes without changing the input; null deletes', () => {
  const doc = { a: { 'x/y': [1, 2] }, b: 1 };
  assert.deepEqual(getAt(doc, '/a/x~1y/1'), 2);
  assert.equal(getAt(doc, '/nope/deeper'), undefined);
  const next = setAt(doc, '/a/x~1y/0', 9);
  assert.deepEqual(next.a['x/y'], [9, 2]);
  assert.deepEqual(doc.a['x/y'], [1, 2]);
  assert.deepEqual(setAt(doc, '/c/d', true).c, { d: true });
  assert.equal('b' in setAt(doc, '/b', null), false);
  assert.deepEqual(setAt(doc, '/', { z: 1 }), { z: 1 });
});

test('the reducer creates, updates and deletes surfaces', () => {
  let s = applyMessage({}, create());
  assert.equal(s.s1.components.reply.component, 'Text');
  s = applyMessage(s, data('/reply', 'Hello'));
  s = applyMessage(s, data('/state/decisions', [{ key: 'tone' }]));
  assert.equal(s.s1.data.reply, 'Hello');
  assert.deepEqual(s.s1.data.state.decisions, [{ key: 'tone' }]);
  const before = s;
  s = applyMessage(s, { version: v, updateComponents: { surfaceId: 's1', components: [{ id: 'card', component: 'VideoFlowAction' }, { id: 'root', component: 'Column', children: ['reply', 'card'] }] } });
  assert.deepEqual(s.s1.components.root.children, ['reply', 'card']);
  assert.deepEqual(before.s1.components.root.children, ['reply'], 'earlier surfaces are not changed');
  assert.equal(applyMessage(s, data('/x', 1, 'unknown')), s, 'an unknown surface is ignored');
  assert.equal(applyMessage(s, { version: v, somethingElse: {} }), s);
  s = applyMessage(s, { version: v, deleteSurface: { surfaceId: 's1' } });
  assert.deepEqual(s, {});
});

test('values resolve from literals, absolute paths and a template item', () => {
  const model = { reply: 'hi', steps: [{ label: 'One' }, { label: 'Two' }] };
  assert.equal(resolve('lit', model), 'lit');
  assert.equal(resolve({ '@path': '/reply' }, model), 'hi');
  assert.equal(resolve({ '@path': 'label' }, model, model.steps[1]), 'Two');
  assert.deepEqual(childList(['a', 'b', 3], model), [['a', undefined], ['b', undefined]]);
  assert.deepEqual(childList({ path: '/steps', componentId: 'step' }, model).map(([id, item]) => id + ':' + item.label), ['step:One', 'step:Two']);
  assert.deepEqual(childList({ path: '/missing', componentId: 'step' }, model), []);
});

test('runA2UI streams a turn and resolves with the surface and the result', async () => {
  const body = sse([
    create(),
    data('/steps', [{ name: 'read_project', label: 'Reading the project', status: 'running' }]),
    data('/steps', [{ name: 'read_project', label: 'Reading the project', status: 'done' }]),
    data('/reply', 'Hello '),
    data('/reply', 'Hello there'),
    data('/run', { status: 'finished', result: { intent: { reply: 'Hello there' }, a2ui_surface: 's1' } })
  ]);
  const { fn, sent } = fakeFetch(body);
  const replies = [];
  const out = await runA2UI({ url: '/a2ui', body: { message: 'hi' }, fetch: fn, onUpdate: s => replies.push(s.data.reply) });
  assert.equal(out.result.a2ui_surface, 's1');
  assert.equal(out.surface.data.reply, 'Hello there');
  assert.ok(replies.includes('Hello '), 'the reply grows as it streams');
  assert.equal(JSON.parse(sent().init.body).message, 'hi');
  assert.deepEqual(stepsActivity(out.surface.data.steps)[0].items, [{ title: 'Reading the project', status: 'done' }]);
});

test('runA2UI updates a known surface for a card result', async () => {
  const known = applyMessage({}, create('turn-1'));
  const body = sse([data('/run', { status: 'running' }, 'turn-1'), data('/result', { status: 'succeeded' }, 'turn-1'), data('/run', { status: 'finished', result: {} }, 'turn-1')]);
  const { fn, sent } = fakeFetch(body);
  const event = actionEvent({ name: 'videoflow.card_result', surfaceId: 'turn-1', sourceComponentId: 'card', context: { op_id: 'op-1' } });
  const out = await runA2UI({ url: '/a2ui', body: event, surfaces: known, fetch: fn });
  assert.deepEqual(out.surface.data.result, { status: 'succeeded' });
  const posted = JSON.parse(sent().init.body);
  assert.equal(posted.version, '1.0');
  assert.equal(posted.action.sourceComponentId, 'card');
  assert.ok(posted.action.timestamp);
});

test('an error ends the request with A2UIRunError and its status', async () => {
  const { fn } = fakeFetch(sse([create(), data('/run', { status: 'error', message: 'Codex could not answer', code: 'http_409' })]));
  await assert.rejects(runA2UI({ url: '/a2ui', body: {}, fetch: fn }), e => e instanceof A2UIRunError && e.status === 409 && e.message === 'Codex could not answer');
  const cut = fakeFetch(sse([create(), data('/reply', 'half')]));
  await assert.rejects(runA2UI({ url: '/a2ui', body: {}, fetch: cut.fn }), A2UIRunError);
  assert.equal(new A2UIRunError('x', 'chat_context_invalid').status, 409);
});

test('a request that cannot start is A2UIUnavailable (fall back to POST /chat)', async () => {
  await assert.rejects(runA2UI({ url: '/a2ui', body: {}, fetch: fakeFetch('nope', { status: 404, type: 'text/plain' }).fn }), A2UIUnavailable);
  await assert.rejects(runA2UI({ url: '/a2ui', body: {}, fetch: fakeFetch('{}', { type: 'application/json' }).fn }), A2UIUnavailable);
  await assert.rejects(runA2UI({ url: '/a2ui', body: {}, fetch: fakeFetch('').fn }), A2UIUnavailable);
  await assert.rejects(runA2UI({ url: '/a2ui', body: {}, fetch: async () => { throw new TypeError('offline'); } }), A2UIUnavailable);
});

test('a /chat reply gets the same surface layout as a streamed one', () => {
  const ui = [{ id: 'ui-root', component: 'Text', text: 'Pick' }];
  const s = localSurface('local-1', { action: 'generate_script', ui });
  assert.deepEqual(s.components.body.children, ['ui-root', 'card']);
  assert.equal(s.components.card.component, 'VideoFlowAction');
  assert.deepEqual(localSurface('l', { action: 'unknown' }).components.body.children, []);
});
