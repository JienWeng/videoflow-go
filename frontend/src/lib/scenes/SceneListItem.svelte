<script lang="ts">
  import { mediaUrl } from '$lib/api';
  import { Film } from '@lucide/svelte';
  import type { Asset, Scene } from '$lib/api-types';

  // A compact, selectable scene row for the master list. All derivation is done
  // by the parent and passed in so the list and detail share one source of truth.
  let {
    scene,
    selected = false,
    statusLine,
    accentDot,
    dirty = false,
    storyboard = null,
    onselect
  }: {
    scene: Scene;
    selected?: boolean;
    statusLine: string;
    accentDot: string;
    dirty?: boolean;
    storyboard?: Asset | null;
    onselect: () => void;
  } = $props();

</script>

<button
  type="button"
  aria-pressed={selected}
  onclick={onselect}
  class="group flex w-full items-start gap-2.5 rounded-lg border px-2.5 py-2 text-left transition-colors
         {selected
    ? 'border-primary/60 bg-accent'
    : 'border-transparent hover:border-border hover:bg-accent/50'}"
>
  <!-- Thumbnail (storyboard) or status dot -->
  {#if storyboard}
    <img
      class="size-10 shrink-0 rounded-md border border-border object-cover"
      src={mediaUrl(storyboard.file_path)}
      alt=""
      loading="lazy"
    />
  {:else}
    <div class="flex size-10 shrink-0 items-center justify-center rounded-md border border-dashed border-border bg-muted/30 text-muted-foreground/50">
      <Film class="size-4" />
    </div>
  {/if}

  <div class="min-w-0 flex-1">
    <div class="flex items-center gap-1.5">
      <span class="size-2 shrink-0 rounded-full {accentDot}"></span>
      <span class="truncate text-sm font-medium" title={scene.title}>
        {scene.title || 'Untitled scene'}
      </span>
      {#if dirty}
        <span class="ml-auto shrink-0 rounded-sm bg-amber-500/15 px-1 text-[10px] font-medium text-amber-800 dark:text-amber-300"
          title="Unsaved edits">unsaved</span>
      {/if}
    </div>
    <p class="mt-0.5 truncate text-xs text-muted-foreground">{statusLine}</p>


  </div>
</button>
