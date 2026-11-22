<script lang="ts">
  import { errorMessage } from '$lib/errors';
  import { onMount } from 'svelte';
  import { get } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import { ArrowRight } from '@lucide/svelte';
  import { makeOverview } from './overview.mjs';
  import type { ApiGraph } from '$lib/canvas/transform';
  import type { SavedComposition } from '$lib/composition/types';
  import type { Op } from '$lib/ops';
  import type { Asset, DirectorSession, DirectorStatus, Project, RenderJob } from '$lib/api-types';

  let { onopenStudio, active = true }: { onopenStudio: () => void; active?: boolean } = $props();
  let project = $state<Project | null>(null);
  let model = $state<ReturnType<typeof makeOverview> | null>(null);
  let busy = $state(false),
    error = $state('');
  const secondaryAttention = $derived(model?.attention.filter(item => item.id !== model?.primary.id) ?? []);
  const metrics = $derived(
    model
      ? [
          { label: 'Scenes', value: model.metrics.scenes, href: '/?view=story' },
          { label: 'Characters', value: model.metrics.characters, href: '/library?tab=characters' },
          { label: 'Assets', value: model.metrics.assets, href: '/library?tab=media' },
          { label: 'Finished videos', value: model.metrics.videos, href: '/render' }
        ]
      : []
  );
  const formatTime = (date: string) =>
    new Date(date).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' });

  export async function refresh() {
    if (busy) return;
    busy = true;
    error = '';
    try {
      const [active, graph, assets, ops, jobs, sessions, aiHealth, appHealth, compositions] = await Promise.all([
        get<Project>('/projects/active'),
        get<ApiGraph>('/graph'),
        get<Asset[]>('/assets'),
        get<Op[]>('/ops?limit=all'),
        get<RenderJob[]>('/render-jobs'),
        get<DirectorSession[]>('/director/sessions').catch(() => null),
        get<DirectorStatus>('/director/status').catch(() => null),
        get('/health').catch(() => null),
        get<SavedComposition[]>('/compositions').catch(() => null)
      ]);
      const next = makeOverview({ graph, assets, ops, jobs, sessions, aiHealth, appHealth, compositions });
      project = active;
      model = next;
    } catch (e) {
      error = errorMessage(e, String(e));
    } finally {
      busy = false;
    }
  }
  onMount(() => {
    void refresh();
    const resume = () => {
      if (active && document.visibilityState === 'visible') void refresh();
    };
    window.addEventListener('focus', resume);
    document.addEventListener('visibilitychange', resume);
    return () => {
      window.removeEventListener('focus', resume);
      document.removeEventListener('visibilitychange', resume);
    };
  });
</script>

