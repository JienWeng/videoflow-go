/** Self-check: after the app makes a picture or a clip, the local AI session looks at it
 * (internal/api/selfcheck.go). The result is stored on the asset (metadata_json.self_check)
 * or the clip (qa_json.self_check). These helpers read it safely and say it in plain words.
 */

const VERDICTS = new Set(['pass', 'fix', 'fail']);
const SEVERITIES = new Set(['low', 'medium', 'high']);

/** The stored check, or null when there is none or it is unreadable.
 * @param {unknown} raw a self_check record, or the metadata/qa object holding one
 * @returns {import('./selfcheck.d.mts').SelfCheck|null} */
export function readSelfCheck(raw) {
  const holder = raw && typeof raw === 'object' ? /** @type {Record<string, any>} */ (raw) : null;
  const r = holder && holder.self_check && typeof holder.self_check === 'object' ? holder.self_check : holder;
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
    engine: typeof r.engine === 'string' ? r.engine : ''
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
