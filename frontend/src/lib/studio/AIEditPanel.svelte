<script lang="ts">
  // The AI editor (internal/api/ai_edit.go): a separate local AI session edits the
  // finished film and proposes a new edit; VideoFlow checks it and saves it as a
  // revision labelled "AI edit proposal". Here the owner starts it, follows its stage,
  // reads what it changed and accepts, discards or opens it on the timeline.
  import { onDestroy, onMount } from 'svelte';
  import { get, post } from '$lib/api';
  import { runBackgroundOp, type Op } from '$lib/ops';
  import { errorMessage } from '$lib/errors';
  import { Button } from '$lib/components/ui/button';
  import { LoaderCircle, Sparkles } from '@lucide/svelte';
  import { aiEditHeadline } from '$lib/composition/ai-edit.mjs';
  import type { AIEditNote, AIEditResult } from '$lib/composition/types';

  let {
    scriptId,
    revision = '',
    proposal = '',
    auto = false,
    onopen,
    onstarted
  }: {
    /** The story whose film is edited. */
    scriptId: string;
    /** The saved edit to improve; empty takes the project's current edit. */
    revision?: string;
    /** A proposal to review instead of starting a new run. */
    proposal?: string;
    /** Full auto: start at once, keep the proposal when every check passes, export. */
    auto?: boolean;
    /** Open a saved edit on the timeline. */
    onopen?: (revision: string) => void;
    /** Called once a run starts (chat: the card no longer starts by itself). */
    onstarted?: () => void;
  } = $props();

  let running = $state(false);
  let stage = $state('');
  let error = $state('');
  let result = $state<AIEditResult | null>(null);
  let note = $state<AIEditNote | null>(null);
  let deciding = $state(false);
  let listening = $state(false);
  let heard = $state(false);
  let poll: ReturnType<typeof setInterval> | undefined;
  onDestroy(() => clearInterval(poll));

  const shown = $derived(result?.revision ?? proposal);
  const status = $derived(note?.status ?? result?.status ?? '');
  const lines = $derived(result?.diff?.lines ?? note?.diff?.lines ?? []);
  const summary = $derived(result?.summary ?? note?.summary ?? '');
  const headline = $derived(status === 'discarded' ? '' : aiEditHeadline({ ...(result ?? {}), status }));
  const refined = $derived((result?.pass ?? note?.pass ?? 1) >= 2 || !!note?.refined_by);

  onMount(() => {
    if (proposal) void loadNote(proposal);
    else if (auto) void start();
  });

  async function loadNote(rev: string) {
    try {
      const r = await get<{ note: AIEditNote; listen?: unknown }>(`/compositions/${encodeURIComponent(rev)}/ai-edit`);
      note = r.note;
      heard = !!r.listen;
    } catch (e) {
      error = errorMessage(e);
    }
  }

  // The op's stage, read while it runs (the op stream only says when it ends).
  function follow() {
    clearInterval(poll);
    poll = setInterval(async () => {
      try {
        const ops = await get<Op<AIEditResult>[]>('/ops?kind=ai_edit&limit=1');
        const op = ops[0];
        if (op?.status === 'running' && op.result_json?.stage) stage = op.result_json.stage;
      } catch {
        /* the stage line is a nicety; completion still arrives */
      }
    }, 4000);
  }

  async function start(refine = false) {
    running = true;
    error = '';
    stage = 'Starting the AI editor';
    onstarted?.();
    follow();
    const body = refine ? { revision: shown, refine: true } : { revision, auto_accept: auto, export: auto };
    const settle = async (op: Op<AIEditResult>) => {
      clearInterval(poll);
      running = false;
      result = op.result_json ?? null;
      note = null;
      heard = false;
      if (op.status !== 'succeeded' && !result?.reasons?.length) error = op.error || 'The AI editor stopped.';
      if (result?.revision && (result.status === 'proposed' || result.status === 'accepted')) await loadNote(result.revision);
    };
    try {
      await runBackgroundOp<AIEditResult>(`/scripts/${encodeURIComponent(scriptId)}/ai-edit`, body, {
        label: refine ? 'AI edit (refine by ear)' : 'AI edit',
        onDone: settle,
        onFail: settle
      });
    } catch (e) {
      clearInterval(poll);
      running = false;
      error = errorMessage(e);
    }
  }

  async function decide(decision: 'accept' | 'discard') {
    if (!shown) return;
    deciding = true;
    error = '';
    try {
      const r = await post<{ note: AIEditNote; open_revision: string }>(`/compositions/${encodeURIComponent(shown)}/ai-edit`, { decision });
      note = r.note;
      if (decision === 'accept') onopen?.(shown);
      else if (r.open_revision && r.open_revision !== shown) onopen?.(r.open_revision);
    } catch (e) {
      error = errorMessage(e);
    } finally {
      deciding = false;
    }
  }

  async function listen() {
    if (!shown) return;
    listening = true;
    error = '';
    try {
      await runBackgroundOp(`/compositions/${encodeURIComponent(shown)}/listen`, {}, {
        label: 'Listening to the film',
        onDone: () => {
          heard = true;
          listening = false;
        },
        onFail: op => {
          error = op.error || 'Listening failed.';
          listening = false;
        }
      });
    } catch (e) {
      listening = false;
      error = errorMessage(e);
    }
  }
