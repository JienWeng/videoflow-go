<script lang="ts">
  import { get } from '$lib/api';
  import { goto } from '$app/navigation';
  import * as Sheet from '$lib/components/ui/sheet';
  import { Button } from '$lib/components/ui/button';
  import { storyCreation } from '$lib/create/story-creation.svelte';
  import { palette } from './palette.svelte';
  import { guide } from './guide.svelte';
  import { Check, Circle, ArrowRight, Compass, MessageCircleQuestion } from '@lucide/svelte';

  let status: any = $state(null), progress: any = $state(null), loading = $state(false);

  async function load() {
    loading = true;
    const [s, p] = await Promise.all([get('/director/status').catch(() => null), get('/production/progress').catch(() => null)]);
    status = s; progress = p; loading = false;
  }
  $effect(() => { if (guide.panelOpen) void load(); });

  const connected = $derived(!!status?.engines?.some((e: any) => e.signed_in));
  const scenes = $derived(progress?.total ?? 0);
  const boarded = $derived((progress?.scenes ?? []).filter((s: any) => ['ready_to_render', 'rendering', 'video_ready'].includes(s.stage)).length);
  const videos = $derived(progress?.video_ready ?? 0);

  type Item = { title: string; detail: string; done: boolean; action: string; run: () => void };
  const items: Item[] = $derived([
    { title: 'Connect your AI', detail: connected ? 'Codex or Claude Code is connected.' : 'Sign in to Codex or Claude Code so the assistant can plan your story.', done: connected, action: 'Connect', run: () => go('/director') },
    { title: 'Create your story', detail: scenes ? `${scenes} scene${scenes === 1 ? '' : 's'} planned.` : 'Describe an idea; VideoFlow plans scenes and characters.', done: scenes > 0, action: 'Create story', run: () => { guide.panelOpen = false; void goto('/').then(() => storyCreation.show()); } },
    { title: 'Make storyboards', detail: scenes ? `${boarded} of ${scenes} scenes have an up-to-date storyboard.` : 'Preview each scene as an image before paying for video.', done: scenes > 0 && boarded === scenes, action: 'Open scenes', run: () => go('/?view=story') },
    { title: 'Make videos', detail: scenes ? `${videos} of ${scenes} scenes have video.` : 'Turn each storyboard into a video clip.', done: scenes > 0 && videos === scenes, action: 'Open scenes', run: () => go('/?view=story') },
    { title: 'Edit and export', detail: 'Put clips in order, add music and captions, then export your finished video.', done: false, action: 'Edit video', run: () => go('/?view=edit') }
  ]);
  const current = $derived(items.findIndex(i => !i.done));

  function go(href: string) { guide.panelOpen = false; void goto(href); }
  function ask() {
    guide.panelOpen = false;
    void goto('/').then(() => window.dispatchEvent(new CustomEvent('videoflow-ask-assistant', { detail: { text: 'I’m stuck. Where is my project up to, and what should I do next?' } })));
  }
</script>

<Sheet.Root bind:open={guide.panelOpen}>
  <Sheet.Content side="right" class="w-full overflow-y-auto sm:max-w-md">
    <Sheet.Header>
      <Sheet.Title>Guide</Sheet.Title>
      <Sheet.Description>Your path from idea to finished video.</Sheet.Description>
    </Sheet.Header>
    <div class="space-y-6 px-4 pb-6">
      <ol class="space-y-1" aria-label="Getting started checklist" aria-busy={loading}>
        {#each items as item, i}
          <li class="flex gap-3 rounded-lg p-3 {i === current ? 'bg-accent/60' : ''}">
            <span class="mt-0.5 shrink-0" aria-hidden="true">{#if item.done}<Check class="size-5 text-emerald-500" />{:else}<Circle class="size-5 {i === current ? 'text-primary' : 'text-muted-foreground/50'}" />{/if}</span>
            <div class="min-w-0 flex-1">
              <p class="text-sm font-medium">{i + 1}. {item.title}<span class="sr-only">{item.done ? ' — done' : i === current ? ' — next' : ''}</span></p>
              <p class="mt-0.5 text-xs text-muted-foreground">{item.detail}</p>
              {#if i === current}<Button size="sm" class="mt-2" onclick={item.run}>{item.action}<ArrowRight class="size-3.5" /></Button>{/if}
            </div>
          </li>
        {/each}
      </ol>

      <div class="grid gap-2">
        <Button variant="outline" class="justify-start" onclick={() => guide.startTour()}><Compass class="size-4" />Take the 1-minute tour</Button>
        <Button variant="outline" class="justify-start" onclick={ask}><MessageCircleQuestion class="size-4" />Ask the assistant for help</Button>
      </div>

      <section aria-labelledby="tips-title" class="space-y-3 text-sm">
        <h3 id="tips-title" class="font-medium">Good to know</h3>
        <ul class="space-y-2 text-muted-foreground">
          <li><span class="font-medium text-foreground">Nothing paid runs by surprise.</span> Image and video steps show what they will use and wait for you to confirm.</li>
          <li><span class="font-medium text-foreground">Storyboards first.</span> A storyboard is a cheap preview image; fix it before making video.</li>
          <li><span class="font-medium text-foreground">Stuck or something failed?</span> Ask the assistant “why did this stop?” — it checks what is saved and suggests one fix.</li>
          <li><span class="font-medium text-foreground">Find anything fast.</span> Press <kbd class="rounded border border-border px-1 text-xs">Ctrl/⌘ K</kbd> or use <button type="button" class="underline underline-offset-4" onclick={() => { guide.panelOpen = false; palette.open = true; }}>Search</button>.</li>
          <li><span class="font-medium text-foreground">Your work is saved as you go.</span> Closing the tab doesn’t lose scenes, images or videos.</li>
        </ul>
      </section>
    </div>
  </Sheet.Content>
</Sheet.Root>
