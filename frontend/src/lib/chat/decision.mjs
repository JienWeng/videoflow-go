/** Ask or decide: each assistant reply carries intent.decision (internal/api/chat_decision.go).
 * mode "act" goes ahead, "ask" waits for one answer (its options are quick replies),
 * "inform" only answers. These helpers read it safely, also from older saved chats
 * that have no decision.
 */
const MODES = new Set(['act', 'ask', 'inform']);

/** @param {unknown} raw @returns {import('./decision.d.mts').ChatDecision} */
export function normalizeDecision(raw) {
  const d = raw && typeof raw === 'object' ? /** @type {Record<string, any>} */ (raw) : {};
  const mode = MODES.has(d.mode) ? d.mode : 'inform';
  const assumptions = Array.isArray(d.assumptions) ? d.assumptions.filter(a => typeof a === 'string' && a.trim()).map(a => a.trim()) : [];
  const question = mode === 'ask' && typeof d.question === 'string' ? d.question.trim() : '';
  const options = mode === 'ask' && Array.isArray(d.options)
    ? d.options
        .filter(o => o && typeof o.label === 'string' && o.label.trim())
        .map(o => ({ label: o.label.trim(), value: typeof o.value === 'string' && o.value.trim() ? o.value.trim() : o.label.trim() }))
    : [];
  return {
    mode: mode === 'ask' && !question ? 'inform' : mode,
    assumptions,
    question,
    options,
    ...(typeof d.key === 'string' && d.key ? { key: d.key } : {}),
    ...(typeof d.overruled === 'string' && d.overruled ? { overruled: d.overruled } : {})
  };
}

/** The muted line under a reply: the choices made without asking. "" when none.
 * @param {unknown} raw */
export function assumptionLine(raw) {
  const { assumptions } = normalizeDecision(raw);
  return assumptions.length ? `Assumed: ${assumptions.join(' · ')}` : '';
}

/** Whether the reply waits for the user's answer, so its button must not start by itself.
 * @param {unknown} raw */
export function isAsking(raw) {
  return normalizeDecision(raw).mode === 'ask';
}

/** The quick replies of a question, and what picking one tells the server to remember.
 * @param {unknown} raw @returns {{label: string; send: string; answer: {key?: string; question: string; label: string; value: string}}[]} */
export function answerChips(raw) {
  const d = normalizeDecision(raw);
  if (d.mode !== 'ask') return [];
  return d.options.map(o => ({
    label: o.label,
    send: o.value,
    answer: { ...(d.key ? { key: d.key } : {}), question: d.question, label: o.label, value: o.value }
  }));
}

/** The decision-log key a dismissed proposal is kept under: rejected:<action>:<target>,
 * where the target is the scene, character, finished video or story it was for, or
 * "project". "" for a reply with nothing to reject.
 * @param {{action?: string; scene_id?: string|null; character_id?: string|null; output_id?: string|null; script_id?: string|null; scene_ids?: string[]}|undefined|null} intent */
export function rejectionKey(intent) {
  const action = typeof intent?.action === 'string' ? intent.action : '';
  if (!/^[a-z][a-z0-9_]*$/.test(action) || action === 'unknown') return '';
  const target = intent?.scene_id || intent?.character_id || intent?.output_id || intent?.script_id || (intent?.scene_ids?.length === 1 ? intent.scene_ids[0] : '') || 'project';
  const safe = String(target).replace(/[^A-Za-z0-9_.-]/g, '_').slice(0, 60);
  return `rejected:${action}:${safe || 'project'}`;
}

const LABELS = /** @type {Record<string,string>} */ ({
  language: 'Language', style_id: 'Visual style', aspect_ratio: 'Format', target_duration: 'Length', platform: 'Platform',
  music: 'Music', conversation_mode: 'Voices or narration', tone: 'Tone', audience: 'Audience', resolution: 'Resolution',
  notes: 'Note', reviewed: 'Reviewed'
});
const PREFIXES = /** @type {Record<string,string>} */ ({
  voice: 'Voice', voice_sample: 'Voice sample', rejected: 'Turned down', choice: 'Chose', character: 'Character', scene: 'Scene', notes: 'Note'
});

/** A decision-log key in plain words, naming a character by its name when known.
 * @param {string} key @param {{id: string; name?: string; title?: string}[]} [named] */
export function decisionLabel(key, named = []) {
  if (LABELS[key]) return LABELS[key];
  const [prefix, ...rest] = key.split(':');
  const head = PREFIXES[prefix];
  if (!head) return key.replace(/_/g, ' ');
  if (prefix === 'choice') return `${head}: ${rest.join(' ').replace(/_/g, ' ')}`;
  if (prefix === 'rejected') {
    const [action = '', target = ''] = rest;
    const item = named.find(n => n.id === target);
    return `${head}: ${action.replace(/_/g, ' ')}${item ? ` for ${item.name ?? item.title}` : ''}`;
  }
  const item = named.find(n => n.id === rest.join(':'));
  return `${head}${item ? ` · ${item.name ?? item.title}` : ''}`;
}
