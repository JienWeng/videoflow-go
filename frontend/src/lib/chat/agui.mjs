/** A small AG-UI client (https://docs.ag-ui.com, protocol 1.0) for POST /agui
 * (internal/api/agui.go). It parses the server-sent event stream, folds state deltas
 * (RFC 6902 JSON Patch) into the shared state and reports how the run ended. The
 * official @ag-ui/client is not used: it brings rxjs, protobuf and more for the few
 * events this app needs.
 */

/** The run could not start: POST /agui is missing, refused or did not stream. The
 * caller falls back to POST /chat; nothing was planned yet. */
export class AguiUnavailable extends Error {
  /** @param {string} message */
  constructor(message) {
    super(message);
    this.name = 'AguiUnavailable';
  }
}

/** The run started and ended with RUN_ERROR (or the stream broke off). */
export class AguiRunError extends Error {
  /** @param {string} message @param {string} [code] */
  constructor(message, code) {
    super(message);
    this.name = 'AguiRunError';
    this.code = code;
  }
  /** The HTTP status POST /chat would have answered with, from codes like "http_409". */
  get status() {
    const m = /^http_(\d{3})$/.exec(this.code ?? '');
    if (m) return Number(m[1]);
    return this.code === 'chat_context_invalid' ? 409 : 500;
  }
}

/** Splits a server-sent event stream into AG-UI events. Push text as it arrives;
 * each complete event's JSON data goes to onEvent. Comments and other fields are
 * skipped.
 * @param {(event: any) => void} onEvent */
export function createSSEParser(onEvent) {
  let buffer = '';
  /** @param {string} block */
  function dispatch(block) {
    const data = [];
    for (const line of block.split(/\r\n|\r|\n/)) {
      if (line.startsWith('data:')) data.push(line.slice(line.startsWith('data: ') ? 6 : 5));
    }
    if (!data.length) return;
    onEvent(JSON.parse(data.join('\n')));
  }
  return {
    /** @param {string} text */
    push(text) {
      buffer += text;
      for (let end = buffer.search(/\r\n\r\n|\n\n|\r\r/); end >= 0; end = buffer.search(/\r\n\r\n|\n\n|\r\r/)) {
        const block = buffer.slice(0, end);
        buffer = buffer.slice(end + (buffer.startsWith('\r\n\r\n', end) ? 4 : 2));
        dispatch(block);
      }
    },
    end() {
      if (buffer.trim()) dispatch(buffer);
      buffer = '';
    }
  };
}

/** @param {string} token */
const unescapeToken = token => token.replace(/~1/g, '/').replace(/~0/g, '~');

/** Applies a JSON Patch (add, remove, replace, move, copy, test) and returns the new
 * document; the input is not changed. Throws when an operation does not apply.
 * @param {any} doc @param {{op: string, path: string, value?: any, from?: string}[]} ops */
export function applyPatch(doc, ops) {
  let result = structuredClone(doc);
  for (const op of ops) {
    if (op.op === 'move' || op.op === 'copy') {
      const value = structuredClone(read(result, op.from ?? ''));
      if (op.op === 'move') result = write(result, op.from ?? '', 'remove');
      result = write(result, op.path, 'add', value);
    } else if (op.op === 'test') {
      if (JSON.stringify(read(result, op.path)) !== JSON.stringify(op.value)) throw new Error(`test failed at ${op.path}`);
    } else {
      result = write(result, op.path, op.op, op.value);
    }
  }
  return result;
}

/** @param {any} doc @param {string} path */
function read(doc, path) {
  if (path === '') return doc;
  let node = doc;
  for (const token of path.slice(1).split('/').map(unescapeToken)) {
    if (node === null || typeof node !== 'object' || !(token in node)) throw new Error(`no value at ${path}`);
    node = node[token];
  }
  return node;
}

