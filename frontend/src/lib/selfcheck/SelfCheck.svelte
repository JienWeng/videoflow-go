<script lang="ts">
  // Self-check badge: what the local AI session thought of a picture or clip the app
  // made, with the findings behind a disclosure, "Check again" (one session turn, no
  // credits) and "Keep it" when the findings are not worth acting on. With kind
  // clip_audio it is the speech check: what was heard in the clip, line by line.
  import { onMount } from 'svelte';
  import { get, post } from '$lib/api';
  import { runBackgroundOp } from '$lib/ops';
  import { errorMessage } from '$lib/errors';
  import { Button } from '$lib/components/ui/button';
  import { checkKey, checkName, engineName, findingLines, heardLines, needsAttention, readSelfCheck, selfCheckBadge } from './selfcheck.mjs';

  let {
    kind,
    id,
    record = undefined,
    load = false,
    compact = false,
    onchange
  }: {
    /** character_sheet, scene_picture, clip or clip_audio (the speech check) */
    kind: string;
    id: string;
    /** The stored self_check, or the asset metadata / clip notes that hold it. */
    record?: unknown;
    /** Ask the server for the latest check when mounted (clips carry no metadata here). */
    load?: boolean;
    compact?: boolean;
    onchange?: () => void;
  } = $props();

  let fetched = $state<unknown>(undefined);
  let busy = $state(false);
  let problem = $state('');
  const speech = $derived(kind === 'clip_audio');
  const check = $derived(readSelfCheck(fetched ?? record, checkKey(kind)));
  const badge = $derived(selfCheckBadge(check));
  const tones = {
    ok: 'border-emerald-500/40 text-emerald-700 dark:text-emerald-400',
    warn: 'border-amber-500/50 text-amber-700 dark:text-amber-400',
    bad: 'border-destructive/50 text-destructive',
    muted: 'border-border text-muted-foreground'
  };

  onMount(() => {
    if (load && !readSelfCheck(record, checkKey(kind))) void refresh();
  });

  async function refresh() {
    try {
      const r = await get<{ self_check: unknown }>(`/selfcheck?kind=${encodeURIComponent(kind)}&id=${encodeURIComponent(id)}`);
      if (r.self_check) fetched = r.self_check;
    } catch {
      /* no check yet reads as "Not checked" */
    }
  }

  async function checkAgain() {
    busy = true;
    problem = '';
    try {
      await runBackgroundOp<{ self_check?: unknown }>(
        '/selfcheck',
        { kind, id, force: !!check },
        {
          label: checkName(kind),
          onDone: op => {
            busy = false;
            if (op.result_json?.self_check) fetched = op.result_json.self_check;
            onchange?.();
          },
          onFail: op => {
            busy = false;
            problem = op.error ?? 'The check could not run.';
          }
        }
      );
    } catch (e) {
      busy = false;
      problem = errorMessage(e);
    }
  }

  async function keep() {
    busy = true;
    problem = '';
    try {
      const r = await post<{ self_check: unknown }>('/selfcheck/accept', { kind, id });
      fetched = r.self_check;
      onchange?.();
    } catch (e) {
      problem = errorMessage(e);
    } finally {
      busy = false;
    }
  }
</script>

<details class="group text-xs {compact ? '' : 'mt-2'}" open={!compact && needsAttention(check)}>
  <summary class="inline-flex cursor-pointer select-none list-none items-center gap-1.5">
    <span class="rounded-full border px-2 py-0.5 font-medium {tones[badge.tone]}" title={speech ? 'What was heard in the clip, compared with the script' : 'Self-check by your local AI session'}>{checkName(kind)}: {badge.label}</span>
    {#if check && (check.issues.length || check.note)}<span class="text-muted-foreground group-open:hidden">Details</span>{/if}
  </summary>
  <div class="mt-2 grid gap-2 rounded-md border border-border p-2">
    {#if check}
      {#if findingLines(check).length}<ul class="list-disc space-y-0.5 pl-4" aria-label="Findings">{#each findingLines(check) as line, i (i)}<li>{line}</li>{/each}</ul>
      {:else if check.verdict === 'pass' && !check.unavailable}<p>Nothing wrong was found.</p>{/if}
      {#if heardLines(check).length}<ul class="space-y-0.5" aria-label="Lines as heard">{#each heardLines(check) as line, i (i)}<li>{line}</li>{/each}</ul>{/if}
      {#if check.suggestion}<p>{check.suggestion}</p>{/if}
      {#if check.change && needsAttention(check)}<p><span class="font-medium">Suggested change:</span> {check.change}</p>{/if}
      {#if check.note}<p class="text-muted-foreground">{check.note}</p>{/if}
      {#if check.attempt > 1}<p class="text-muted-foreground">This is the one automatic retry; it is not made again by itself.</p>{/if}
    {:else}
      <p class="text-muted-foreground">{speech
        ? 'Not checked yet. A check listens to the clip with the Listening service in AI setup (about half a cent per minute of video, once per clip) and compares what is said with the script.'
        : 'Not checked yet. A check asks your local AI session to look at it; it uses no credits.'}</p>
    {/if}
    <div class="flex flex-wrap items-center gap-2">
      <Button size="sm" variant="outline" class="h-7 rounded-full px-3 text-xs" disabled={busy} onclick={checkAgain}>{busy ? 'Checking…' : check ? 'Check again' : 'Check'}</Button>
      {#if needsAttention(check)}<Button size="sm" variant="ghost" class="h-7 rounded-full px-3 text-xs" disabled={busy} onclick={keep}>Keep it</Button>{/if}
      {#if check?.checkedAt}<span class="text-muted-foreground">{new Date(check.checkedAt).toLocaleString()}{check.engine ? ' · ' + engineName(check.engine) : ''}</span>{/if}
    </div>
    {#if problem}<p class="text-destructive">{problem}</p>{/if}
  </div>
</details>