</script>

<div class="grid gap-2 text-sm" data-testid="ai-edit">
  {#if !shown && !running && !result}
    <p class="text-xs text-muted-foreground">
      A separate local AI session edits the finished film for a better look and sound: trims and order, titles,
      motion overlays, caption style, sound effects, ambience and volumes. It never cuts a spoken line or moves a
      caption off its heard time, and nothing changes until you accept. Free: turns of your local AI session and
      local CPU time, no credits. It can take up to 20 minutes.
    </p>
    <div><Button size="sm" class="rounded-full px-4" disabled={!scriptId} onclick={() => start()}><Sparkles
      class="size-4"/>Start the AI edit</Button></div>
  {/if}

  {#if running}
    <p class="flex items-center gap-2 text-xs" role="status"><LoaderCircle class="size-4 animate-spin"/>{stage || 'Working…'}</p>
  {/if}

  {#if headline}<p class="text-xs font-medium">{headline}</p>{/if}
  {#if status === 'discarded'}<p class="text-xs font-medium">Discarded. The edit before it is your current edit again.</p>{/if}
  {#if summary}<p class="text-xs text-muted-foreground">{summary}</p>{/if}
  {#if lines.length}
    <ul class="list-disc pl-5 text-xs">{#each lines as line (line)}<li>{line}</li>{/each}</ul>
  {/if}
  {#if result?.reasons?.length}
    <ul class="list-disc pl-5 text-xs text-destructive" aria-label="Why it was not kept">{#each result.reasons as reason (reason)}<li>{reason}</li>{/each}</ul>
  {/if}
  {#if result?.video_url}<a class="text-xs underline" href={result.video_url} target="_blank" rel="noreferrer">Watch the exported video</a>{/if}
  {#if error}<p role="alert" class="text-xs text-destructive">{error}</p>{/if}

  {#if shown && (status === 'proposed' || status === 'accepted')}
    <div class="flex flex-wrap gap-2">
      {#if status === 'proposed'}<Button size="sm" disabled={deciding} onclick={() => decide('accept')}>Accept</Button>{/if}
      <Button size="sm" variant="outline" disabled={deciding} onclick={() => decide('discard')}>{status === 'accepted' ? 'Undo (back to the edit before)' : 'Discard'}</Button>
      <Button size="sm" variant="ghost" href={`/?view=edit&revision=${encodeURIComponent(shown)}`} data-sveltekit-reload onclick={(e: MouseEvent) => { if (onopen) { e.preventDefault(); onopen(shown); } }}>Open in timeline</Button>
    </div>
    {#if !refined}
      <details class="text-xs text-muted-foreground">
        <summary class="cursor-pointer select-none hover:text-foreground">Refine once by ear</summary>
        <p class="mt-1">After you export this proposal, earshot can listen to the film second by second and the AI editor gets one pass to correct the sound. It never loops.</p>
        <div class="mt-2 flex flex-wrap gap-2">
          <Button size="sm" variant="outline" disabled={listening || running} onclick={listen}>{listening ? 'Listening…' : heard ? 'Listened ✓' : 'Listen to the export'}</Button>
          <Button size="sm" variant="outline" disabled={!heard || running} onclick={() => start(true)}>Refine once</Button>
        </div>
      </details>
    {/if}
  {/if}
</div>
