<script lang="ts">
  // Cost report: a read-only view of what paid services have charged, built to be
  // reviewed, filtered, exported and shared.
  import { onMount } from 'svelte';
  import { get } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import { Badge } from '$lib/components/ui/badge';
  import { Skeleton } from '$lib/components/ui/skeleton';
  import * as Table from '$lib/components/ui/table';
  import { Download, Printer, RefreshCw, TriangleAlert, X } from '@lucide/svelte';
  import VideoServices from '$lib/costs/VideoServices.svelte';

  type Entry = { ref: string; project_id: string; script_id: string; category: string; provider: string; model: string; label: string; quantity: number; unit: string; amount: number; basis: string; created_at: string };
  type Free = { key: string; label: string; count: number };
  type Costs = { project_id: string; project_names: Record<string, string>; episode_names: Record<string, string>; generated_at: string; all_items: Entry[]; included: Free[]; fal_billed?: { amount?: number; since?: string; unavailable?: boolean } };

  let data = $state<Costs | null>(null), error = $state(''), loading = $state(false);
  let scope = $state<'project' | 'all'>('project'), period = $state<'all' | '7' | '30'>('all');
  let day = $state(''), kind = $state(''), service = $state(''), group = $state('');
  let showAll = $state(false), hover = $state<{ x: number; label: string; value: string } | null>(null);

  const kinds: Record<string, string> = { video: 'Video clips', image: 'Pictures', voice: 'Voice', text: 'Text analysis' };
  const kindOne: Record<string, string> = { video: 'Video', image: 'Picture', voice: 'Voice', text: 'Text' };
  const services: Record<string, string> = { fal: 'fal.ai', openai: 'OpenAI', fish: 'Fish Audio', elevenlabs: 'ElevenLabs', openrouter: 'OpenRouter', codex: 'Codex', claude: 'Claude Code', byteplus: 'BytePlus', argolink: 'ArgoLink', loopling: 'Loopling', replicate: 'Replicate', kie: 'Kie.ai', wavespeed: 'WaveSpeed', runway: 'Runway', higgsfield: 'Higgsfield' };
  const how: Record<string, string> = { metered: 'Billed amount', rate: 'List price', estimate: 'Estimate', included: 'In your plan', unpriced: 'Price unknown', unconfirmed: 'Not confirmed' };
  const priced = (e: Entry) => e.basis === 'metered' || e.basis === 'rate' || e.basis === 'estimate';
  const usd = (v: number) => (v > 0 && v < 0.01 ? 'under $0.01' : '$' + v.toFixed(2));
  const dayKey = (v: string) => v.slice(0, 10);
  const date = (v?: string) => (v ? new Date(v).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' }) : '');
  const shortDay = (k: string) => new Date(k + 'T00:00:00').toLocaleDateString(undefined, { day: 'numeric', month: 'short' });
  const sum = (list: Entry[]) => list.reduce((t, e) => t + e.amount, 0);
  const serviceName = (p: string) => services[p] ?? p;
  // What a row belongs to: a project in the all-projects view, an episode inside one project.
  const groupKey = (e: Entry) => (scope === 'all' ? e.project_id : e.script_id);
  function groupName(key: string) {
    if (!data) return key;
    if (scope === 'all') return data.project_names[key] ?? 'Deleted project';
    return data.episode_names[key] ?? 'Shared across episodes';
  }

  async function load(reconcile = false) {
    loading = true;
    try { data = await get('/costs' + (reconcile ? '?reconcile=1' : '')); error = ''; }
    catch (e) { error = (e as Error).message; }
    loading = false;
  }
  onMount(() => void load());

  // Rows in scope and period: the base every chart and total is drawn from.
  const base = $derived.by(() => {
    if (!data) return [] as Entry[];
    const since = period === 'all' ? 0 : Date.now() - Number(period) * 86400000;
    return data.all_items.filter((e) => (scope === 'all' || e.project_id === data!.project_id) && new Date(e.created_at).getTime() >= since);
  });
  const match = (e: Entry, skip = '') =>
    (skip === 'day' || !day || dayKey(e.created_at) === day) && (skip === 'kind' || !kind || e.category === kind) &&
    (skip === 'service' || !service || e.provider === service) && (skip === 'group' || !group || groupKey(e) === group);
  const rows = $derived(base.filter((e) => match(e)));
  const paid = $derived(rows.filter(priced));
  const total = $derived(sum(paid));
  const estimated = $derived(sum(paid.filter((e) => e.basis === 'estimate')));
  const unconfirmed = $derived(rows.filter((e) => e.basis === 'unconfirmed'));
  const filtered = $derived(!!(day || kind || service || group));

  // Each chart ignores its own filter, so the selected bar stays in context among the others.
  function breakdown(skip: string, key: (e: Entry) => string, name: (k: string) => string) {
    const totals = new Map<string, { amount: number; count: number }>();
    for (const e of base.filter((e) => priced(e) && match(e, skip))) {
      const t = totals.get(key(e)) ?? { amount: 0, count: 0 };
      t.amount += e.amount; t.count++; totals.set(key(e), t);
    }
    return [...totals].map(([k, t]) => ({ key: k, label: name(k), ...t })).sort((a, b) => b.amount - a.amount);
  }
  const byKind = $derived(breakdown('kind', (e) => e.category, (k) => kinds[k] ?? k));
  const byService = $derived(breakdown('service', (e) => e.provider, serviceName));
  const byGroup = $derived(breakdown('group', groupKey, groupName));
  const byDay = $derived.by(() => {
    const totals = new Map<string, number>();
    for (const e of base.filter((e) => priced(e) && match(e, 'day'))) totals.set(dayKey(e.created_at), (totals.get(dayKey(e.created_at)) ?? 0) + e.amount);
    const keys = [...totals.keys()].sort();
    if (!keys.length) return [] as { key: string; amount: number }[];
    const out: { key: string; amount: number }[] = [];
    const end = new Date(keys[keys.length - 1] + 'T00:00:00Z');
    for (let d = new Date(keys[0] + 'T00:00:00Z'); d <= end && out.length < 400; d.setUTCDate(d.getUTCDate() + 1)) {
      const k = d.toISOString().slice(0, 10);
      out.push({ key: k, amount: totals.get(k) ?? 0 });
    }
    return out.slice(-60);
  });
  const dayMax = $derived(Math.max(0.01, ...byDay.map((d) => d.amount)));

  const range = $derived.by(() => {
    if (!rows.length) return '';
    const times = rows.map((e) => new Date(e.created_at).getTime());
    const a = date(new Date(Math.min(...times)).toISOString()), b = date(new Date(Math.max(...times)).toISOString());
    return a === b ? a : `${a} to ${b}`;
  });
  const title = $derived(scope === 'all' ? 'All projects' : (data?.project_names[data.project_id] ?? 'This project'));

  const findings = $derived.by(() => {
    const out: string[] = [];
    if (!paid.length) return out;
    const video = paid.filter((e) => e.category === 'video'), seconds = video.reduce((t, e) => t + e.quantity, 0);
    if (video.length && total > 0) out.push(`Video is ${Math.round((sum(video) / total) * 100)}% of the spend: ${usd(sum(video))} for ${video.length} ${video.length === 1 ? 'clip' : 'clips'} (${Math.round(seconds)} seconds, about ${usd(sum(video) / Math.max(1, seconds))} per second).`);
    const top = byGroup[0];
    if (top && byGroup.length > 1 && !group) out.push(`${top.label} cost the most: ${usd(top.amount)}, ${Math.round((top.amount / total) * 100)}% of the total.`);
    const big = [...paid].sort((a, b) => b.amount - a.amount)[0];
    if (big) out.push(`The largest single charge was ${usd(big.amount)} for "${big.label}" on ${serviceName(big.provider)}.`);
    const pictures = paid.filter((e) => e.category === 'image');
    if (pictures.length) out.push(`${pictures.length} paid ${pictures.length === 1 ? 'picture' : 'pictures'} cost ${usd(sum(pictures))} in total, about ${usd(sum(pictures) / pictures.length)} each.`);
    if (estimated > 0) out.push(`${usd(estimated)} of the total is an estimate, from pictures made before exact billing was recorded.`);
    return out;
  });

  function clear() { day = ''; kind = ''; service = ''; group = ''; }
  function size(e: Entry) {
    if (e.unit === 'seconds') return `${Math.round(e.quantity)} s`;
    if (e.unit === 'characters') return `${Math.round(e.quantity)} characters`;
    if (e.unit === 'tokens') return `${Math.round(e.quantity)} tokens`;
    return '';
  }
  function cost(e: Entry) {
    if (e.basis === 'included') return 'No charge';
    if (e.basis === 'unpriced') return 'Unknown';
    if (e.basis === 'unconfirmed') return 'up to ' + usd(e.amount);
    return usd(e.amount);
  }
  function exportCSV() {
    const cell = (v: string | number) => `"${String(v).replace(/"/g, '""')}"`;
    const head = ['Date', 'Project', 'Episode', 'Type', 'Service', 'Model', 'Item', 'Quantity', 'Unit', 'Cost (USD)', 'How it is known'];
    const lines = rows.map((e) => [e.created_at, data!.project_names[e.project_id] ?? 'Deleted project', data!.episode_names[e.script_id] ?? '', kindOne[e.category] ?? e.category, serviceName(e.provider), e.model, e.label, e.quantity, e.unit, priced(e) ? e.amount.toFixed(4) : '', how[e.basis] ?? e.basis].map(cell).join(','));
    const link = document.createElement('a');
    link.href = URL.createObjectURL(new Blob(['﻿' + [head.map(cell).join(','), ...lines].join('\r\n')], { type: 'text/csv' }));
    link.download = `videoflow-costs-${new Date().toISOString().slice(0, 10)}.csv`;
    link.click();
    URL.revokeObjectURL(link.href);
  }
  const shown = $derived(showAll ? rows : rows.slice(0, 25));
  const sections = [['summary', 'Summary'], ['over-time', 'Spend over time'], ['breakdown', 'Where the money went'], ['charges', 'Every charge'], ['no-charge', 'No extra charge'], ['services', 'Video service prices']];
</script>

<svelte:head><title>Cost report · VideoFlow</title></svelte:head>

{#snippet bars(title: string, list: { key: string; label: string; amount: number; count: number }[], selected: string, pick: (k: string) => void)}
  <div class="grid content-start gap-2">
    <h3 class="text-sm font-medium">{title}</h3>
    {#if list.length === 0}<p class="text-sm text-muted-foreground">Nothing to show.</p>{/if}
    <ul class="grid gap-1">
      {#each list as row (row.key)}
        <li>
          <button type="button" aria-pressed={selected === row.key} title={`${row.label}: ${usd(row.amount)} · ${row.count} ${row.count === 1 ? 'charge' : 'charges'}. Click to filter.`}
            class="grid w-full grid-cols-[minmax(0,10rem)_minmax(2rem,1fr)_auto] items-center gap-3 rounded-md px-2 py-1.5 text-left text-sm hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring aria-pressed:bg-muted {selected && selected !== row.key ? 'opacity-50' : ''}"
            onclick={() => pick(selected === row.key ? '' : row.key)}>
            <span class="truncate">{row.label}</span>
            <span class="h-3"><span class="bar block h-full rounded-r-[4px]" style="width:{Math.max(1, (row.amount / (list[0].amount || 1)) * 100)}%"></span></span>
            <span class="tabular-nums">{usd(row.amount)}</span>
          </button>
        </li>
      {/each}
    </ul>
  </div>
{/snippet}

<div class="report print-area mx-auto w-full max-w-6xl p-4 sm:p-6">
  <header class="grid gap-4 border-b border-border pb-5">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div class="grid gap-1">
        <p class="text-xs font-medium uppercase tracking-wide text-muted-foreground">Cost report</p>
        <h1 class="text-2xl font-semibold">{title}</h1>
      </div>
      <div class="no-print flex flex-wrap gap-2">
        <Button variant="outline" size="sm" disabled={!rows.length} onclick={exportCSV}><Download class="size-4" />Download CSV</Button>
        <Button variant="outline" size="sm" onclick={() => window.print()}><Printer class="size-4" />Print or save as PDF</Button>
        <Button variant="ghost" size="sm" disabled={loading} onclick={() => load()} aria-label="Refresh the report"><RefreshCw class="size-4 {loading ? 'animate-spin motion-reduce:animate-none' : ''}" /></Button>
      </div>
    </div>
    {#if data}
      <dl class="grid grid-cols-2 gap-x-6 gap-y-2 text-sm sm:grid-cols-4">
        <div><dt class="text-xs text-muted-foreground">Covers</dt><dd>{range || 'No charges yet'}</dd></div>
        <div><dt class="text-xs text-muted-foreground">Generated</dt><dd>{new Date(data.generated_at).toLocaleString(undefined, { dateStyle: 'medium', timeStyle: 'short' })}</dd></div>
        <div><dt class="text-xs text-muted-foreground">Prepared by</dt><dd>VideoFlow, from its own record of every paid request</dd></div>
        <div><dt class="text-xs text-muted-foreground">Currency</dt><dd>US dollars, at each service's list price</dd></div>
      </dl>
      <div class="no-print flex flex-wrap items-end gap-3">
        <label class="grid gap-1 text-xs font-medium">Show
          <select class="h-9 rounded-md border border-input bg-background px-2 text-sm font-normal" bind:value={scope} onchange={clear}>
            <option value="project">{data.project_names[data.project_id] ?? 'This project'}</option><option value="all">All projects</option>
          </select>
        </label>
        <label class="grid gap-1 text-xs font-medium">Period
          <select class="h-9 rounded-md border border-input bg-background px-2 text-sm font-normal" bind:value={period} onchange={() => (day = '')}>
            <option value="all">All time</option><option value="30">Last 30 days</option><option value="7">Last 7 days</option>
          </select>
        </label>
        {#if filtered}
          <div class="flex flex-wrap items-center gap-2 pb-1">
            <span class="text-xs text-muted-foreground">Filtered to</span>
            {#if day}<Badge variant="secondary">{shortDay(day)}</Badge>{/if}
            {#if kind}<Badge variant="secondary">{kinds[kind] ?? kind}</Badge>{/if}
            {#if service}<Badge variant="secondary">{serviceName(service)}</Badge>{/if}
            {#if group}<Badge variant="secondary">{groupName(group)}</Badge>{/if}
            <Button variant="ghost" size="sm" onclick={clear}><X class="size-4" />Clear filters</Button>
          </div>
        {/if}
      </div>
    {/if}
  </header>

  {#if error}
    <p role="alert" class="mt-6 text-sm text-destructive">{error}</p>
  {:else if !data}
    <div class="mt-6 grid gap-4"><Skeleton class="h-28 w-full" /><Skeleton class="h-56 w-full" /></div>
  {:else}
    <div class="mt-6 grid gap-8 lg:grid-cols-[11rem_minmax(0,1fr)]">
      <nav aria-label="Report sections" class="no-print hidden content-start gap-1 self-start text-sm lg:sticky lg:top-6 lg:grid">
        <p class="mb-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">Contents</p>
        {#each sections as [id, name]}<a class="rounded-md px-2 py-1.5 text-muted-foreground hover:bg-muted hover:text-foreground" href={'#' + id}>{name}</a>{/each}
      </nav>

      <div class="grid min-w-0 gap-10">
        <section id="summary" class="grid scroll-mt-6 gap-4" aria-labelledby="summary-h">
          <h2 id="summary-h" class="text-lg font-semibold">Summary</h2>
          <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <div class="rounded-lg border border-border p-4"><p class="text-xs text-muted-foreground">Total spent</p><p class="mt-1 text-3xl font-semibold tabular-nums">{usd(total)}</p><p class="mt-1 text-xs text-muted-foreground">{paid.length} paid {paid.length === 1 ? 'charge' : 'charges'}</p></div>
            {#each byKind.slice(0, 3) as k (k.key)}
              <div class="rounded-lg border border-border p-4"><p class="text-xs text-muted-foreground">{k.label}</p><p class="mt-1 text-3xl font-semibold tabular-nums">{usd(k.amount)}</p><p class="mt-1 text-xs text-muted-foreground">{total > 0 ? Math.round((k.amount / total) * 100) : 0}% of the total · {k.count} {k.count === 1 ? 'charge' : 'charges'}</p></div>
            {/each}
          </div>
          {#if findings.length}
            <div class="grid gap-2">
              <h3 class="text-sm font-medium">Key findings</h3>
              <ul class="grid list-disc gap-1.5 pl-5 text-sm">{#each findings as f}<li>{f}</li>{/each}</ul>
            </div>
          {:else}
            <p class="text-sm text-muted-foreground">Nothing has been paid for {filtered || period !== 'all' ? 'with these filters' : 'yet'}. Pictures, video clips and voice lines appear here as soon as they are made.</p>
          {/if}
          {#if unconfirmed.length}
            <div role="status" class="flex gap-3 rounded-lg border border-border p-3 text-sm">
              <TriangleAlert class="mt-0.5 size-4 shrink-0" />
              <p>{unconfirmed.length === 1 ? '1 video clip was' : `${unconfirmed.length} video clips were`} sent to the video service but the result never came back. It may have charged up to {usd(sum(unconfirmed))} more. This is not in the total; check that service's billing page.</p>
            </div>
          {/if}
        </section>

        <section id="over-time" class="grid scroll-mt-6 gap-3" aria-labelledby="time-h">
          <div><h2 id="time-h" class="text-lg font-semibold">Spend over time</h2><p class="text-sm text-muted-foreground">Dollars spent each day. Click a day to see only its charges.</p></div>
          {#if byDay.length === 0}
            <p class="text-sm text-muted-foreground">No paid charges in this view.</p>
          {:else}
            <div class="relative">
              <div class="flex items-center justify-between text-xs text-muted-foreground"><span>{usd(dayMax)}</span><span>busiest day</span></div>
              <div class="flex h-40 items-end gap-[2px] border-b border-t border-border" role="group" aria-label="Spend per day" onmouseleave={() => (hover = null)}>
                {#each byDay as d, i (d.key)}
                  <button type="button" aria-pressed={day === d.key} aria-label={`${shortDay(d.key)}: ${usd(d.amount)}`}
                    class="flex h-full min-w-0 flex-1 items-end justify-center hover:bg-muted focus-visible:bg-muted focus-visible:outline-2 focus-visible:outline-ring {day && day !== d.key ? 'opacity-50' : ''}"
                    onclick={() => (day = day === d.key ? '' : d.key)}
                    onmouseenter={() => (hover = { x: ((i + 0.5) / byDay.length) * 100, label: shortDay(d.key), value: usd(d.amount) })}
                    onfocus={() => (hover = { x: ((i + 0.5) / byDay.length) * 100, label: shortDay(d.key), value: usd(d.amount) })} onblur={() => (hover = null)}>
                    <span class="bar block w-full max-w-6 rounded-t-[4px]" style="height:{d.amount > 0 ? Math.max(2, (d.amount / dayMax) * 100) : 0}%"></span>
                  </button>
                {/each}
              </div>
              {#if hover}
                <div class="pointer-events-none absolute top-6 z-10 -translate-x-1/2 rounded-md border border-border bg-popover px-2 py-1 text-xs shadow-sm" style="left:clamp(3rem, {hover.x}%, calc(100% - 3rem))"><span class="font-medium tabular-nums">{hover.value}</span> <span class="text-muted-foreground">{hover.label}</span></div>
              {/if}
              <div class="mt-1 flex justify-between text-xs text-muted-foreground"><span>{shortDay(byDay[0].key)}</span>{#if byDay.length > 1}<span>{shortDay(byDay[byDay.length - 1].key)}</span>{/if}</div>
            </div>
          {/if}
        </section>

        <section id="breakdown" class="grid scroll-mt-6 gap-4" aria-labelledby="break-h">
          <div><h2 id="break-h" class="text-lg font-semibold">Where the money went</h2><p class="text-sm text-muted-foreground">Click any bar to narrow the whole report to it. Click it again to undo.</p></div>
          <div class="grid gap-8 lg:grid-cols-3">
            {@render bars('By type', byKind, kind, (k) => (kind = k))}
            {@render bars('By service', byService, service, (k) => (service = k))}
            {@render bars(scope === 'all' ? 'By project' : 'By episode', byGroup, group, (k) => (group = k))}
          </div>
          {#if byService.some((g) => g.key === 'fal')}
            <div class="no-print text-sm">
              {#if data.fal_billed?.amount !== undefined}
                <p>fal.ai's own billing shows <strong class="tabular-nums">{usd(data.fal_billed.amount)}</strong> charged to your whole account since {date(data.fal_billed.since)}, including anything made outside VideoFlow.</p>
              {:else if data.fal_billed?.unavailable}
                <p class="text-muted-foreground">fal.ai would not share billing with this key. Create a key with billing access at fal.ai, add it in Settings, then try again.</p>
              {:else}
                <Button variant="outline" size="sm" disabled={loading} onclick={() => load(true)}>Compare with fal.ai billing</Button>
              {/if}
            </div>
          {/if}
        </section>

        <section id="charges" class="grid scroll-mt-6 gap-3" aria-labelledby="charges-h">
          <div class="flex flex-wrap items-end justify-between gap-2">
            <div><h2 id="charges-h" class="text-lg font-semibold">Every charge</h2><p class="text-sm text-muted-foreground">{rows.length} {rows.length === 1 ? 'row' : 'rows'}{filtered ? ' with the current filters' : ''} · {usd(total)} paid</p></div>
          </div>
          {#if rows.length}
            <Table.Root>
              <Table.Header>
                <Table.Row><Table.Head>Date</Table.Head>{#if scope === 'all'}<Table.Head>Project</Table.Head>{/if}<Table.Head>What</Table.Head><Table.Head>Service</Table.Head><Table.Head>How it's known</Table.Head><Table.Head class="text-right">Cost</Table.Head></Table.Row>
              </Table.Header>
              <Table.Body>
                {#each shown as e (e.ref)}
                  <Table.Row>
                    <Table.Cell class="whitespace-nowrap text-muted-foreground">{date(e.created_at)}</Table.Cell>
                    {#if scope === 'all'}<Table.Cell class="max-w-40 whitespace-normal">{data.project_names[e.project_id] ?? 'Deleted project'}</Table.Cell>{/if}
                    <Table.Cell class="max-w-72 whitespace-normal"><span class="font-medium">{kindOne[e.category] ?? e.category}</span> · {e.label.replace(/^Voice · /, '').replace(/\.mp3$/, '')}{size(e) ? ` · ${size(e)}` : ''}</Table.Cell>
                    <Table.Cell>{serviceName(e.provider)}</Table.Cell>
                    <Table.Cell><Badge variant="outline">{how[e.basis] ?? e.basis}</Badge></Table.Cell>
                    <Table.Cell class="text-right tabular-nums">{cost(e)}</Table.Cell>
                  </Table.Row>
                {/each}
              </Table.Body>
            </Table.Root>
            {#if rows.length > 25}<Button variant="ghost" size="sm" class="no-print justify-self-start" onclick={() => (showAll = !showAll)}>{showAll ? 'Show fewer' : `Show all ${rows.length}`}</Button>{/if}
          {:else}
            <p class="text-sm text-muted-foreground">No charges match.</p>
          {/if}
        </section>

        <section id="no-charge" class="grid scroll-mt-6 gap-3" aria-labelledby="free-h">
          <div><h2 id="free-h" class="text-lg font-semibold">No extra charge</h2><p class="text-sm text-muted-foreground">Work in the open project covered by your Claude or Codex subscription, so nothing is billed per use.</p></div>
          {#if data.included.length}
            <ul class="grid gap-1 text-sm">{#each data.included as g (g.key)}<li class="flex justify-between gap-4 border-b border-border py-2"><span>{g.label}</span><span class="tabular-nums text-muted-foreground">{g.count}</span></li>{/each}</ul>
          {:else}<p class="text-sm text-muted-foreground">Nothing yet.</p>{/if}
        </section>

        <div id="services" class="no-print scroll-mt-6 border-t border-border pt-8"><VideoServices /></div>

        <p class="text-xs text-muted-foreground">Amounts are worked out from each service's published price and what was made. Your invoice can differ slightly through discounts, tax or price changes.</p>
      </div>
    </div>
  {/if}
</div>

<style>
  /* One series, so one color: the label beside each bar says what it is. */
  .report { --series: #2a78d6; }
  :global(.dark) .report { --series: #3987e5; }
  .bar { background: var(--series); print-color-adjust: exact; -webkit-print-color-adjust: exact; }
</style>
