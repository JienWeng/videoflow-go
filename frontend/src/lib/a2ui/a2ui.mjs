/** A small A2UI client (https://a2ui.org, protocol v1.0) for POST /a2ui
 * (internal/api/a2ui.go). It parses the server-sent stream of A2UI messages, folds
 * them into surfaces (components by id plus a data model) and reports how the request
 * ended, which the server keeps at /run in the surface's data model. There is no
 * official Svelte renderer; A2UISurface.svelte draws a surface with the app's own
 * components.
 */

export const A2UI_VERSION = '1.0';
/** The actions the app sends back (internal/api/a2ui.go). */
export const REPLY_ACTION = 'videoflow.reply';
export const CARD_RESULT_ACTION = 'videoflow.card_result';
/** The VideoFlowAction component's id in every turn surface. */
export const CARD_ID = 'card';

/** The request could not start: POST /a2ui is missing, refused or did not stream. The
 * caller falls back to POST /chat; nothing was planned yet. */
export class A2UIUnavailable extends Error {
  /** @param {string} message */
  constructor(message) {
    super(message);
    this.name = 'A2UIUnavailable';
  }
}

/** The request started and ended in an error (or the stream broke off). */
export class A2UIRunError extends Error {
  /** @param {string} message @param {string} [code] */
  constructor(message, code) {
    super(message);
    this.name = 'A2UIRunError';
    this.code = code;
  }
  /** The HTTP status POST /chat would have answered with, from codes like "http_409". */
  get status() {
    const m = /^http_(\d{3})$/.exec(this.code ?? '');
    if (m) return Number(m[1]);
    return this.code === 'chat_context_invalid' ? 409 : 500;
  }
}

/** Splits a server-sent event stream into messages. Push text as it arrives; each
 * complete event's JSON data goes to onMessage. Comments and other fields are skipped.
 * @param {(message: any) => void} onMessage */
