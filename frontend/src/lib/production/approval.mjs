/** Whether a clip's notes (render output qa_json) hold an approval. */
export function clipApproved(qa) {
  let notes = qa;
  if (typeof notes === 'string') {
    try {
      notes = JSON.parse(notes);
    } catch {
      return false;
    }
  }
  return !!notes && typeof notes === 'object' && notes.approval?.status === 'approved';
}
