/** How the editor speaks about timing taken from the clips' own sound
 * (internal/api/script_captions.go, voiceover.go). Pure helpers, tested in speech.test.mjs.
 */

/** The note after "Captions from script": which captions follow the speech heard in
 * the clips, which keep an estimate, and which show what was said instead of the script.
 * @param {{heard?: number, estimated?: number, voice?: number, changed?: number}|undefined} timing
 * @returns {string} */
export function captionTimingNote(timing) {
  const t = { heard: 0, estimated: 0, voice: 0, changed: 0, ...(timing ?? {}) };
  const parts = [];
  if (t.voice) parts.push(`${t.voice} follow${t.voice === 1 ? 's' : ''} the voice lines`);
  if (t.heard) parts.push(`${t.heard} follow${t.heard === 1 ? 's' : ''} the speech heard in the clips`);
  if (t.estimated)
    parts.push(
      `${t.estimated} use${t.estimated === 1 ? 's' : ''} estimated shot timing` +
        (t.heard || t.voice ? '' : ' (clips without their own sound, or no Listening key in AI setup)')
    );
  let note = parts.length ? 'Timing: ' + parts.join(', ') + '.' : '';
  if (t.changed)
    note += ` ${t.changed} caption${t.changed === 1 ? ' shows' : 's show'} what was actually said, because it differs from the script.`;
  return note.trim();
}

/** Mutes or restores the video clips that speak with their own voices, and lowers music
 * under voice lines when asked. Voice lines and other clips are left alone.
 * @template {{id: string, asset_id: string, muted?: boolean, volume?: number|null}} C
 * @param {C[]} clips @param {C[]} audio
 * @param {{mode?: string, native?: string[], mute?: string[], lowerMusic?: boolean}} options
 * @returns {{clips: C[], audio: C[]}} */
export function applyVoiceSound(clips, audio, options) {
  const mute = new Set(options.mute ?? []);
  const native = new Set(options.native ?? []);
  const nextClips = clips.map(c => {
    if (mute.has(c.id)) return { ...c, muted: true };
    // Keeping the clip's own voices: a clip muted by an earlier "replace" speaks again.
    if (options.mode === 'keep' && native.has(c.id) && c.muted) return { ...c, muted: false };
    return c;
  });
  const nextAudio = options.lowerMusic
    ? audio.map(a => (String(a.asset_id).startsWith('voiceover_') ? a : { ...a, volume: Math.min(a.volume ?? 1, 0.25) }))
    : audio;
  return { clips: nextClips, audio: nextAudio };
}

/** The note after voice lines were made.
 * @param {{items?: unknown[], generated?: number, reused?: number, shifted?: number, overrun?: number,
 *   mode?: string, kept_native?: number, mute_clip_ids?: string[], too_long?: {fits_with_speed_up?: boolean}[],
 *   sped_up?: number, speed_up_limit?: number, skipped?: {reason: string}[]}} r
 * @returns {string} */
export function voiceoverSummary(r) {
  const items = r.items?.length ?? 0;
  const parts = [];
  if (items)
    parts.push(
      `Added ${items} voice line${items === 1 ? '' : 's'}` +
        (r.generated ? ` (${r.generated} new` + (r.reused ? `, ${r.reused} reused)` : ')') : ' (all reused, no charge)') +
        '.'
    );
  if (r.mode === 'replace' && r.mute_clip_ids?.length)
    parts.push(
      `Muted the own sound of ${r.mute_clip_ids.length} clip${r.mute_clip_ids.length === 1 ? '' : 's'} so only one voice speaks; lines start when the characters say them in the clip.`
    );
  if (r.mode === 'keep' && r.kept_native)
    parts.push(`Kept the clips' own voices for ${r.kept_native} line${r.kept_native === 1 ? '' : 's'}.`);
  if (r.sped_up) parts.push(`Sped up ${r.sped_up} line${r.sped_up === 1 ? '' : 's'} slightly to fit.`);
  const long = r.too_long ?? [];
  if (long.length)
    parts.push(
      `${long.length} line${long.length === 1 ? ' is' : 's are'} longer than the time the character speaks` +
        (long.some(l => l.fits_with_speed_up) ? '' : ' even at the fastest allowed speed') +
        (r.shifted ? ', so the next line starts later.' : '.')
    );
  else if (r.shifted) parts.push(`Moved ${r.shifted} later so lines don't overlap.`);
  if ((r.overrun ?? 0) > 0.5)
    parts.push(`The voice runs ${r.overrun?.toFixed(1)}s past the video: shorten the script, pick a faster voice, or add a closing shot.`);
  if (r.skipped?.length) parts.push(`Skipped ${r.skipped.length}: ${r.skipped[0].reason}`);
  return parts.join(' ');
}

/** Whether to offer "Speed up" for the lines that do not fit.
 * @param {{too_long?: {fits_with_speed_up?: boolean}[]}} r */
export function canSpeedUp(r) {
  return (r.too_long ?? []).some(l => l.fits_with_speed_up);
}
