<script lang="ts">
  // Make the videos of several scenes with one click: each scene, the seconds still to
  // make and the price, read from the same estimate the run will spend against.
  import { onMount } from 'svelte';
  import { get } from '$lib/api';
  import Clapperboard from '@lucide/svelte/icons/clapperboard';
  import { Button } from '$lib/components/ui/button';
  import type { CostEstimate } from '$lib/api-types';

  type Line = { scene_id: string; title: string; seconds: number; amount: number; note?: string };
  let { scenes, onstart }: { scenes: { id: string; title: string; duration?: number }[]; onstart: () => void } = $props();
  let lines = $state<Line[]>([]), total = $state<number | null>(null), seconds = $state(0), known = $state(true), failed = $state(false);
  const usd = (v: number) => '$' + v.toFixed(2);
  onMount(async () => {
    try {
      const e = await get<CostEstimate>('/costs/estimate?scene_ids=' + encodeURIComponent(scenes.map((s) => s.id).join(',')));
      lines = e.scenes ?? []; total = e.total; seconds = e.seconds; known = e.known;
    } catch { failed = true; }
  });
  const rows = $derived(lines.length ? lines : scenes.map((s) => ({ scene_id: s.id, title: s.title, seconds: s.duration ?? 0, amount: 0 }) as Line));
</script>

<div class="mt-2 grid gap-2 rounded-md border border-border p-3 text-sm">
  <ul class="grid gap-1">
    {#each rows as l (l.scene_id)}
      <li class="flex flex-wrap justify-between gap-2"><span
        class="font-medium">{l.title || 'Scene'}</span><span
        class="text-xs text-muted-foreground">{l.note ? l.note : `${l.seconds} s${l.amount ? ' · ' + usd(l.amount) : ''}`}</span></li>
    {/each}
  </ul>
  <p class="text-xs font-medium">
    {#if total === null}{failed ? 'The price could not be read just now. The button still uses video credits.' : 'Reading the price…'}
    {:else if seconds === 0}Every scene here already has its video, so this costs nothing more.
    {:else}{known ? 'About ' + usd(total) : 'At least ' + usd(total)} for {seconds} seconds of new video.{/if}
  </p>
  <div class="flex flex-wrap items-center gap-2">
    <Button size="sm" class="rounded-full px-4" onclick={onstart}><Clapperboard class="size-4" />Make these videos</Button>
    <span class="text-xs text-muted-foreground">The scenes are made side by side. Uses video credits.</span>
  </div>
</div>
