<script lang="ts">
  // A scene video the video service took but never answered about. Check again asks the
  // service (free); Stop waiting closes it on the owner's word so the scene can be made again.
  import { onMount } from 'svelte';
  import { post } from '$lib/api';
  import { Button } from '$lib/components/ui/button';

  type Check = { job_id: string; scene_id?: string; service: string; outcome: 'found' | 'failed' | 'unknown'; message: string };
  let {
    jobId,
    service = 'The video service',
    reason = '',
    autoCheck = false,
    askStop = false,
    onsettled
  }: {
    jobId: string;
    service?: string;
    reason?: string;
    /** Ask the service right away (it is free). */
    autoCheck?: boolean;
    /** Open the Stop waiting question straight away. */
    askStop?: boolean;
    /** Called once the job is no longer waiting: found (being saved) or failed (can be made again). */
    onsettled?: (check: Check) => void;
  } = $props();

  let busy = $state(''), result = $state<Check | null>(null), error = $state('');
  // svelte-ignore state_referenced_locally
  let confirming = $state(askStop);
  const name = $derived(result?.service ?? service);

  async function run(kind: 'check' | 'stop-waiting') {
    busy = kind; error = '';
    try {
      result = await post(`/render-jobs/${encodeURIComponent(jobId)}/${kind}`, {});
      confirming = false;
      if (result && result.outcome !== 'unknown') onsettled?.(result);
    } catch (e) { error = (e as Error).message; }
    busy = '';
  }
  onMount(() => { if (autoCheck) void run('check'); });
</script>

<div class="grid gap-2 text-sm" role="group" aria-label="Video waiting on an answer">
  {#if result}<p role="status" class={result.outcome === 'unknown' ? 'text-destructive' : ''}>{result.message}</p>
  {:else if reason}<p class="text-muted-foreground">{reason}</p>{/if}
  {#if !result || result.outcome === 'unknown'}
    {#if confirming}
      <div class="grid gap-2 rounded-md border border-border p-3">
        <p>{name} may still have the earlier request and might bill it, even though VideoFlow stops waiting for it. Making the scene again is a new charge.</p>
        <div class="flex flex-wrap gap-2">
          <Button size="sm" variant="destructive" disabled={!!busy} onclick={() => run('stop-waiting')}>{busy === 'stop-waiting' ? 'Stopping…' : 'Stop waiting'}</Button>
          <Button size="sm" variant="outline" disabled={!!busy} onclick={() => (confirming = false)}>Keep waiting</Button>
        </div>
      </div>
    {:else}
      <div class="flex flex-wrap items-center gap-2">
        <Button size="sm" variant="outline" disabled={!!busy} onclick={() => run('check')}>{busy === 'check' ? 'Asking…' : 'Check again'}</Button>
        <Button size="sm" variant="ghost" disabled={!!busy} onclick={() => (confirming = true)}>Stop waiting</Button>
        <span class="text-xs text-muted-foreground">Check again is free.</span>
      </div>
    {/if}
  {/if}
  {#if error}<p role="alert" class="text-xs text-destructive">{error}</p>{/if}
</div>
