<script lang="ts">
  import { onMount } from 'svelte';
  import { get } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import { ArrowRight, RefreshCw } from '@lucide/svelte';
  import { makeOverview } from './overview.mjs';

  let { onopenStudio }: { onopenStudio: () => void } = $props();
  let project: any = $state(null);
  let model = $state<ReturnType<typeof makeOverview> | null>(null);
  let busy = $state(false), error = $state('');
  let updatedAt = $state('');
  const secondaryAttention = $derived(model?.attention.filter(item => item.id !== model?.primary.id) ?? []);
  const metrics = $derived(model ? [
    { label: 'Scenes', value: model.metrics.scenes, href: '/scenes' },
    { label: 'Characters', value: model.metrics.characters, href: '/characters' },
    { label: 'Assets', value: model.metrics.assets, href: '/assets' },
    { label: 'Finished videos', value: model.metrics.videos, href: '/render' }
  ] : []);
  const formatTime = (date: string) => new Date(date).toLocaleString(undefined, { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' });

  export async function refresh() {
    if (busy) return;
    busy = true; error = '';
    try {
      const [active, graph, assets, ops, jobs, sessions, aiHealth, appHealth] = await Promise.all([
        get('/projects/active'), get('/graph'), get('/assets'), get('/ops?limit=all'), get('/render-jobs'),
        get('/director/sessions').catch(() => null), get('/director/status').catch(() => null), get('/health').catch(() => null)
      ]);
      const next = makeOverview({ graph, assets, ops, jobs, sessions, aiHealth, appHealth });
      project = active; model = next; updatedAt = new Date().toISOString();
    } catch (e: any) { error = e.message ?? String(e); }
    finally { busy = false; }
  }
  onMount(() => { void refresh(); });
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
    <header class="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div class="min-w-0 flex-1">
        <h2 class="break-words text-xl font-semibold tracking-tight">{project?.name || 'Your project'}</h2>
        {#if project?.description}<p class="mt-1 max-w-prose break-words text-sm text-muted-foreground">{project.description}</p>{/if}
      </div>
      <Button variant="ghost" class="min-h-11" disabled={busy} onclick={refresh} aria-label="Refresh overview"><RefreshCw class="size-4" /><span class="hidden sm:inline">Refresh</span></Button>
    </header>

    <section aria-label="Project totals" class="border-y border-border py-5">
      <div class="mb-4 flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground"><p>Project totals</p><p>{busy ? 'Refreshing…' : 'Updated ' + formatTime(updatedAt)}</p></div>
      <ul class="grid grid-cols-2 gap-5 sm:grid-cols-4">
        {#each metrics as item (item.label)}
          <li><a href={item.href} aria-label={`${item.label}: ${item.value}`} class="group inline-flex min-h-11 items-baseline gap-3 rounded-sm hover:text-primary focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ring"><span class="text-2xl font-semibold tabular-nums">{item.value}</span><span class="text-sm text-muted-foreground group-hover:text-foreground">{item.label}</span></a></li>
        {/each}
      </ul>
    </section>

    <section aria-label="Your next step" class="flex flex-wrap items-start justify-between gap-5 py-7">
      <div class="min-w-0 max-w-prose flex-1 basis-64">
        <h3 class="text-lg font-semibold">{model.primary.id === 'progress' || model.primary.id === 'session-progress' ? 'Work is in progress' : model.primary.id === 'new-story' ? 'Start your first story' : model.primary.id === 'studio' ? 'Pick up where you left off' : 'Your next step'}</h3>
        <p class="mt-2 text-sm leading-relaxed text-muted-foreground">{model.primary.detail}</p>
        {#if model.runningCount}<p class="mt-2 text-xs text-muted-foreground">{model.runningCount} {model.runningCount === 1 ? 'task' : 'tasks'} in progress</p>{/if}
      </div>
      {#if model.primary.id === 'studio'}<Button class="min-h-11" onclick={onopenStudio}>{model.primary.label}<ArrowRight class="size-4" /></Button>
      {:else}<Button class="min-h-11" href={model.primary.href}>{model.primary.label}<ArrowRight class="size-4" /></Button>{/if}
    </section>

    {#if secondaryAttention.length}
      <section aria-labelledby="overview-attention" class="border-t border-border py-6">
        <h3 id="overview-attention" class="mb-3 text-sm font-semibold">Needs attention</h3>
        <ul class="divide-y divide-border">
          {#each secondaryAttention as item (item.id)}<li class="flex flex-wrap items-center justify-between gap-3 py-3"><div class="min-w-0 flex-1 basis-56"><p class="text-sm font-medium">{item.label}</p><p class="mt-1 text-sm text-muted-foreground">{item.detail}</p></div><a href={item.href} class="inline-flex min-h-11 items-center gap-2 text-sm underline underline-offset-4">{item.action}<ArrowRight class="size-3.5" /></a></li>{/each}
        </ul>
      </section>
    {/if}

    <section aria-labelledby="overview-actions" class="border-t border-border py-6">
      <h3 id="overview-actions" class="mb-3 text-sm font-semibold">Quick actions</h3>
      <div class="flex flex-wrap gap-2">
        {#if model.primary.href !== '/create'}<Button variant="outline" class="min-h-11" href="/create">New story</Button>{/if}
        <Button variant="outline" class="min-h-11" href="/assets">Add references</Button>
        <Button variant="outline" class="min-h-11" href="/characters">Manage characters</Button>
      </div>
    </section>

    <section aria-labelledby="overview-recent" class="border-t border-border pt-6">
      <h3 id="overview-recent" class="mb-3 text-sm font-semibold">Recent activity</h3>
      {#if model.recent.length}
        <ol class="divide-y divide-border">
          {#each model.recent as item (item.id)}<li><a href={item.href} class="flex flex-wrap items-center justify-between gap-2 rounded-sm py-4 hover:bg-muted/30 focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ring"><div class="min-w-0 flex-1 basis-48"><p class="break-words text-sm font-medium">{item.title}</p><p class="mt-1 text-xs text-muted-foreground">{item.kind} · {item.status}</p></div><time datetime={item.date} class="text-xs text-muted-foreground">{formatTime(item.date)}</time></a></li>{/each}
        </ol>
      {:else}
        <p class="py-3 text-sm text-muted-foreground">No activity yet. Start a story or add your first reference to begin.</p>
      {/if}
    </section>
  {/if}
</div>
