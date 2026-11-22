// Shared helpers for turning caught errors into text the UI can show.

const NO_DETAIL = 'Something went wrong (no detail provided).';

/**
 * The message carried by a caught value: an Error's message, a string as-is,
 * or `fallback` when there is nothing usable.
 */
export function errorMessage(e: unknown, fallback = NO_DETAIL): string {
  if (typeof e === 'string') return e || fallback;
  if (e && typeof e === 'object' && 'message' in e && typeof e.message === 'string') return e.message || fallback;
  return fallback;
}

/**
 * Map a raw provider/transport error onto a short, human-friendly line.
 * Falls back to the original text (capped) when nothing matches, so a real
 * message is never hidden; only the noisy ones are softened.
 */
export function friendlyError(e: unknown, fallback = NO_DETAIL): string {
  const text = errorMessage(e, '').trim();
  if (!text) return fallback;
  const t = text.toLowerCase();
  const has = (...needles: string[]) => needles.some((n) => t.includes(n));
  if (has('timeout', 'timed out'))
    return 'The provider took too long to respond — try again.';
  if (has('rate limit', '429', 'too many requests'))
    return 'Rate limited by the provider — wait a moment and retry.';
  if (has('insufficient') && has('balance', 'credit', 'quota'))
    return 'The provider account is out of credit/quota.';
  if (has('401', 'unauthorized', 'api key', 'forbidden', '403'))
    return 'Provider rejected the API key — check Settings.';
  if (has('content') && has('policy', 'moderation', 'blocked', 'safety'))
    return 'The prompt was blocked by the provider content filter.';
  if (has('download failed'))
    return 'The finished video could not be downloaded — try again.';
  if (has('ffmpeg', 'no frames'))
    return 'Could not process the video file (ffmpeg).';
  if (has('connection', 'network', 'econn', 'failed to fetch'))
    return 'Network error reaching the provider — check the connection.';
  if (has('500', 'internal server error', 'bad gateway', '502', '503'))
    return 'The provider had a server error — try again shortly.';
  // Unknown: surface the original, capped so compact lists stay tidy.
  return text.length > 160 ? text.slice(0, 157) + '…' : text;
}
