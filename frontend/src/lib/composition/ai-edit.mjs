/** An AI edit proposal the owner has not accepted is not the project's current edit. */
/** @param {any} saved */
export function isPendingProposal(saved) {
  return !!saved?.ai_edit && saved.ai_edit.status !== 'accepted';
}

/** The newest saved edit that is not a pending AI edit proposal (null when none). */
/** @param {any[] | null | undefined} saved */
export function latestEdit(saved) {
  const current = (saved ?? []).filter(e => !isPendingProposal(e));
  current.sort((a, b) => String(b.created_at).localeCompare(String(a.created_at)));
  return current[0] ?? null;
}

/** A saved edit's label in the list of saved edits. */
/** @param {any} saved */
export function savedEditLabel(saved) {
  const note = saved?.ai_edit;
  if (!note) return '';
  const status = /** @type {Record<string, string>} */ ({ proposed: 'waiting for you', accepted: 'accepted', discarded: 'discarded' })[note.status] ?? note.status;
  return `${note.label}${note.pass === 2 ? ' (refined by ear)' : ''} · ${status}`;
}

/** One line on how an AI edit run ended, for its card. */
/** @param {any} result */
export function aiEditHeadline(result) {
  switch (result?.status) {
    case 'proposed':
      return 'The AI editor has a proposal. Look at it on the timeline, then accept or discard it.';
    case 'accepted':
      return result.video_url ? 'The AI edit passed every check, was kept and exported.' : 'The AI edit passed every check and was kept.';
    case 'rejected':
      return 'The AI edit was not kept: it broke a rule your film depends on. Your edit is unchanged.';
    case 'unchanged':
      return 'The AI editor proposed an edit you already have. Nothing changed.';
    default:
      return '';
  }
}