<div class="mx-auto w-full max-w-6xl px-4 py-6 sm:px-8 sm:py-8" aria-busy={busy}>
  {#if error}
    <div role="alert" class="mb-6 rounded-lg border border-destructive p-4 text-sm">
      <p class="font-medium">Could not refresh your project overview.</p>
      <p class="mt-1 break-words text-muted-foreground">{error}</p>
      {#if model}<p class="mt-2 text-muted-foreground">Showing the last loaded project data.</p>{/if}
      <Button variant="outline" class="mt-3 min-h-11" disabled={busy} onclick={refresh}>Try again</Button>
    </div>
  {/if}

  {#if !model && !error}
    <div role="status" aria-label="Loading project overview" class="space-y-6">
      <p class="text-sm text-muted-foreground">Loading your project…</p>
      <div class="h-7 w-52 rounded bg-muted"></div>
      <div class="grid grid-cols-2 gap-4 sm:grid-cols-4">{#each Array(4) as _, i (i)}<div class="h-16 rounded bg-muted"></div>{/each}</div>
      <div class="h-28 rounded bg-muted"></div>
    </div>
  {:else if model}
    <header class="mb-6">
      <h2 class="break-words text-xl font-semibold tracking-tight">{project?.name || 'Your project'}</h2>
      {#if project?.description}<p class="mt-1 max-w-prose break-words text-sm text-muted-foreground">{project.description}</p>{/if}
    </header>

    <section aria-label="Your next step" class="flex flex-wrap items-center justify-between gap-5 rounded-xl border border-border bg-card p-5 sm:p-6">
      <div class="min-w-0 max-w-prose flex-1 basis-64">
        <h3
          class="text-lg font-semibold">{model.primary.id === 'progress' || model.primary.id === 'session-progress'
              ? 'Work is in progress'
              : model.primary.id === 'new-story'
                ? 'Start your first story'
                : model.primary.id === 'studio'
                  ? 'Pick up where you left off'
                  : 'Your next step'}</h3>
        <p class="mt-2 max-w-[55ch] text-sm leading-relaxed text-muted-foreground">{model.primary.detail}</p>
        {#if model.runningCount}<p
          class="mt-2 text-xs text-muted-foreground">{model.runningCount} {model.runningCount === 1 ? 'task' : 'tasks'} in progress</p>{/if}
      </div>
      {#if model.primary.id === 'studio'}<Button class="min-h-11" onclick={onopenStudio}>{model.primary.label}<ArrowRight class="size-4" /></Button>
      {:else}<Button class="min-h-11" href={model.primary.href}>{model.primary.label}<ArrowRight class="size-4" /></Button>{/if}
    </section>

    {#if secondaryAttention.length}
      <section aria-label="Needs attention" class="mt-4">
        <ul class="space-y-3">
          {#each secondaryAttention as item (item.id)}<li
            class="flex flex-wrap items-center justify-between gap-x-5 gap-y-1 px-1"><div
            class="min-w-0 flex-1 basis-56"><p class="text-sm font-medium">{item.label}</p><p
            class="mt-1 text-sm text-muted-foreground">{item.detail}</p></div><a
            href={item.href}
            class="inline-flex min-h-11 items-center gap-2 text-sm underline underline-offset-4">{item.action}<ArrowRight class="size-3.5" /></a></li>{/each}
        </ul>
      </section>
    {/if}

    <section aria-label="Project totals" class="my-7 border-y border-border py-4">
      <ul class="grid grid-cols-2 gap-x-6 gap-y-2 sm:grid-cols-4">
        {#each metrics as item (item.label)}
          <li><a
            href={item.href}
            aria-label={`${item.label}: ${item.value}`}
            class="group inline-flex min-h-11 items-baseline gap-2.5 rounded-sm hover:text-primary focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ring"><span
              class="text-xl font-semibold tabular-nums">{item.value}</span><span
              class="text-sm text-muted-foreground group-hover:text-foreground">{item.label}</span></a></li>
        {/each}
      </ul>
    </section>

    <div class="grid gap-7 md:grid-cols-[minmax(0,1fr)_15rem] md:gap-10">
      <section aria-labelledby="overview-recent">
        <h3 id="overview-recent" class="mb-2 text-sm font-semibold">Recent activity</h3>
        {#if model.recent.length}
          <ol class="divide-y divide-border">
            {#each model.recent as item (item.id)}<li><a
              href={item.href}
              class="flex flex-wrap items-center justify-between gap-2 rounded-sm py-3 hover:bg-muted/30 focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ring"><div
                class="min-w-0 flex-1 basis-40"><p class="break-words text-sm font-medium">{item.title}</p><p
                class="mt-1 text-xs text-muted-foreground">{item.kind} · {item.status}</p></div><time
              datetime={item.date}
              class="text-xs text-muted-foreground">{formatTime(item.date)}</time></a></li>{/each}
          </ol>
        {:else}
          <p class="py-3 text-sm leading-relaxed text-muted-foreground">No activity yet. Start a story or add your first reference to begin.</p>
        {/if}
      </section>
      <section aria-labelledby="overview-actions" class="md:border-l md:border-border md:pl-6">
        <h3 id="overview-actions" class="mb-2 text-sm font-semibold">Quick actions</h3>
        <div class="flex flex-col">
          {#if model.primary.id !== 'new-story'}<a
            href="/?view=story&new_story=1"
            class="flex min-h-11 items-center justify-between gap-3 text-sm hover:underline underline-offset-4">New story<ArrowRight
              class="size-3.5 text-muted-foreground" /></a>{/if}
          <a
            href="/library?tab=media"
            class="flex min-h-11 items-center justify-between gap-3 text-sm hover:underline underline-offset-4">Add references<ArrowRight
              class="size-3.5 text-muted-foreground" /></a>
          <a
            href="/library?tab=characters"
            class="flex min-h-11 items-center justify-between gap-3 text-sm hover:underline underline-offset-4">Manage characters<ArrowRight
              class="size-3.5 text-muted-foreground" /></a>
        </div>
      </section>
    </div>
  {/if}
</div>
