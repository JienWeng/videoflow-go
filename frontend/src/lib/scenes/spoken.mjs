/** @typedef {{ id: string, character_id: string, language: string, text: string, visible: boolean }} SpokenLine */

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
      visible: !!line.visible
    }));
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
