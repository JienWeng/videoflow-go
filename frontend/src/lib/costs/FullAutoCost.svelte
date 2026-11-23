<script lang="ts">
  // What Full auto may spend on video, shown at its one confirmation: the story's video
  // seconds, and the worst case if the speech check flags clips and Full auto shortens
  // their lines and re-renders each scene once (internal/api/costs.go).
  import { get } from '$lib/api';
  import type { CostEstimate } from '$lib/api-types';
  let { seconds, aspect = '' }: { seconds: number; aspect?: string } = $props();
  let estimate = $state<CostEstimate | null>(null);
  $effect(() => {
    if (!seconds) return;
    const query = `/costs/estimate?full_auto=true&target_seconds=${seconds}&aspect_ratio=${encodeURIComponent(aspect)}`;
    get<CostEstimate>(query)
      .then(e => (estimate = e))
      .catch(() => (estimate = null)); // the text below still states the worst case in seconds
  });
  const usd = (v: number) => '$' + v.toFixed(2);
  const text = $derived.by(() => {
    if (!seconds) return '';
    const retry = `if the speech check flags a clip, Full auto shortens its lines and re-renders that scene once, never more, so the worst case is ${seconds * 2} seconds of video`;
    if (!estimate?.known) return `Full auto makes about ${seconds} seconds of video; ${retry}.`;
    return `Full auto makes about ${seconds} seconds of video (about ${usd(estimate.total)}); ${retry} (about ${usd(estimate.worst_total ?? estimate.total * 2)}).`;
  });
</script>
{#if text}<p class="text-xs font-medium">{text}</p>{/if}
