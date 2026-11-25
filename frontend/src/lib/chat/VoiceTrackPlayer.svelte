<script lang="ts">
  // Voice first: one scene's dialogue tracks, one per clip, to listen to before any clip
  // is paid for. Each line shows when it is spoken in its clip.
  import MicOff from '@lucide/svelte/icons/mic-off';
  import Clock from '@lucide/svelte/icons/clock';
  import AudioLines from '@lucide/svelte/icons/audio-lines';
  import { mediaUrl } from '$lib/api';
  import type { VoiceTrack } from './types';
  let { tracks }: { tracks: VoiceTrack[] } = $props();
  const secs = (v: number) => `${v.toFixed(1)}s`;
</script>

<div class="grid gap-2">
  {#each tracks as t (t.unit_index)}
    <div class="grid gap-1 rounded-md border border-border p-2 text-xs">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <span class="font-medium">Clip {t.unit_index + 1}</span>
        <span class="inline-flex items-center gap-1 text-muted-foreground">
          {#if !t.lines.length}<MicOff class="size-3" aria-hidden="true" />Nobody speaks{:else if t.ready}<AudioLines class="size-3" aria-hidden="true" />{secs(t.seconds)} of speech · clip {t.clip_seconds}s{:else}<Clock class="size-3" aria-hidden="true" />Not made yet{/if}
        </span>
      </div>
      {#if t.ready && t.url}
        <audio class="w-full" controls preload="none" src={mediaUrl(t.url)}></audio>
      {/if}
      {#if t.too_long}
        <p role="alert" class="text-destructive">The speech is longer than this clip in the edit; shorten a line, then make the tracks again.</p>
      {/if}
      <ul class="grid gap-0.5">
        {#each t.lines as l (l.line_id)}
          <li><span class="text-muted-foreground">{t.ready ? secs(l.start) : '–'}</span> <b>{l.speaker}:</b> {l.text}</li>
        {/each}
      </ul>
    </div>
  {/each}
</div>
