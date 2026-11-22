<script lang="ts">
  import { mediaUrl } from '$lib/api';
  import type { CompositionManifest, CompositionSource } from '$lib/composition/types';
  type Track = 'clips' | 'audio' | 'captions';
  /** A block on any track: a clip, or a caption with its length worked out. */
  type TimelineItem = { id: string; start: number; duration: number; asset_id?: string; text?: string };
  let {
    draft,
    assets,
    selected = '',
    time = $bindable(0),
    zoom = $bindable(60),
    onselect,
    onmove,
    ontrim,
    ondrop
  }: {
    draft: CompositionManifest;
    assets: CompositionSource[];
    selected?: string;
    time?: number;
    zoom?: number;
    onselect: (id: string, track: Track) => void;
    onmove: (track: Track, id: string, start: number) => void;
    ontrim: (track: Track, id: string, edge: 'start' | 'end', value: number) => void;
    ondrop: (assetId: string, track: Track, start: number) => void;
  } = $props();

  const TRACKS: { key: Track; label: string; hint: string }[] = [
    { key: 'clips', label: 'Video', hint: 'Drop images or video here' },
    { key: 'audio', label: 'Audio', hint: 'Drop music or sound here' },
    { key: 'captions', label: 'Captions', hint: 'Add captions from the Text tab' }
  ];
  let scroller: HTMLDivElement | undefined = $state();
  let dragging = $state<{ id: string; dx: number; edge?: 'start' | 'end' } | null>(null);
  let dropTrack = $state<Track | ''>('');

  const length = $derived(Math.max(draft.duration, 10) + 5);
  const width = $derived(length * zoom);
  const step = $derived(zoom >= 80 ? 1 : zoom >= 40 ? 2 : zoom >= 20 ? 5 : 10);
  const ticks = $derived(Array.from({ length: Math.floor(length / step) + 1 }, (_, i) => i * step));
  const asset = (id: string | undefined) => assets.find(a => a.id === id);
  const items = (track: Track): TimelineItem[] =>
    track === 'captions' ? draft.captions.map(c => ({ ...c, duration: c.end - c.start })) : draft[track];
  const label = (track: Track, item: TimelineItem) =>
    track === 'captions' ? item.text || 'Empty caption' : (asset(item.asset_id)?.name ?? 'Missing media');
  const fmt = (t: number) => `${Math.floor(t / 60)}:${(t % 60).toFixed(1).padStart(4, '0')}`;

  function timeAt(clientX: number) {
    const rect = scroller!.getBoundingClientRect();
    return Math.max(0, (clientX - rect.left + scroller!.scrollLeft) / zoom);
  }
  function scrub(e: PointerEvent) {
    if (e.button !== 0) return;
    const el = e.currentTarget as HTMLElement;
    try {
      el.setPointerCapture(e.pointerId);
    } catch {
      /* capture unsupported */
    }
    time = Math.min(draft.duration, timeAt(e.clientX));
    const move = (ev: PointerEvent) => {
      time = Math.min(draft.duration, timeAt(ev.clientX));
    };
    const up = () => {
      el.removeEventListener('pointermove', move);
      el.removeEventListener('pointerup', up);
    };
    el.addEventListener('pointermove', move);
    el.addEventListener('pointerup', up);
  }
  // One gesture handler for move and both trim edges; a click without movement just selects.
  function grab(e: PointerEvent, track: Track, item: TimelineItem, edge?: 'start' | 'end') {
    if (e.button !== 0) return;
    e.preventDefault();
    e.stopPropagation();
    const el = e.currentTarget as HTMLElement;
    try {
      el.setPointerCapture(e.pointerId);
    } catch {
      /* capture unsupported */
    }
    const x0 = e.clientX;
    onselect(item.id, track);
    dragging = { id: item.id, dx: 0, edge };
    const move = (ev: PointerEvent) => {
      if (dragging) dragging = { ...dragging, dx: ev.clientX - x0 };
    };
    const up = (ev: PointerEvent) => {
      el.removeEventListener('pointermove', move);
      el.removeEventListener('pointerup', up);
      el.removeEventListener('pointercancel', cancel);
      const delta = (ev.clientX - x0) / zoom;
      dragging = null;
      if (Math.abs(ev.clientX - x0) < 3) return;
      if (edge === 'start') ontrim(track, item.id, 'start', item.start + delta);
      else if (edge === 'end') ontrim(track, item.id, 'end', item.start + item.duration + delta);
      else onmove(track, item.id, Math.max(0, item.start + delta));
    };
    const cancel = () => {
      dragging = null;
      el.removeEventListener('pointermove', move);
      el.removeEventListener('pointerup', up);
      el.removeEventListener('pointercancel', cancel);
    };
    el.addEventListener('pointermove', move);
    el.addEventListener('pointerup', up);
    el.addEventListener('pointercancel', cancel);
  }
  // Live geometry while dragging so the block follows the pointer.
  function geometry(item: TimelineItem) {
    let left = item.start * zoom,
      w = Math.max(6, item.duration * zoom);
    const d = dragging;
    if (d && d.id === item.id) {
      if (d.edge === 'start') {
        left += d.dx;
        w = Math.max(6, w - d.dx);
      } else if (d.edge === 'end') w = Math.max(6, w + d.dx);
      else left = Math.max(0, left + d.dx);
    }
    return `left:${left}px;width:${w}px`;
  }
  function key(e: KeyboardEvent, track: Track, item: TimelineItem) {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
    e.preventDefault();
    const nudge = (e.key === 'ArrowLeft' ? -1 : 1) * (e.shiftKey ? 1 : 1 / draft.fps);
    onmove(track, item.id, Math.max(0, item.start + nudge));
  }
  function accepts(track: Track, kind: string) {
    return track === 'clips'
      ? kind === 'image' || kind === 'video'
      : track === 'audio'
        ? kind === 'audio' || kind === 'video'
        : false;
  }
  function over(e: DragEvent, track: Track) {
    const kind =
      [...(e.dataTransfer?.types ?? [])]
        .find(t => t.startsWith('application/x-videoflow-kind-'))
        ?.slice('application/x-videoflow-kind-'.length) ?? '';
    if (!accepts(track, kind)) return;
    e.preventDefault();
    dropTrack = track;
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy';
  }
  function drop(e: DragEvent, track: Track) {
    dropTrack = '';
    const id = e.dataTransfer?.getData('application/x-videoflow-asset');
    if (!id) return;
    e.preventDefault();
    ondrop(id, track, Math.round(timeAt(e.clientX) * draft.fps) / draft.fps);
  }
  // Keep the playhead visible during playback.
  $effect(() => {
    if (!scroller) return;
    const x = time * zoom,
      view = scroller.scrollLeft,
      w = scroller.clientWidth;
    if (x < view || x > view + w - 40) scroller.scrollLeft = Math.max(0, x - w / 3);
  });
