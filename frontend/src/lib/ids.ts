/**
 * A random id that also works on plain-http pages. Browsers hide crypto.randomUUID()
 * unless the page is https or localhost, e.g. when the app is opened over a Tailscale
 * address, and the missing function would stop the whole action before it starts.
 */
export function randomId(): string {
  if (typeof crypto.randomUUID === 'function') return crypto.randomUUID();
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  return Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('');
}
