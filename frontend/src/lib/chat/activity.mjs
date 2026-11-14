/** Public application telemetry only. Never pass private model reasoning to this module. */
export const ACTIVITY_LIMITS = Object.freeze({ events: 80, depth: 5, text: 12000, title: 300, entries: 80, totalText: 120000 });
const kinds = new Set(['progress','reasoning','plan','queue','task','tool','terminal','stack-trace','test-results','subagent','sources','inline-citation','file-tree','context']);
const record = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const number = value => Number.isSafeInteger(value) && value >= 0;
export function normalizeActivity(input) {
  let remaining = ACTIVITY_LIMITS.events, textBudget = ACTIVITY_LIMITS.totalText, fileBudget = ACTIVITY_LIMITS.entries;
  function text(value, max = ACTIVITY_LIMITS.text) {
    if (typeof value !== 'string') return undefined;
    const result = value.slice(0, Math.min(max, textBudget)).replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f]/g, '').trim();
    textBudget -= result.length;
    return result || undefined;
  }
  function status(value) {
    return value === 'succeeded' ? 'done' : ['pending','running','done','failed','skipped'].includes(value) ? value : undefined;
  }
  function sources(input) {
    if (!Array.isArray(input)) return [];
    const seen = new Set();
    return input.slice(0, ACTIVITY_LIMITS.entries).flatMap(source => {
      if (!record(source) || typeof source.url !== 'string' || source.url.length > 4096) return [];
      let url;
      try { url = new URL(source.url); } catch { return []; }
      if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password || seen.has(url.href)) return [];
      seen.add(url.href);
      return [{ url: url.href, title: text(source.title, 300) || url.hostname, description: text(source.description) }];
    });
  }
  function files(input, depth = 0) {
    if (!Array.isArray(input) || depth >= ACTIVITY_LIMITS.depth) return [];
    return input.slice(0, ACTIVITY_LIMITS.entries).flatMap(node => {
      if (fileBudget <= 0 || !record(node) || !['file','folder'].includes(node.type)) return [];
      const name = text(node.name, 300);
      if (!name) return [];
      fileBudget--;
      return [{ name, path: text(node.path, 1000), type: node.type, children: node.type === 'folder' ? files(node.children, depth + 1) : [] }];
    });
  }
  function events(input, depth = 0) {
    if (!Array.isArray(input) || depth >= ACTIVITY_LIMITS.depth) return [];
    return input.slice(0, ACTIVITY_LIMITS.events).flatMap(event => {
      if (remaining <= 0 || !record(event) || !kinds.has(event.kind)) return [];
      const title = text(event.title, ACTIVITY_LIMITS.title);
      if (!title) return [];
      remaining--;
      const result = { id: text(event.id, 200), kind: event.kind, title, status: status(event.status), detail: text(event.detail), code: text(event.code), items: [], sources: sources(event.sources), files: files(event.files), suggestions: [], children: [] };
      if (Array.isArray(event.items)) result.items = event.items.slice(0, ACTIVITY_LIMITS.entries).flatMap(item => {
        if (!record(item)) return [];
        const title = text(item.title, 300);
        return title ? [{ title, status: status(item.status), detail: text(item.detail) }] : [];
      });
      if (Array.isArray(event.suggestions)) result.suggestions = event.suggestions.slice(0, 20).flatMap(value => { const s = text(value, 2000); return s ? [s] : []; });
      if (event.kind === 'context') {
        const usage = event.usage;
        if (!record(usage) || !number(usage.used) || !number(usage.limit) || usage.limit === 0) return [];
        result.usage = { used: usage.used, limit: usage.limit };
        for (const field of ['input', 'output', 'reasoning']) if (number(usage[field])) result.usage[field] = usage[field];
      }
      result.children = events(event.children, depth + 1);
      return [result];
    });
  }
  return events(input);
}
