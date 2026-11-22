<script lang="ts">
  // The price of finishing a story, shown before the button is pressed.
  import { onMount } from 'svelte';
  import { get } from '$lib/api';
  import type { CostEstimate } from '$lib/api-types';
  let { scriptId }: { scriptId: string } = $props();
  let text = $state('');
  onMount(async () => {
    try {
      const e = await get<CostEstimate>('/costs/estimate?script_id=' + encodeURIComponent(scriptId));
      if (!e.known) text = '';
      else if (e.seconds === 0) text = 'Every scene video is already made, so this costs nothing more.';
      else text = `About $${e.total.toFixed(2)} for ${e.seconds} seconds of new video.`;
    } catch { /* the button still explains that credits are used */ }
  });
</script>
{#if text}<span class="text-xs font-medium">{text}</span>{/if}
