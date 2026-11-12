export function sessionState(session, health) {
  const state = (label, help, canContinue = false, needsSetup = false) => ({ label, help, canContinue, needsSetup });
  if (!session) return state('Choose a session', 'Create or select a session for this project.');
  if (session.status === 'running') return state('Planning in progress', 'Your current request is still running. Refresh to check its progress.');
  if (session.status === 'review_required') return state('Review needed', 'Review the interrupted request in History before continuing.');
  if (!health) return state('Checking AI setup', 'Checking whether your selected AI is installed.');
  if (health.all_text_local === false) return state('AI sessions disabled', 'Enable local planning in the backend configuration.', false, true);
  if (!health.engines?.some(item => item.engine === session.engine && item.installed === true)) {
    return state('AI setup required', 'Your session is saved. Set up the selected AI to start planning.', false, true);
  }
  if (session.status !== 'ready') return state('Session unavailable', 'Refresh to check this session’s latest status.');
  return state(session.native_id ? 'Session selected' : 'Session saved', 'AI sign-in and account access are checked when you send your first prompt.', true);
}
