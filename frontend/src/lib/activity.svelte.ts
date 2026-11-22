/**
 * Global activity store: merged recent background ops + render jobs, kept
 * fresh via the SSE stream. Singleton — the root layout calls initActivity()
 * once; everything else reads `activity.items` / `activity.runningCount`.
 */
import { get } from '$lib/api';
import { subscribeJobs } from '$lib/sse';
import type { Op } from '$lib/ops';
import type { RenderJob } from '$lib/api-types';

export type ActivityItem = {
  id: string;
  /** 'render' for render jobs, otherwise the op kind (storyboard | assets | shots | caption | style_ingest). */
  kind: string;
  /** pending | running | succeeded | failed, or another backend state such as reconciliation_required. */
  status: string;
  scene_id?: string | null;
  output_id?: string | null;
  error?: string | null;
  created_at: string;
};

const MAX_ITEMS = 15;

const store = $state({ items: [] as ActivityItem[] });

let initialized = false;
let unsubscribe: (() => void) | null = null;

async function refresh(): Promise<void> {
  try {
    const [ops, jobs] = await Promise.all([
      get<(Op & { created_at: string })[]>(`/ops?limit=${MAX_ITEMS}`),
      get<RenderJob[]>('/render-jobs')
    ]);
    const merged: ActivityItem[] = [
      ...ops.map((o) => ({
        id: o.id,
        kind: o.kind ?? 'op',
        status: o.status,
        scene_id: o.scene_id,
        output_id: o.output_id,
        error: o.error,
        created_at: o.created_at
      })),
      ...jobs.map((j) => ({
        id: j.id,
        kind: 'render',
        status: j.status,
        scene_id: j.scene_id,
        output_id: null,
        error: j.error,
        created_at: j.created_at
      }))
    ];
    merged.sort((a, b) => (a.created_at < b.created_at ? 1 : -1));
    store.items = merged.slice(0, MAX_ITEMS);
  } catch {
    /* transient — next SSE event or poll retries */
  }
}

/** Idempotent init: fetch once and follow the SSE stream. Returns a cleanup (HMR safety). */
export function initActivity(): () => void {
  if (initialized) return () => {};
  initialized = true;
  void refresh();
  unsubscribe = subscribeJobs(
    () => void refresh(),
    () => void refresh()
  );
  return () => {
    unsubscribe?.();
    unsubscribe = null;
    initialized = false;
  };
}

export const activity = {
  get items(): ActivityItem[] {
    return store.items;
  },
  get runningCount(): number {
    return store.items.filter((i) => i.status === 'running' || i.status === 'pending').length;
  }
};
