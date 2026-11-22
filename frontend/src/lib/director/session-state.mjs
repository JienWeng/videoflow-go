export function sessionState(session, health) {
  const state = (label, help, canContinue = false, needsSetup = false) => ({ label, help, canContinue, needsSetup });
  if (!session) return state('Choose a session', 'Create or select a session for this project.');
  if (session.status === 'running') return state('Planning in progress', 'Your current request is still running. Refresh to check its progress.');
  if (!health) return state('Checking AI setup', 'Checking whether your selected AI is installed.');
  if (health.all_text_local === false) return state('AI sessions disabled', 'Enable local planning in the backend configuration.', false, true);
  if (!health.engines?.some(item => item.engine === session.engine && item.installed === true)) {
    return state('AI setup required', 'Your session is saved. Set up the selected AI to start planning.', false, true);
  }
  if (health.engines?.some(item => item.engine === session.engine && item.signed_in === false)) {
    const name = session.engine === 'claude' ? 'Claude Code' : 'Codex';
    return state('Sign-in needed', `Connect ${name} to start planning. It takes about a minute.`, false, true);
  }
  if (session.status !== 'ready') return state('Session unavailable', 'Refresh to check this session’s latest status.');
  const connected = health.engines?.some(item => item.engine === session.engine && item.signed_in === true);
  return state(session.native_id ? 'Session selected' : 'Session saved', connected ? 'Your AI is connected. Continue in Studio to start planning.' : 'AI sign-in and account access are checked when you send your first prompt.', true);
}