/** @param {any} doc @param {string} path @param {string} op @param {any} [value] */
function write(doc, path, op, value) {
  if (path === '') {
    if (op === 'remove') return undefined;
    return value;
  }
  const tokens = path.slice(1).split('/').map(unescapeToken);
  const key = /** @type {string} */ (tokens.pop());
  const parent = read(doc, tokens.length ? '/' + tokens.map(t => t.replace(/~/g, '~0').replace(/\//g, '~1')).join('/') : '');
  if (Array.isArray(parent)) {
    const index = key === '-' ? parent.length : Number(key);
    if (!Number.isInteger(index) || index < 0 || index > parent.length || (index === parent.length && op !== 'add'))
      throw new Error(`bad index at ${path}`);
    if (op === 'add') parent.splice(index, 0, value);
    else if (op === 'replace') parent[index] = value;
    else if (op === 'remove') parent.splice(index, 1);
    else throw new Error(`unsupported op ${op}`);
    return doc;
  }
  if (parent === null || typeof parent !== 'object') throw new Error(`no object at ${path}`);
  if (op !== 'add' && !(key in parent)) throw new Error(`no value at ${path}`);
  if (op === 'add' || op === 'replace') parent[key] = value;
  else if (op === 'remove') delete parent[key];
  else throw new Error(`unsupported op ${op}`);
  return doc;
}

/** Folds one event into a run's view: the streamed text, the steps, the tool calls
 * and the shared state. Returns a new view.
 * @param {import('./agui.d.mts').AguiView} view @param {any} event
 * @returns {import('./agui.d.mts').AguiView} */
export function reduceEvent(view, event) {
  switch (event.type) {
    case 'RUN_STARTED':
      return { ...view, status: 'running' };
    case 'RUN_FINISHED':
      return { ...view, status: 'finished', result: event.result };
    case 'RUN_ERROR':
      return { ...view, status: 'error', error: { message: event.message, code: event.code } };
    case 'STEP_STARTED':
      return { ...view, steps: [...view.steps.filter(s => s.name !== event.stepName), { name: event.stepName, status: 'running' }] };
    case 'STEP_FINISHED':
      return { ...view, steps: view.steps.map(s => (s.name === event.stepName ? { ...s, status: 'done' } : s)) };
    case 'TEXT_MESSAGE_START':
      return { ...view, messageId: event.messageId, text: '' };
    case 'TEXT_MESSAGE_CONTENT':
      return { ...view, text: view.text + event.delta };
    case 'TOOL_CALL_START':
      return { ...view, toolCalls: { ...view.toolCalls, [event.toolCallId]: { name: event.toolCallName, args: '' } } };
    case 'TOOL_CALL_ARGS': {
      const call = view.toolCalls[event.toolCallId];
      return call ? { ...view, toolCalls: { ...view.toolCalls, [event.toolCallId]: { ...call, args: call.args + event.delta } } } : view;
    }
    case 'TOOL_CALL_RESULT': {
      const call = view.toolCalls[event.toolCallId] ?? { name: '', args: '' };
      return { ...view, toolCalls: { ...view.toolCalls, [event.toolCallId]: { ...call, result: event.content } } };
    }
    case 'STATE_SNAPSHOT':
      return { ...view, state: event.snapshot };
    case 'STATE_DELTA':
      return { ...view, state: applyPatch(view.state, event.delta) };
  }
  return view;
}

/** @returns {import('./agui.d.mts').AguiView} */
export function emptyView() {
  return { status: 'idle', text: '', steps: [], toolCalls: {}, state: null };
}

/** Runs one AG-UI run: posts the input, streams the events into the view and calls
 * onView after each. Resolves with RUN_FINISHED's result. Throws AguiUnavailable when
 * the run never started (fall back to POST /chat), AguiRunError when it ended in an
 * error.
 * @param {{url: string, input: object, onView?: (view: import('./agui.d.mts').AguiView, event: any) => void, signal?: AbortSignal, fetch?: typeof fetch}} options */
export async function runAgent({ url, input, onView, signal, fetch: fetchImpl = fetch }) {
  /** @type {Response} */
  let response;
  try {
    response = await fetchImpl(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' },
      body: JSON.stringify(input),
      signal
    });
  } catch (e) {
    if (signal?.aborted) throw e;
    throw new AguiUnavailable(e instanceof Error ? e.message : String(e));
  }
  if (!response.ok || !response.body || !(response.headers.get('Content-Type') ?? '').startsWith('text/event-stream'))
    throw new AguiUnavailable(`POST ${url} answered ${response.status}`);
  let view = emptyView();
  /** @type {Error | null} */
  let parseError = null;
  const parser = createSSEParser(event => {
    try {
      view = reduceEvent(view, event);
    } catch (e) {
      // A delta that does not apply leaves the state as it was; the next snapshot fixes it.
      parseError = e instanceof Error ? e : new Error(String(e));
      return;
    }
    onView?.(view, event);
  });
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      parser.push(decoder.decode(value, { stream: true }));
      if (view.status === 'finished' || view.status === 'error') break;
    }
    parser.push(decoder.decode());
    parser.end();
  } catch (e) {
    if (view.status === 'idle' && !signal?.aborted) throw new AguiUnavailable(e instanceof Error ? e.message : String(e));
    if (view.status !== 'finished' && view.status !== 'error') throw e;
  } finally {
    reader.cancel().catch(() => {});
  }
  if (view.status === 'idle') throw new AguiUnavailable('the run never started');
  if (view.status === 'error') throw new AguiRunError(view.error?.message ?? 'The assistant could not answer', view.error?.code);
  if (view.status !== 'finished')
    throw new AguiRunError(parseError?.message ?? 'The answer stopped before it finished. Send your message again.');
  return view.result;
}

/** Human labels of the chat turn's steps (internal/api/chat.go). */
export const STEP_LABELS = Object.freeze({
  read_project: 'Reading the project',
  ask_ai_session: 'Asking the AI session',
  check_answer: 'Checking the answer'
});

/** The steps as an activity "plan" entry (activity.mjs) for the chat's activity view.
 * @param {import('./agui.d.mts').AguiView['steps']} steps */
export function stepsActivity(steps) {
  if (!steps.length) return [];
  return [
    {
      kind: 'plan',
      id: 'agui-steps',
      title: 'Steps',
      status: steps.every(s => s.status === 'done') ? 'done' : 'running',
      items: steps.map(s => ({ title: STEP_LABELS[/** @type {keyof typeof STEP_LABELS} */ (s.name)] ?? s.name, status: s.status }))
    }
  ];
}
