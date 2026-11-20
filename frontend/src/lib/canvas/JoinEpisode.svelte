<script lang="ts">
  // Join an episode's finished scene videos into one final video, from the canvas.
  // Free: it never makes a video, and says which scene is missing one instead.
  import { onDestroy } from 'svelte';
  import { get, post } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import { Combine } from '@lucide/svelte';

  let { scriptId, episode = '', again = false, onjoined }: { scriptId: string; episode?: string; again?: boolean; onjoined?: () => void } = $props();
  let busy = $state(false), stage = $state(''), problem = $state(''), done = $state(false), alive = true;
  onDestroy(() => (alive = false));

  async function join() {
    busy = true; problem = ''; done = false; stage = 'Starting…';
    try {
      const started = await post('/autopilot/finish?background=true', { script_id: scriptId, join_only: true });
      for (let i = 0; alive && i < 600; i++) {
        const op = await get('/ops/' + encodeURIComponent(started.op_id));
        if (op.status === 'succeeded') { done = true; onjoined?.(); return; }
        if (op.status !== 'running') { problem = op.error ?? 'The episode could not be joined.'; return; }
        stage = (op.result_json?.stage ?? 'Working') + '…';
        await new Promise((r) => setTimeout(r, 3000));
      }
    } catch (e) { problem = (e as Error).message; }
    finally { busy = false; }
  }
</script>

<span class="inline-flex flex-wrap items-center gap-2">
  <Button variant="ghost" size="sm" disabled={busy} onclick={join} title="Free. Joins the finished scene videos in story order, adds captions and exports one video.">
    <Combine class="size-3.5" />{busy ? stage : again ? 'Join again' : episode ? `Join ${episode}` : 'Join episode'}
  </Button>
  {#if done}<span role="status" class="text-xs text-muted-foreground">Joined. The final video is on the canvas.</span>{/if}
  {#if problem}<span role="alert" class="max-w-xs text-xs text-destructive">{problem}</span>{/if}
</span>
