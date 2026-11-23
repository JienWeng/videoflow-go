/** Self-check: after the app makes a picture or a clip, the local AI session looks at it
 * (internal/api/selfcheck.go). The result is stored on the asset (metadata_json.self_check)
 * or the clip (qa_json.self_check). The speech check (kind clip_audio,
 * internal/api/audio_check.go) listens to a clip made with its own sound and is stored as
 * qa_json.audio_check in the same shape, with each line as heard. These helpers read
 * them safely and say them in plain words.
 */

const LINE_STATUSES = new Set(['ok', 'changed', 'missing', 'cut_off']);

/** Where a kind's result is kept on the asset or clip. @param {string} kind */
export function checkKey(kind) {
  return kind === 'clip_audio' ? 'audio_check' : 'self_check';
}

/** The check's name for people. @param {string} kind */
export function checkName(kind) {
  return kind === 'clip_audio' ? 'Speech check' : 'Self-check';
}

/** Who did the check. @param {string} engine */
export function engineName(engine) {
  return (
    /** @type {Record<string, string>} */ ({ claude: 'Claude Code', codex: 'Codex', openai: 'OpenAI', gemini: 'Google Gemini' })[engine] ?? engine
  );
}

/** @param {unknown} v */
const seconds = v => (typeof v === 'number' && Number.isFinite(v) ? v : null);

/** The speech check's detail, or null. @param {any} audio */
function readAudio(audio) {
  if (!audio || typeof audio !== 'object' || !Array.isArray(audio.lines)) return null;
  return {
    lines: audio.lines
      .filter((/** @type {any} */ l) => l && typeof l.text === 'string')
      .map((/** @type {any} */ l) => ({
        text: l.text,
        heard: typeof l.heard === 'string' ? l.heard : '',
        similarity: Number.isFinite(l.similarity) ? Math.max(0, Math.min(1, l.similarity)) : 0,
        start: seconds(l.start),
        end: seconds(l.end),
        status: LINE_STATUSES.has(l.status) ? l.status : 'missing'
      })),
    heardLanguage: typeof audio.heard_language === 'string' ? audio.heard_language : '',
    expectedLanguage: typeof audio.expected_language === 'string' ? audio.expected_language : '',
    cost: seconds(audio.cost) ?? 0
  };
}

/** One line per script line of a speech check: when it is said and how close it is.
 * @param {import('./selfcheck.d.mts').SelfCheck|null} c @returns {string[]} */
export function heardLines(c) {
  if (!c?.audio) return [];
  return c.audio.lines.map((l, i) => {
    const at = l.start !== null && l.end !== null ? ` at ${l.start.toFixed(1)}–${l.end.toFixed(1)} s` : '';
    const alike = `${Math.round(l.similarity * 100)}% as written`;
    switch (l.status) {
      case 'ok':
        return `Line ${i + 1}: said${at} (${alike}).`;
      case 'changed':
        return `Line ${i + 1}: heard “${l.heard}”${at} instead of “${l.text}” (${alike}).`;
      case 'cut_off':
        return l.heard ? `Line ${i + 1}: cut off${at}, heard “${l.heard}”.` : `Line ${i + 1}: never reached before the clip ended.`;
      default:
        return `Line ${i + 1}: not heard (“${l.text}”).`;
    }
  });
}

const VERDICTS = new Set(['pass', 'fix', 'fail']);
const SEVERITIES = new Set(['low', 'medium', 'high']);

/** The stored check, or null when there is none or it is unreadable.
 * @param {unknown} raw a check record, or the metadata/qa object holding one under key
 * @param {string} [key] self_check (default) or audio_check
 * @returns {import('./selfcheck.d.mts').SelfCheck|null} */
export function readSelfCheck(raw, key = 'self_check') {
  const holder = raw && typeof raw === 'object' ? /** @type {Record<string, any>} */ (raw) : null;
  const r = holder && holder[key] && typeof holder[key] === 'object' ? holder[key] : holder;
  const result = r && typeof r.result === 'object' && r.result ? r.result : null;
  if (!result || !VERDICTS.has(result.verdict)) return null;
  const issues = Array.isArray(result.issues)
    ? result.issues
        .filter((/** @type {any} */ i) => i && typeof i.detail === 'string' && i.detail.trim())
        .map((/** @type {any} */ i) => ({
          code: typeof i.code === 'string' ? i.code : '',
          severity: SEVERITIES.has(i.severity) ? i.severity : 'low',
          detail: i.detail.trim()
        }))
    : [];
  const score = Number.isFinite(result.score) ? Math.max(0, Math.min(100, Math.round(result.score))) : 0;
  return {
    verdict: result.verdict,
    score,
    issues,
    suggestion: typeof result.suggestion === 'string' ? result.suggestion.trim() : '',
    change: typeof result.retry?.change === 'string' ? result.retry.change.trim() : '',
    note: typeof result.note === 'string' ? result.note.trim() : '',
    unavailable: result.unavailable === true,
    accepted: typeof r.accepted_at === 'string' && r.accepted_at !== '',
    attempt: Number.isInteger(r.attempt) ? r.attempt : 1,
    checkedAt: typeof r.checked_at === 'string' ? r.checked_at : '',
    engine: typeof r.engine === 'string' ? r.engine : '',
    audio: readAudio(result.audio)
  };
}

/** Whether the findings still need the user: a fix or fail that was not kept.
 * @param {import('./selfcheck.d.mts').SelfCheck|null} c */
export function needsAttention(c) {
  return !!c && !c.unavailable && !c.accepted && c.verdict !== 'pass';
}

/** The badge's words and tone.
 * @param {import('./selfcheck.d.mts').SelfCheck|null} c
 * @returns {{label: string; tone: 'ok'|'warn'|'bad'|'muted'}} */
export function selfCheckBadge(c) {
  if (!c) return { label: 'Not checked', tone: 'muted' };
  if (c.unavailable) return { label: 'Check unavailable', tone: 'muted' };
  if (c.verdict === 'pass') return { label: `Checked · ${c.score}`, tone: 'ok' };
  if (c.accepted) return { label: 'Kept despite check', tone: 'muted' };
  return c.verdict === 'fix' ? { label: 'Needs a fix', tone: 'warn' } : { label: 'Check failed', tone: 'bad' };
}

/** One line per finding, most important first, for the expandable list.
 * @param {import('./selfcheck.d.mts').SelfCheck|null} c @returns {string[]} */
export function findingLines(c) {
  if (!c) return [];
  const rank = { high: 0, medium: 1, low: 2 };
  return [...c.issues]
    .sort((a, b) => rank[a.severity] - rank[b.severity])
    .map(i => (i.severity === 'high' ? `${i.detail}` : `${i.detail} (${i.severity})`));
}

/** The kind the server expects for an asset type, or '' when it is not checked.
 * @param {string|undefined|null} assetType */
export function selfCheckKindFor(assetType) {
  if (assetType === 'character_reference') return 'character_sheet';
  if (assetType === 'storyboard') return 'scene_picture';
  return '';
}
