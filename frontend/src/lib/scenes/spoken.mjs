/** @typedef {{ id: string, character_id: string, language: string, text: string, visible: boolean, shot_id?: string, start_hint?: number }} SpokenLine */

/**
 * The lines a scene's script has its characters or narrator speak, as the backend saves
 * them in scene_json.spoken_lines. Anything unreadable is left out.
 * @param {{ scene_json?: Record<string, unknown> | null } | null | undefined} scene
 * @returns {SpokenLine[]}
 */
export function sceneSpokenLines(scene) {
  const raw = scene?.scene_json?.spoken_lines;
  if (!Array.isArray(raw)) return [];
  return raw
    .filter(line => line && typeof line === 'object')
    .map(line => ({
      id: String(line.id ?? ''),
      character_id: String(line.character_id ?? ''),
      language: String(line.language ?? ''),
      text: String(line.text ?? ''),
      visible: !!line.visible,
      // The shot it is spoken in, when one was chosen; lines without one are placed by time.
      ...(line.shot_id ? { shot_id: String(line.shot_id) } : {}),
      ...(typeof line.start_hint === 'number' && Number.isFinite(line.start_hint) ? { start_hint: line.start_hint } : {})
    }));
}

/**
 * Seconds with at most one decimal: 7.2 s, 8 s.
 * @param {number} value
 */
export function secondsText(value) {
  const v = Math.round((Number(value) || 0) * 10) / 10;
  return `${Number.isInteger(v) ? v : v.toFixed(1)} s`;
}

/**
 * The fit bar of some speech in some seconds: how full it is (never past 100) and its
 * tone. Above 90 % is tight, past the end is too long.
 * @param {number} used
 * @param {number} seconds
 * @param {string} [verdict] the server's verdict, when known
 * @returns {{ percent: number, tone: 'ok' | 'tight' | 'too_long' | 'empty', label: string }}
 */
export function fitBar(used, seconds, verdict) {
  const percent = seconds > 0 ? Math.min(100, Math.round((used / seconds) * 100)) : used > 0 ? 100 : 0;
  /** @type {'ok' | 'tight' | 'too_long' | 'empty'} */
  let tone = 'ok';
  if (!used) tone = 'empty';
  else if (verdict === 'too_long' || (!verdict && used > seconds)) tone = 'too_long';
  else if (verdict === 'tight' || (!verdict && used > seconds * 0.9)) tone = 'tight';
  return { percent, tone, label: `${secondsText(used)} of ${secondsText(seconds)}` };
}

/**
 * How much a writer should aim for in a shot: "8–10 characters", "5–6 words".
 * @param {{ unit: string, low: number, high: number } | null | undefined} budget
 */
export function budgetText(budget) {
  if (!budget || !budget.high) return '';
  const low = Math.max(1, budget.low);
  return low >= budget.high ? `about ${budget.high} ${budget.unit}` : `${low}–${budget.high} ${budget.unit}`;
}

/**
 * Who says a line: the narrator when no character is set.
 * @param {{ character_id?: string | null }} line
 * @param {readonly { id: string, name: string }[]} characters
 */
export function speakerLabel(line, characters) {
  if (!line.character_id) return 'Narrator';
  return characters.find(c => c.id === line.character_id)?.name || 'Unknown character';
}

/**
 * A short read-only description of a scene's spoken script for lists and cards; empty
 * when nothing is said, so scenes not yet written show nothing.
 * @param {readonly SpokenLine[]} lines
 * @param {readonly { id: string, name: string }[]} characters
 */
export function spokenSummary(lines, characters) {
  if (!lines.length) return '';
  const first = `${speakerLabel(lines[0], characters)}: ${lines[0].text}`;
  return lines.length === 1 ? first : `${lines.length} lines · ${first}`;
}

/**
 * The lines with one moved up (-1) or down (1); out-of-range moves change nothing.
 * @template T
 * @param {readonly T[]} lines
 * @param {number} index
 * @param {-1 | 1} dir
 * @returns {T[]}
 */
export function moveLine(lines, index, dir) {
  const next = [...lines];
  const to = index + dir;
  if (index < 0 || index >= next.length || to < 0 || to >= next.length) return next;
  [next[index], next[to]] = [next[to], next[index]];
  return next;
}
