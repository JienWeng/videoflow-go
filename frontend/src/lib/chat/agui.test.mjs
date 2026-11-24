import { test } from 'node:test';
import assert from 'node:assert/strict';
import { AguiRunError, AguiUnavailable, applyPatch, createSSEParser, reduceEvent, emptyView, runAgent, stepsActivity } from './agui.mjs';

/** @param {any[]} events */
const sse = events => events.map(e => `data: ${JSON.stringify(e)}\n\n`).join('');

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

const input = { threadId: 't', runId: 'r', state: {}, messages: [{ id: 'm', role: 'user', content: 'hi' }], tools: [], context: [] };

test('the parser reassembles events split across chunks and skips comments', () => {
  const got = [];
  const parser = createSSEParser(e => got.push(e));
  const text = ': heartbeat\n\n' + sse([{ type: 'RUN_STARTED', threadId: 't', runId: 'r' }, { type: 'TEXT_MESSAGE_CONTENT', messageId: 'm', delta: 'a\nb' }]) + 'data: {"type":"RUN_FINISHED",\r\ndata: "threadId":"t","runId":"r"}\r\n\r\n';
  for (let i = 0; i < text.length; i += 5) parser.push(text.slice(i, i + 5));
  parser.end();
  assert.deepEqual(got.map(e => e.type), ['RUN_STARTED', 'TEXT_MESSAGE_CONTENT', 'RUN_FINISHED']);
  assert.equal(got[1].delta, 'a\nb');
});

test('JSON Patch applies add, remove, replace, move, copy and test without changing the input', () => {
  const doc = { stages: { current: 'brief', list: [1, 2] }, 'a/b': { '~x': 1 }, decisions: [] };
  const out = applyPatch(doc, [
    { op: 'replace', path: '/stages/current', value: 'script' },
    { op: 'add', path: '/stages/list/1', value: 9 },
    { op: 'add', path: '/decisions/-', value: { key: 'tone' } },
    { op: 'remove', path: '/stages/list/0' },
    { op: 'replace', path: '/a~1b/~0x', value: 2 },
    { op: 'copy', from: '/stages/current', path: '/copied' },
    { op: 'move', from: '/copied', path: '/moved' },
    { op: 'test', path: '/moved', value: 'script' }
  ]);
  assert.deepEqual(out, { stages: { current: 'script', list: [9, 2] }, 'a/b': { '~x': 2 }, decisions: [{ key: 'tone' }], moved: 'script' });
  assert.equal(doc.stages.current, 'brief');
  assert.throws(() => applyPatch(doc, [{ op: 'replace', path: '/missing', value: 1 }]));
  assert.throws(() => applyPatch(doc, [{ op: 'test', path: '/stages/current', value: 'x' }]));
});

test('events fold into text, steps, tool calls and state', () => {
  let view = emptyView();
  for (const e of [
    { type: 'RUN_STARTED', threadId: 't', runId: 'r' },
    { type: 'STATE_SNAPSHOT', snapshot: { mode: 'manual', decisions: [] } },
    { type: 'STEP_STARTED', stepName: 'read_project' },
    { type: 'STEP_FINISHED', stepName: 'read_project' },
    { type: 'STEP_STARTED', stepName: 'ask_ai_session' },
    { type: 'TEXT_MESSAGE_START', messageId: 'm', role: 'assistant' },
    { type: 'TEXT_MESSAGE_CONTENT', messageId: 'm', delta: 'Hello ' },
    { type: 'TEXT_MESSAGE_CONTENT', messageId: 'm', delta: 'there' },
    { type: 'TOOL_CALL_START', toolCallId: 'c', toolCallName: 'render_scene' },
    { type: 'TOOL_CALL_ARGS', toolCallId: 'c', delta: '{"action":' },
    { type: 'TOOL_CALL_ARGS', toolCallId: 'c', delta: '"render_scene"}' },
    { type: 'TOOL_CALL_RESULT', messageId: 'x', toolCallId: 'c', content: '{"status":"succeeded"}' },
    { type: 'STATE_DELTA', delta: [{ op: 'add', path: '/decisions/0', value: { key: 'tone' } }] }
  ])
    view = reduceEvent(view, e);
  assert.equal(view.text, 'Hello there');
  assert.deepEqual(view.steps, [{ name: 'read_project', status: 'done' }, { name: 'ask_ai_session', status: 'running' }]);
  assert.deepEqual(JSON.parse(view.toolCalls.c.args), { action: 'render_scene' });
  assert.equal(view.toolCalls.c.result, '{"status":"succeeded"}');
  assert.deepEqual(view.state, { mode: 'manual', decisions: [{ key: 'tone' }] });
  assert.equal(stepsActivity(view.steps)[0].items[1].title, 'Asking the AI session');
});

test('a run resolves with RUN_FINISHED result and reports each event', async () => {
  const { fn, sent } = fakeFetch(
    sse([
      { type: 'RUN_STARTED', threadId: 't', runId: 'r' },
      { type: 'TEXT_MESSAGE_START', messageId: 'm' },
      { type: 'TEXT_MESSAGE_CONTENT', messageId: 'm', delta: 'Done.' },
      { type: 'TEXT_MESSAGE_END', messageId: 'm' },
      { type: 'RUN_FINISHED', threadId: 't', runId: 'r', result: { intent: { action: 'unknown', reply: 'Done.' } } }
    ])
  );
  const texts = [];
  const result = await runAgent({ url: '/agui', input, fetch: fn, onView: v => texts.push(v.text) });
  assert.equal(result.intent.reply, 'Done.');
  assert.ok(texts.includes('Done.'));
  assert.equal(JSON.parse(sent().init.body).threadId, 't');
  assert.equal(sent().init.headers.Accept, 'text/event-stream');
});

test('a run that never starts is unavailable, so the caller falls back to /chat', async () => {
  await assert.rejects(runAgent({ url: '/agui', input, fetch: fakeFetch('not found', { status: 404, type: 'text/plain' }).fn }), AguiUnavailable);
  await assert.rejects(runAgent({ url: '/agui', input, fetch: fakeFetch('{"ok":true}', { type: 'application/json' }).fn }), AguiUnavailable);
  await assert.rejects(runAgent({ url: '/agui', input, fetch: fakeFetch(': heartbeat\n\n').fn }), AguiUnavailable);
  await assert.rejects(runAgent({ url: '/agui', input, fetch: async () => { throw new TypeError('Failed to fetch'); } }), AguiUnavailable);
});

test('a started run that fails is an error, not a fallback', async () => {
  const failed = fakeFetch(sse([{ type: 'RUN_STARTED', threadId: 't', runId: 'r' }, { type: 'RUN_ERROR', message: 'Saved chat context is out of date.', code: 'chat_context_invalid' }]));
  await assert.rejects(runAgent({ url: '/agui', input, fetch: failed.fn }), e => e instanceof AguiRunError && e.code === 'chat_context_invalid' && e.status === 409);
  const cut = fakeFetch(sse([{ type: 'RUN_STARTED', threadId: 't', runId: 'r' }]));
  await assert.rejects(runAgent({ url: '/agui', input, fetch: cut.fn }), e => e instanceof AguiRunError && !(e instanceof AguiUnavailable));
  assert.equal(new AguiRunError('x', 'http_500').status, 500);
});