export function createSSEParser(onMessage) {
  let buffer = '';
  /** @param {string} block */
  function dispatch(block) {
    const data = [];
    for (const line of block.split(/\r\n|\r|\n/)) {
      if (line.startsWith('data:')) data.push(line.slice(line.startsWith('data: ') ? 6 : 5));
    }
    if (!data.length) return;
    onMessage(JSON.parse(data.join('\n')));
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

/** @param {string} pointer */
const tokens = pointer =>
  pointer === '' || pointer === '/' ? [] : pointer.replace(/^\//, '').split('/').map(t => t.replace(/~1/g, '/').replace(/~0/g, '~'));

/** The value at a JSON Pointer, or undefined.
 * @param {any} data @param {string} pointer */
export function getAt(data, pointer) {
  let node = data;
  for (const t of tokens(pointer)) {
    if (node === null || typeof node !== 'object') return undefined;
    node = node[t];
  }
  return node;
}

/** Returns data with value set at pointer (null deletes the key, as A2UI says); the
 * input is not changed. Missing objects on the way are created.
 * @param {any} data @param {string} pointer @param {any} value */
export function setAt(data, pointer, value) {
  const path = tokens(pointer);
  if (!path.length) return value === null ? {} : value;
  /** @param {any} node @param {number} i @returns {any} */
  const go = (node, i) => {
    const key = path[i];
    const base = Array.isArray(node) ? [...node] : node && typeof node === 'object' ? { ...node } : {};
    if (i === path.length - 1) {
      if (value === null && !Array.isArray(base)) delete base[key];
      else base[key] = value;
      return base;
    }
    base[key] = go(base[key], i + 1);
    return base;
  };
  return go(data, 0);
}

/** Folds one A2UI message into the surfaces (by id). Returns new surfaces; a message
 * for an unknown surface or of an unknown kind leaves them as they were.
 * @param {Record<string, import('./a2ui.d.mts').Surface>} surfaces @param {any} message
 * @returns {Record<string, import('./a2ui.d.mts').Surface>} */
export function applyMessage(surfaces, message) {
  if (!message || typeof message !== 'object') return surfaces;
  if (message.createSurface) {
    const c = message.createSurface;
    /** @type {Record<string, any>} */
    const components = {};
    for (const comp of c.components ?? []) components[comp.id] = comp;
    return { ...surfaces, [c.surfaceId]: { id: c.surfaceId, catalogId: c.catalogId ?? '', components, data: c.dataModel ?? {} } };
  }
  if (message.updateComponents) {
    const u = message.updateComponents;
    const s = surfaces[u.surfaceId];
    if (!s) return surfaces;
    const components = { ...s.components };
    for (const comp of u.components ?? []) components[comp.id] = comp;
    return { ...surfaces, [s.id]: { ...s, components } };
  }
  if (message.updateDataModel) {
    const u = message.updateDataModel;
    const s = surfaces[u.surfaceId];
    if (!s) return surfaces;
    return { ...surfaces, [s.id]: { ...s, data: setAt(s.data, u.path ?? '/', u.value === undefined ? null : u.value) } };
  }
  if (message.deleteSurface) {
    const { [message.deleteSurface.surfaceId]: _gone, ...rest } = surfaces;
    return rest;
  }
  return surfaces;
}

/** A property's value: a literal, or a {"@path"} binding read from the data model
 * (relative paths read from scope, the item of a template list).
 * @param {any} value @param {any} data @param {any} [scope] */
export function resolve(value, data, scope) {
  if (value && typeof value === 'object' && !Array.isArray(value) && typeof value['@path'] === 'string') {
    const path = value['@path'];
    return path.startsWith('/') ? getAt(data, path) : getAt(scope ?? data, '/' + path);
  }
  return value;
}

/** A container's children as [component id, scope] pairs: a static list of ids, or a
 * template ({path, componentId}) repeated for each item at path.
 * @param {any} children @param {any} data
 * @returns {[string, any][]} */
export function childList(children, data) {
  if (Array.isArray(children)) return children.filter(id => typeof id === 'string').map(id => [id, undefined]);
  if (children && typeof children === 'object' && typeof children.componentId === 'string') {
    const items = getAt(data, children.path ?? '');
    if (Array.isArray(items)) return items.map(item => [children.componentId, item]);
  }
  return [];
}

/** An A2UI action event (renderer to agent).
 * @param {{name: string, surfaceId: string, sourceComponentId: string, context?: Record<string, unknown>}} a */
export function actionEvent(a) {
  return { version: A2UI_VERSION, action: { ...a, timestamp: new Date().toISOString(), context: a.context ?? {} } };
}

/** Runs one POST /a2ui request: posts the body, folds the streamed messages into
 * surfaces and calls onUpdate after each. Resolves with the surface the request was
 * about and the /run result. Throws A2UIUnavailable when the request never started
 * (fall back to POST /chat), A2UIRunError when it ended in an error.
 * @param {{url: string, body: object, surfaces?: Record<string, import('./a2ui.d.mts').Surface>, onUpdate?: (surface: import('./a2ui.d.mts').Surface, message: any) => void, signal?: AbortSignal, fetch?: typeof fetch}} options */
export async function runA2UI({ url, body, surfaces: initial = {}, onUpdate, signal, fetch: fetchImpl = fetch }) {
  /** @type {Response} */
  let response;
  try {
    response = await fetchImpl(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream' },
      body: JSON.stringify(body),
      signal
    });
  } catch (e) {
    if (signal?.aborted) throw e;
    throw new A2UIUnavailable(e instanceof Error ? e.message : String(e));
  }
  if (!response.ok || !response.body || !(response.headers.get('Content-Type') ?? '').startsWith('text/event-stream'))
    throw new A2UIUnavailable(`POST ${url} answered ${response.status}`);
  let surfaces = initial;
  /** @type {string | null} */
  let surfaceId = null;
  const run = () => (surfaceId ? surfaces[surfaceId]?.data?.run : undefined);
  const ended = () => run()?.status === 'finished' || run()?.status === 'error';
  const parser = createSSEParser(message => {
    if (message?.version !== A2UI_VERSION) return;
    surfaces = applyMessage(surfaces, message);
    const id = Object.values(message).find(v => v && typeof v === 'object' && typeof v.surfaceId === 'string')?.surfaceId;
    if (id && surfaces[id]) {
      surfaceId ??= id;
      if (id === surfaceId) onUpdate?.(surfaces[id], message);
    }
  });
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      parser.push(decoder.decode(value, { stream: true }));
      if (ended()) break;
    }
    parser.push(decoder.decode());
    parser.end();
  } catch (e) {
    if (!surfaceId && !signal?.aborted) throw new A2UIUnavailable(e instanceof Error ? e.message : String(e));
    if (!ended()) throw e;
  } finally {
    reader.cancel().catch(() => {});
  }
  if (!surfaceId) throw new A2UIUnavailable('the request never started');
  const r = run();
  if (r?.status === 'error') throw new A2UIRunError(r.message ?? 'The assistant could not answer', r.code);
  if (r?.status !== 'finished') throw new A2UIRunError('The answer stopped before it finished. Send your message again.');
  return { surface: surfaces[surfaceId], result: r.result };
}

/** A turn's phases (its /steps) as an activity "plan" entry (chat/activity.mjs).
 * @param {{name: string, label?: string, status: 'running' | 'done'}[] | undefined} steps */
export function stepsActivity(steps) {
  if (!Array.isArray(steps) || !steps.length) return [];
  return [
    {
      kind: 'plan',
      id: 'turn-steps',
      title: 'Steps',
      status: steps.every(s => s.status === 'done') ? 'done' : 'running',
      items: steps.map(s => ({ title: s.label || s.name, status: s.status }))
    }
  ];
}

/** A surface for a /chat reply that did not stream: the assistant's interface and the
 * card under a body column, so both paths draw the same way.
 * @param {string} id @param {any} intent @returns {import('./a2ui.d.mts').Surface} */
export function localSurface(id, intent) {
  /** @type {Record<string, any>} */
  const components = {};
  const body = [];
  if (Array.isArray(intent?.ui) && intent.ui.length) {
    for (const c of intent.ui) components[c.id] = c;
    body.push('ui-root');
  }
  if (intent?.action && intent.action !== 'unknown') {
    components[CARD_ID] = { id: CARD_ID, component: 'VideoFlowAction', intent: { '@path': '/intent' }, result: { '@path': '/result' } };
    body.push(CARD_ID);
  }
  components.body = { id: 'body', component: 'Column', children: body };
  return { id, catalogId: '', components, data: { intent } };
}