</script>

<div class="flex h-full min-h-0 select-none flex-col text-xs" aria-label="Timeline" role="region">
  <div class="flex min-h-0 flex-1">
    <div class="w-24 shrink-0 border-r border-border bg-card" aria-hidden="true">
      <div class="h-7 border-b border-border"></div>
      {#each TRACKS as t}<div class="flex h-14 items-center border-b border-border px-3 font-medium text-muted-foreground">{t.label}</div>{/each}
    </div>
    <div bind:this={scroller} class="relative min-w-0 flex-1 overflow-x-auto overflow-y-hidden">
      <div class="relative" style={`width:${width}px`}>
        <div
          class="sticky top-0 z-10 h-7 cursor-col-resize border-b border-border bg-card"
          role="slider"
          tabindex="0"
          aria-label="Playhead position"
          aria-valuemin="0"
          aria-valuemax={draft.duration}
          aria-valuenow={time}
          aria-valuetext={fmt(time)}
          onpointerdown={scrub}
          onkeydown={e => {
            if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
              e.preventDefault();
              time = Math.max(
                0,
                Math.min(draft.duration, time + (e.key === 'ArrowLeft' ? -1 : 1) * (e.shiftKey ? 1 : 1 / draft.fps))
              );
            }
          }}>
          {#each ticks as t}<span
            class="absolute top-0 h-full border-l border-border/70 pl-1 pt-1 text-[10px] tabular-nums text-muted-foreground"
            style={`left:${t * zoom}px`}>{fmt(t).replace(/\.0$/, '')}</span>{/each}
          <span
            class="absolute inset-y-0 right-auto bg-muted/40"
            style={`left:${draft.duration * zoom}px;width:${(length - draft.duration) * zoom}px`}
            title="After the end of your video"></span>
        </div>
        {#each TRACKS as t}
          <div class="relative h-14 border-b border-border {dropTrack === t.key ? 'bg-primary/10' : ''}" role="list" aria-label={`${t.label} track`}
            ondragover={e => over(e, t.key)} ondragleave={() => dropTrack = ''} ondrop={e => drop(e, t.key)}
            onpointerdown={scrub}>
            {#if !items(t.key).length}<span class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground/70">{t.hint}</span>{/if}
            {#each items(t.key) as item (item.id)}
              {@const src = t.key === 'clips' ? asset(item.asset_id) : null}
              <div
                role="listitem"
                class="absolute top-1.5 bottom-1.5 overflow-hidden rounded-md border text-left shadow-sm {selected === item.id ? 'border-primary ring-2 ring-primary/60' : 'border-border'} {t.key === 'clips' ? 'bg-sky-500/25' : t.key === 'audio' ? 'bg-emerald-500/25' : 'bg-amber-500/25'}"
                style={geometry(item)}>
                <button
                  type="button"
                  class="absolute inset-0 flex cursor-grab items-center gap-2 overflow-hidden px-3 text-left active:cursor-grabbing focus-visible:outline-2 focus-visible:outline-ring"
                  aria-label={`${t.label}: ${label(t.key, item)}, ${fmt(item.start)} to ${fmt(item.start + item.duration)}`}
                  aria-pressed={selected === item.id}
                  onpointerdown={e => grab(e, t.key, item)}
                  onkeydown={e => key(e, t.key, item)}
                  onclick={() => onselect(item.id, t.key)}>
                  {#if src?.media_kind === 'image'}<img
                    src={mediaUrl(src.url) ?? ''}
                    alt=""
                    class="h-8 w-8 shrink-0 rounded object-cover"
                    draggable="false" />{/if}
                  <span class="truncate">{label(t.key, item)}</span>
                </button>
                <span
                  class="absolute inset-y-0 left-0 w-2 cursor-ew-resize bg-foreground/0 hover:bg-foreground/30"
                  title="Drag to trim the start"
                  aria-hidden="true"
                  onpointerdown={e => grab(e, t.key, item, 'start')}></span>
                <span
                  class="absolute inset-y-0 right-0 w-2 cursor-ew-resize bg-foreground/0 hover:bg-foreground/30"
                  title="Drag to trim the end"
                  aria-hidden="true"
                  onpointerdown={e => grab(e, t.key, item, 'end')}></span>
              </div>
            {/each}
          </div>
        {/each}
        <div
          class="pointer-events-none absolute inset-y-0 z-20 w-px bg-red-500"
          style={`left:${time * zoom}px`}
          aria-hidden="true"><span class="absolute -left-1.5 top-0 size-3 rotate-45 bg-red-500"></span></div>
      </div>
    </div>
  </div>
</div>
