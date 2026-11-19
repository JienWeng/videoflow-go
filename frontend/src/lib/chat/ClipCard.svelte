<script lang="ts">
  // One short clip made directly from attached or library media ("make this photo dance").
  import { onDestroy, onMount } from 'svelte';
  import { get, post, mediaUrl, isImage } from '$lib/api';
  import { editOutputHref } from '$lib/editor/links';
  import { Button } from '$lib/components/ui/button';
  import * as Item from '$lib/components/ui/item';
  import { Textarea } from '$lib/components/ui/textarea';
  import LoaderCircle from '@lucide/svelte/icons/loader-circle';
  import X from '@lucide/svelte/icons/x';

  type Ref = { asset_id: string; kind: string; purpose: string; name?: string; file_path?: string };
  type Clip = { prompt: string; duration: number; aspect_ratio: string; generate_audio: boolean; references: Ref[]; job_id?: string };
  let { clip, onjob, ondismiss }: { clip: Clip; onjob?: (id: string) => void; ondismiss?: () => void } = $props();

  // svelte-ignore state_referenced_locally
  let prompt = $state(clip.prompt), duration = $state(clip.duration), aspect = $state(clip.aspect_ratio), audio = $state(clip.generate_audio), references = $state<Ref[]>([...clip.references]);
  // svelte-ignore state_referenced_locally
  let job = $state(clip.job_id ?? ''), status = $state(''), output = $state<{ id: string; video_path: string } | null>(null);
  let busy = $state(false), error = $state('');
  let alive = true, timer: ReturnType<typeof setTimeout> | undefined, polls = 0;
  onDestroy(() => { alive = false; if (timer) clearTimeout(timer); });
  onMount(() => { if (job) void poll(); });

  const visual = () => references.some((r) => r.kind !== 'audio');
  const body = () => ({ prompt: prompt.trim(), duration, aspect_ratio: aspect, resolution: '720p', generate_audio: audio, references: references.map(({ asset_id, kind, purpose }) => ({ asset_id, kind, purpose })) });

  async function poll() {
    if (!alive || !job) return;
    if (++polls > 360) { error = 'Still working. Check Videos for the result.'; return; }
    try {
      const r = await get('/render-jobs/' + job);
      status = r.job?.status ?? '';
      if (status === 'succeeded') { output = r.outputs?.[0] ?? null; return; }
      if (['failed', 'reconciliation_required', 'canceled'].includes(status)) { error = r.job?.error || 'The video service could not make this clip.'; return; }
    } catch { /* keep polling through brief network errors */ }
    timer = setTimeout(() => void poll(), 5000);
  }
  async function make() {
    if (busy || !prompt.trim() || !visual()) return;
    busy = true; error = '';
    try {
      const plan = await post('/reference-videos/plan', body());
      const r = await post('/reference-videos/render', { ...body(), confirmed_plan_id: plan.id });
      job = r.job_id; status = 'pending'; output = null; polls = 0; onjob?.(job);
      void poll();
    } catch (e) { error = (e as Error).message; } finally { busy = false; }
  }
  const working = $derived(!!job && !output && !error);
  const locked = $derived(working || !!output);
</script>

<Item.Root variant="outline" class="mt-2 flex-col items-stretch gap-3">
  <Item.Content>
    <Item.Title>Make a clip</Item.Title>
    <Item.Description>Turns your picture into one short video. Check the direction, then make it.</Item.Description>
  </Item.Content>
  <ul class="flex flex-wrap gap-2" aria-label="Media used">
    {#each references as ref, i (ref.asset_id)}
      <li class="flex items-center gap-2 rounded-md border border-border p-1 pr-2 text-xs">
        {#if isImage(ref.file_path)}<img class="size-10 rounded object-cover" src={mediaUrl(ref.file_path) ?? ''} alt="" />{:else if ref.kind === 'video'}<video class="size-10 rounded object-cover" src={mediaUrl(ref.file_path) ?? ''} muted preload="metadata"></video>{/if}
        <span class="grid max-w-40"><span class="truncate">{ref.name ?? ref.asset_id}</span><span class="text-muted-foreground">{ref.kind === 'video' ? 'Movement to copy' : ref.kind === 'audio' ? 'Sound' : 'Who appears'}</span></span>
        {#if !locked && references.length > 1}<button type="button" class="grid size-7 place-items-center rounded hover:bg-muted" aria-label={`Don't use ${ref.name ?? 'this file'}`} onclick={() => (references = references.filter((_, j) => j !== i))}><X class="size-3.5" /></button>{/if}
      </li>
    {/each}
  </ul>
  <label class="grid gap-1 text-xs font-medium">What happens in the clip
    <Textarea rows={4} bind:value={prompt} disabled={locked} class="text-sm font-normal" />
  </label>
  <div class="grid grid-cols-2 gap-2 text-xs font-medium">
    <label class="grid gap-1">Length
      <select class="h-9 rounded-md border border-input bg-background px-2 text-sm font-normal" bind:value={duration} disabled={locked}>
        {#each Array.from({ length: 12 }, (_, i) => i + 4) as s}<option value={s}>{s} seconds</option>{/each}
      </select>
    </label>
    <label class="grid gap-1">Shape
      <select class="h-9 rounded-md border border-input bg-background px-2 text-sm font-normal" bind:value={aspect} disabled={locked}>
        <option value="9:16">Tall · 9:16</option><option value="16:9">Wide · 16:9</option><option value="1:1">Square · 1:1</option>
      </select>
    </label>
  </div>
  <label class="flex items-center gap-2 text-sm"><input type="checkbox" class="size-4" bind:checked={audio} disabled={locked} />Add sound and music made by the AI</label>

  {#if output}
    <!-- svelte-ignore a11y_media_has_caption -->
    <video class="max-h-80 w-full rounded-md bg-black" src={mediaUrl(output.video_path) ?? ''} controls playsinline></video>
    <div class="flex flex-wrap gap-2">
      <Button size="sm" class="rounded-full px-4" href={editOutputHref(output.id)}>Edit this clip</Button>
      <Button size="sm" variant="outline" class="rounded-full px-4" href={mediaUrl(output.video_path) ?? ''} download>Download</Button>
      <Button size="sm" variant="ghost" onclick={() => { job = ''; output = null; status = ''; }}>Change and make another</Button>
    </div>
  {:else if working}
    <p role="status" class="flex items-center gap-2 text-sm"><LoaderCircle class="size-4 animate-spin motion-reduce:animate-none" />Making your clip… this usually takes a few minutes. You can keep chatting; it also appears in Videos.</p>
  {:else}
    <div class="flex flex-wrap items-center gap-2">
      <Button size="sm" class="rounded-full px-4" disabled={busy || !prompt.trim() || !visual()} onclick={make}>{#if busy}<LoaderCircle class="size-4 animate-spin motion-reduce:animate-none" />{/if}{job ? 'Try again' : 'Make clip'}</Button>
      {#if !job && ondismiss}<Button size="sm" variant="ghost" onclick={ondismiss}>Not now</Button>{/if}
      <span class="text-xs text-muted-foreground">1 video · {duration} seconds · uses video credits</span>
    </div>
  {/if}
  {#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
</Item.Root>
