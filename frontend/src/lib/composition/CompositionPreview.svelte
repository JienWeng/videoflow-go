<script lang="ts">
  import { onDestroy, untrack } from 'svelte';
  import { mediaUrl } from '$lib/api';
  import { clipStyle } from './edits.mjs';
  let {
    draft,
    assets,
    time = $bindable(0),
    playing = $bindable(false),
    controls = true,
    fill = false
  }: { draft: any; assets: any[]; time?: number; playing?: boolean; controls?: boolean; fill?: boolean } = $props();
  let boxW = $state(0),
    boxH = $state(0);
  // In fill mode the frame takes the largest size that fits the panel at the composition's aspect ratio.
  const frameWidth = $derived(fill && boxW && boxH ? Math.min(boxW, (boxH * draft.width) / draft.height) : 0);
  let previewWidth = $state(0);
  let root: HTMLDivElement | undefined = $state(),
    error = $state('');
  let frame = 0,
    last = 0;
  const source = (id: string) => assets.find(a => a.id === id);
  const active = (c: any) => time >= c.start && time < c.start + c.duration;
  function tick(now: number) {
    if (!playing) return;
    if (last) time = Math.min(draft.duration, time + (now - last) / 1000);
    last = now;
    if (time >= draft.duration) {
      playing = false;
      last = 0;
      return;
    }
    frame = requestAnimationFrame(tick);
  }
  function toggle() {
    playing = !playing;
  }
  // Start or stop the clock whenever playing changes, including from outside controls.
  $effect(() => {
    const on = playing;
    untrack(() => {
      cancelAnimationFrame(frame);
      last = 0;
      if (on) {
        if (time >= draft.duration) time = 0;
        frame = requestAnimationFrame(tick);
      }
    });
  });
  onDestroy(() => cancelAnimationFrame(frame));
  function syncMedia() {
    const t = time,
      p = playing;
    const tracks = [...draft.clips, ...draft.audio];
    if (!root) return;
    for (const el of root.querySelectorAll<HTMLMediaElement>('video,audio')) {
      const clip = tracks.find(c => c.id === el.dataset.clip);
      if (!clip) continue;
      const on = t >= clip.start && t < clip.start + clip.duration;
      const target = Math.max(0, clip.trim + t - clip.start);
      if (on && el.readyState > 0 && Number.isFinite(target) && Math.abs(el.currentTime - target) > 0.15)
        el.currentTime = target;
      el.muted = clip.muted || !clip.has_audio;
      const local = t - clip.start;
      let gain = Number(clip.volume ?? 1);
      if (clip.fade_in > 0) gain *= Math.max(0, Math.min(1, local / clip.fade_in));
      if (clip.fade_out > 0) gain *= Math.max(0, Math.min(1, (clip.duration - local) / clip.fade_out));
      el.volume = Math.max(0, Math.min(1, Number.isFinite(gain) ? gain : 1));
      if (p && on) void el.play().catch(() => {});
      else el.pause();
    }
  }
  $effect(syncMedia);
</script>
<section aria-label="Composition preview" class={fill?"flex h-full min-h-0 flex-col":"space-y-3"}>
 <div class={fill?"flex min-h-0 flex-1 items-center justify-center":"contents"} bind:clientWidth={boxW} bind:clientHeight={boxH}>
 <div
   bind:this={root}
   bind:clientWidth={previewWidth}
   class="relative mx-auto max-w-full overflow-hidden rounded-lg bg-black {fill?'':'max-h-[420px]'}"
   style={fill
     ? `aspect-ratio:${draft.width}/${draft.height};width:${frameWidth}px`
     : `aspect-ratio:${draft.width}/${draft.height};width:min(100%,${(420 * draft.width) / draft.height}px)`}>
 {#each draft.clips as clip(clip.id)}{@const asset=source(clip.asset_id)}
 {#if asset?.media_kind==='image'&&active(clip)}<img
   src={mediaUrl(asset.url)??''}
   alt={asset.name}
   class="absolute inset-0 size-full"
   style={clipStyle(clip)} />
 {:else if asset?.media_kind==='video'}<video
   data-clip={clip.id}
   src={mediaUrl(asset.url)??''}
   playsinline
   preload="metadata"
   onloadedmetadata={syncMedia}
   class="absolute inset-0 size-full"
   style={clipStyle(clip)+`;visibility:${active(clip)?'visible':'hidden'}`}
   onerror={()=>error='A source cannot be previewed in this browser. Try another supported file.'}><track kind="captions" /></video>{/if}
 {/each}
 {#each draft.audio as clip(clip.id)}{@const asset=source(clip.asset_id)}<audio
   data-clip={clip.id}
   src={mediaUrl(asset?.url)??''}
   preload="metadata"
   onloadedmetadata={syncMedia}><track kind="captions" /></audio>{/each}
 {#each (draft.texts??[]).filter((t:any)=>time>=t.start&&time<t.end) as layer(layer.id)}<p
   class="absolute left-[5%] w-[90%] -translate-y-1/2 whitespace-pre-wrap text-center"
   style={`top:${layer.position==='top'?15:layer.position==='bottom'?80:50}%;color:${layer.color};background:${layer.background};font-family:${layer.font};font-size:${layer.size*previewWidth/draft.width}px`}>{layer.text}</p>{/each}
 {#each draft.captions.filter((c:any)=>time>=c.start&&time<c.end) as cue(cue.id)}<p
   class="absolute inset-x-[5%] bottom-[8%] text-center text-white"
   style={`font-size:${Math.round(Math.min(draft.width,draft.height)*(draft.caption_style==='large'?0.07:draft.caption_style==='compact'?0.036:0.05))*previewWidth/draft.width}px;font-weight:700;line-height:1.25;text-shadow:-1px -1px 0 #000,1px -1px 0 #000,-1px 1px 0 #000,1px 1px 0 #000,0 2px 6px rgba(0,0,0,.8)`}>{cue.text}</p>{/each}
 {#if !draft.clips.length}<p
   class="absolute inset-0 flex items-center justify-center p-6 text-center text-sm text-white">Add an image or video to preview your edit.</p>{/if}
 </div>
 </div>
 {#if controls}<div class="flex items-center gap-3"><button
   type="button"
   class="min-h-11 rounded-lg border px-4 text-sm"
   onclick={toggle}
   aria-label={playing?'Pause composition preview':'Play composition preview'}>{playing?'Pause':'Play'}</button><label
     class="flex min-w-0 flex-1 items-center gap-3 text-sm"><span class="sr-only">Preview time</span><input
   class="w-full"
   aria-label="Preview time"
   type="range"
   min="0"
   max={draft.duration}
   step={1/draft.fps}
   bind:value={time}/><span class="shrink-0 tabular-nums">{Number(time).toFixed(2)} / {Number(draft.duration).toFixed(2)}s</span></label></div>
{/if}
 {#if error}<p role="status" class="text-sm text-muted-foreground">{error}</p>{/if}
 {#if controls}<p class="text-xs text-muted-foreground">Live editing preview. Save and preview rendered frames to check the final export.</p>{/if}
</section>
