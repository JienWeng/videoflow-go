<script lang="ts">
  // What the open project has spent on paid services, and where the money went.
  import { onMount } from 'svelte';
  import { get } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import { Badge } from '$lib/components/ui/badge';
  import { Skeleton } from '$lib/components/ui/skeleton';
  import * as Card from '$lib/components/ui/card';
  import * as Table from '$lib/components/ui/table';
  import { LoaderCircle, TriangleAlert } from '@lucide/svelte';

  type Group = { key: string; label: string; amount: number; count: number; quantity: number; unit: string; exact: boolean };
  type Entry = { ref: string; category: string; provider: string; model: string; label: string; quantity: number; unit: string; amount: number; basis: string; created_at: string };
  type Costs = { projects?: Group[]; all_total?: number; total: number; estimated: number; since?: string; categories: Group[]; services: Group[]; episodes: Group[]; included: Group[]; unconfirmed: Entry[]; unpriced: number; items: Entry[]; fal_billed?: { amount?: number; since?: string; unavailable?: boolean } };

  let data = $state<Costs | null>(null), error = $state(''), comparing = $state(false), showAll = $state(false);
  const usd = (v: number) => (v > 0 && v < 0.01 ? 'under $0.01' : '$' + v.toFixed(2));
  const day = (v?: string) => (v ? new Date(v).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' }) : '');
  const services: Record<string, string> = { fal: 'fal.ai', openai: 'OpenAI', fish: 'Fish Audio', elevenlabs: 'ElevenLabs', openrouter: 'OpenRouter', codex: 'Codex', claude: 'Claude Code' };
  const kinds: Record<string, string> = { video: 'Video', image: 'Picture', voice: 'Voice', text: 'Text' };
  const how: Record<string, string> = { metered: 'Billed amount', rate: 'List price', estimate: 'Estimate', included: 'In your plan', unpriced: 'Price unknown' };
  function made(g: Group) {
    if (g.key === 'video') return `${g.count} ${g.count === 1 ? 'clip' : 'clips'} · ${Math.round(g.quantity)} seconds`;
    if (g.key === 'image') return `${g.count} ${g.count === 1 ? 'picture' : 'pictures'}`;
    if (g.key === 'voice') return `${g.count} ${g.count === 1 ? 'line' : 'lines'} · ${Math.round(g.quantity)} characters`;
    return `${g.count} ${g.count === 1 ? 'call' : 'calls'}`;
  }
  function amountOf(e: Entry) {
    if (e.basis === 'included') return 'No charge';
    if (e.basis === 'unpriced') return 'Unknown';
    return usd(e.amount);
  }
  function size(e: Entry) {
    if (e.unit === 'seconds') return `${Math.round(e.quantity)} s`;
    if (e.unit === 'characters') return `${Math.round(e.quantity)} characters`;
    if (e.unit === 'tokens') return `${Math.round(e.quantity)} tokens`;
    return '';
  }
  async function load(reconcile = false) {
    try { data = await get('/costs' + (reconcile ? '?reconcile=1' : '')); error = ''; }
    catch (e) { error = (e as Error).message; }
  }
  async function compare() { comparing = true; await load(true); comparing = false; }
  onMount(() => void load());
  const rows = $derived(data ? (showAll ? data.items : data.items.slice(0, 25)) : []);
</script>

<svelte:head><title>Costs · VideoFlow</title></svelte:head>

<div class="mx-auto grid w-full max-w-5xl gap-6 p-4 sm:p-6">
  <header class="grid gap-1">
    <h1 class="text-2xl font-semibold">Costs</h1>
    <p class="text-sm text-muted-foreground">What this project has spent on paid services, in US dollars.</p>
  </header>

  {#if error}
    <p role="alert" class="text-sm text-destructive">{error}</p>
  {:else if !data}
    <Skeleton class="h-28 w-full" /><Skeleton class="h-48 w-full" />
  {:else}
    <Card.Root>
      <Card.Header>
        <Card.Description>Spent so far{data.since ? ` since ${day(data.since)}` : ''}</Card.Description>
        <Card.Title class="text-4xl tabular-nums">{usd(data.total)}</Card.Title>
      </Card.Header>
      <Card.Content class="grid gap-2 text-sm text-muted-foreground">
        {#if data.items.length === 0}
          <p>Nothing paid for yet. Pictures, video clips and voice lines appear here as soon as they are made.</p>
        {:else}
          <p>Worked out from each service's published price and what was made. Your invoice can differ slightly through discounts, tax or price changes.</p>
          {#if data.estimated > 0}<p>{usd(data.estimated)} of this is an estimate: older pictures were made before exact billing was recorded.</p>{/if}
        {/if}
      </Card.Content>
    </Card.Root>

    {#if data.unconfirmed.length}
      <div role="status" class="flex gap-3 rounded-lg border border-border p-3 text-sm">
        <TriangleAlert class="mt-0.5 size-4 shrink-0" />
        <p>{data.unconfirmed.length === 1 ? '1 video clip was' : `${data.unconfirmed.length} video clips were`} sent to the video service but the result never came back. It may have charged up to {usd(data.unconfirmed.reduce((t, e) => t + e.amount, 0))} more. This is not in the total; check your fal.ai billing page.</p>
      </div>
    {/if}

    {#if data.categories.length}
      <section class="grid gap-3" aria-labelledby="by-kind">
        <h2 id="by-kind" class="text-base font-medium">By type</h2>
        <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          {#each data.categories as g (g.key)}
            <Card.Root>
              <Card.Header>
                <Card.Description>{g.label}</Card.Description>
                <Card.Title class="text-2xl tabular-nums">{usd(g.amount)}</Card.Title>
              </Card.Header>
              <Card.Content class="text-xs text-muted-foreground">{made(g)}{g.exact ? '' : ' · estimate'}</Card.Content>
            </Card.Root>
          {/each}
        </div>
      </section>

      <div class="grid gap-6 lg:grid-cols-2">
        <section class="grid content-start gap-3" aria-labelledby="by-episode">
          <h2 id="by-episode" class="text-base font-medium">By episode</h2>
          <Table.Root>
            <Table.Body>
              {#each data.episodes as g (g.key)}
                <Table.Row><Table.Cell class="whitespace-normal">{g.label}</Table.Cell><Table.Cell class="text-right tabular-nums">{usd(g.amount)}</Table.Cell></Table.Row>
              {/each}
            </Table.Body>
          </Table.Root>
        </section>
        <section class="grid content-start gap-3" aria-labelledby="by-service">
          <h2 id="by-service" class="text-base font-medium">By service</h2>
          <Table.Root>
            <Table.Body>
              {#each data.services as g (g.key)}
                <Table.Row><Table.Cell>{g.label}</Table.Cell><Table.Cell class="text-right tabular-nums">{usd(g.amount)}</Table.Cell></Table.Row>
              {/each}
            </Table.Body>
          </Table.Root>
          {#if data.services.some((g) => g.key === 'fal')}
            <div class="grid gap-2 text-sm">
              {#if data.fal_billed?.amount !== undefined}
                <p>fal.ai's own billing shows <strong class="tabular-nums">{usd(data.fal_billed.amount)}</strong> charged to your whole account since {day(data.fal_billed.since)}. That covers every project and anything made outside VideoFlow.</p>
              {:else if data.fal_billed?.unavailable}
                <p class="text-muted-foreground">fal.ai would not share billing with this key. Create a key with billing access at fal.ai, add it in Settings, then try again.</p>
              {:else}
                <Button variant="outline" size="sm" class="justify-self-start" disabled={comparing} onclick={compare}>{#if comparing}<LoaderCircle class="size-4 animate-spin motion-reduce:animate-none" />{/if}Compare with fal.ai billing</Button>
              {/if}
            </div>
          {/if}
        </section>
      </div>
    {/if}

    {#if data.included.length}
      <section class="grid gap-3" aria-labelledby="no-charge">
        <h2 id="no-charge" class="text-base font-medium">No extra charge</h2>
        <p class="text-sm text-muted-foreground">Covered by your Claude or Codex subscription, so nothing is billed per use.</p>
        <ul class="grid gap-1 text-sm">
          {#each data.included as g (g.key)}<li class="flex justify-between gap-4 border-b border-border py-2"><span>{g.label}</span><span class="tabular-nums text-muted-foreground">{g.count}</span></li>{/each}
        </ul>
      </section>
    {/if}

    {#if data.projects && data.projects.length > 1}
      <section class="grid gap-3" aria-labelledby="all-projects">
        <h2 id="all-projects" class="text-base font-medium">All projects</h2>
        <Table.Root>
          <Table.Body>
            {#each data.projects as g (g.key)}
              <Table.Row><Table.Cell class="whitespace-normal">{g.label}{g.exact ? ' (open now)' : ''}</Table.Cell><Table.Cell class="text-right tabular-nums">{usd(g.amount)}</Table.Cell></Table.Row>
            {/each}
            <Table.Row class="font-medium"><Table.Cell>Total across all projects</Table.Cell><Table.Cell class="text-right tabular-nums">{usd(data.all_total ?? 0)}</Table.Cell></Table.Row>
          </Table.Body>
        </Table.Root>
      </section>
    {/if}

    {#if data.items.length}
      <section class="grid gap-3" aria-labelledby="every-charge">
        <h2 id="every-charge" class="text-base font-medium">Every charge</h2>
        {#if data.unpriced > 0}<p class="text-sm text-muted-foreground">{data.unpriced} {data.unpriced === 1 ? 'item uses' : 'items use'} a service VideoFlow has no price for, so {data.unpriced === 1 ? 'it is' : 'they are'} not in the total.</p>{/if}
        <Table.Root>
          <Table.Header>
            <Table.Row><Table.Head>Date</Table.Head><Table.Head>What</Table.Head><Table.Head>Service</Table.Head><Table.Head>How it's known</Table.Head><Table.Head class="text-right">Cost</Table.Head></Table.Row>
          </Table.Header>
          <Table.Body>
            {#each rows as e (e.ref)}
              <Table.Row>
                <Table.Cell class="whitespace-nowrap text-muted-foreground">{day(e.created_at)}</Table.Cell>
                <Table.Cell class="max-w-72 whitespace-normal"><span class="font-medium">{kinds[e.category] ?? e.category}</span> · {e.label}{size(e) ? ` · ${size(e)}` : ''}</Table.Cell>
                <Table.Cell>{services[e.provider] ?? e.provider}</Table.Cell>
                <Table.Cell><Badge variant="outline">{e.basis === 'unconfirmed' ? 'Not confirmed' : (how[e.basis] ?? e.basis)}</Badge></Table.Cell>
                <Table.Cell class="text-right tabular-nums">{e.basis === 'unconfirmed' ? `up to ${usd(e.amount)}` : amountOf(e)}</Table.Cell>
              </Table.Row>
            {/each}
          </Table.Body>
        </Table.Root>
        {#if data.items.length > 25}<Button variant="ghost" size="sm" class="justify-self-start" onclick={() => (showAll = !showAll)}>{showAll ? 'Show fewer' : `Show all ${data.items.length}`}</Button>{/if}
      </section>
    {/if}
  {/if}
</div>
