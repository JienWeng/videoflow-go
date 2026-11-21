<script lang="ts">
  // Compare what each service charges for the same video model, connect services,
  // and choose between "always the cheapest" and one fixed service.
  import { onMount } from 'svelte';
  import { get, post, put } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import { Badge } from '$lib/components/ui/badge';
  import { Input } from '$lib/components/ui/input';
  import { Skeleton } from '$lib/components/ui/skeleton';
  import * as Table from '$lib/components/ui/table';
  import { LoaderCircle } from '@lucide/svelte';

  type Service = { provider: string; label: string; model: string; per_second: number; total: number; checked: string; connected: boolean; usable: boolean; checked_at?: string; problem?: string; cheapest: boolean; in_use: boolean; note?: string; key_check?: { state: string; message?: string } };
  type State = { families: { id: string; label: string }[]; model: string; resolution: string; seconds: number; routing: string; current: { provider: string; label: string; model: string; family: string }; services: Service[] };

  let data = $state<State | null>(null), error = $state(''), busy = $state('');
  let model = $state(''), resolution = $state('720p');
  let adding = $state(''), key = $state('');
  const usd = (v: number, digits = 2) => '$' + v.toFixed(digits);

  let refreshing = $state(false);
  function ago(v?: string) {
    if (!v) return '';
    const minutes = Math.round((Date.now() - new Date(v).getTime()) / 60000);
    if (minutes < 1) return 'just now';
    if (minutes < 60) return `${minutes} min ago`;
    if (minutes < 1440) return `${Math.round(minutes / 60)} h ago`;
    return new Date(v).toLocaleDateString(undefined, { day: 'numeric', month: 'short' });
  }
  async function refresh() { refreshing = true; await load(true); refreshing = false; }
  async function load(fresh = false) {
    try {
      const q = new URLSearchParams({ resolution, seconds: '10' });
      if (fresh) q.set('refresh', '1');
      if (model) q.set('model', model);
      data = await get('/video-services?' + q);
      model = data!.model; error = '';
    } catch (e) { error = (e as Error).message; }
  }
  async function route(mode: 'cheapest' | 'manual', provider = '') {
    busy = provider || mode; error = '';
    try { await put('/video-services/routing', { mode, model, provider }); await load(); }
    catch (e) { error = (e as Error).message; }
    busy = '';
  }
  async function saveKey(provider: string) {
    if (!key.trim()) return;
    busy = provider; error = '';
    try { await put('/settings/providers/' + provider, { api_key: key.trim() }); adding = ''; key = ''; await load(); }
    catch (e) { error = (e as Error).message; }
    busy = '';
  }
  // An account that said it has no balance: ask again once the owner has topped it up.
  async function checkAgain(provider: string) {
    busy = provider; error = '';
    try {
      const r = await post('/settings/providers/' + provider + '/test');
      await load();
      if (!r.ok && r.error) error = r.error;
    } catch (e) { error = (e as Error).message; }
    busy = '';
  }
  // Changing the model while "cheapest" is on must move the automatic pick too.
  async function modelChanged() { if (data?.routing === 'cheapest') await route('cheapest'); else await load(); }
  onMount(() => void load());
  const pick = $derived(data?.services.find((s) => s.cheapest));
</script>

<section class="grid gap-4" aria-labelledby="video-services">
  <div class="grid gap-1">
    <h2 id="video-services" class="text-base font-medium">Video service</h2>
    <p class="text-sm text-muted-foreground">The same video model is sold by several services at different prices. Connect the ones you have an account with and VideoFlow can use the cheapest.</p>
  </div>

  {#if !data && !error}
    <div role="status" aria-busy="true" aria-label="Loading prices" class="grid gap-2"><Skeleton class="h-9 w-72" /><Skeleton class="h-48 w-full" /></div>
  {:else if data}
    <div class="flex flex-wrap items-end gap-3">
      <label class="grid gap-1 text-xs font-medium">Video model
        <select class="h-9 rounded-md border border-input bg-background px-2 text-sm font-normal" bind:value={model} onchange={modelChanged}>
          {#each data.families as f (f.id)}<option value={f.id}>{f.label}</option>{/each}
        </select>
      </label>
      <label class="grid gap-1 text-xs font-medium">Quality
        <select class="h-9 rounded-md border border-input bg-background px-2 text-sm font-normal" bind:value={resolution} onchange={() => load()}>
          <option value="480p">480p</option><option value="720p">720p</option><option value="1080p">1080p</option>
        </select>
      </label>
      <Button variant="outline" size="sm" class="h-9" disabled={refreshing} onclick={refresh}>{#if refreshing}<LoaderCircle class="size-4 animate-spin motion-reduce:animate-none" />{/if}Check prices now</Button>
    </div>

    <label class="flex items-start gap-3 rounded-lg border border-border p-3 text-sm">
      <input type="checkbox" class="mt-0.5 size-4" checked={data.routing === 'cheapest'} disabled={!!busy}
        onchange={(e) => (e.currentTarget.checked ? route('cheapest') : route('manual'))} />
      <span class="grid gap-0.5">
        <span class="font-medium">Always use the cheapest connected service</span>
        <span class="text-muted-foreground">
          {#if data.routing === 'cheapest' && pick}Prices are compared each time a video is made. Right now that is {pick.label} at {usd(pick.per_second, 3)} per second.
          {:else if data.routing === 'cheapest'}No connected service sells this model, so videos keep using {data.current.label}.
          {:else}Off. Videos use {data.current.label}.{/if}
        </span>
      </span>
    </label>

    <Table.Root>
      <Table.Header>
        <Table.Row><Table.Head>Service</Table.Head><Table.Head class="text-right">Per second</Table.Head><Table.Head class="text-right">10-second clip</Table.Head><Table.Head>Status</Table.Head></Table.Row>
      </Table.Header>
      <Table.Body>
        {#each data.services as s (s.provider)}
          <Table.Row>
            <Table.Cell class="whitespace-normal">
              <span class="font-medium">{s.label}</span>
              {#if s.key_check?.state === 'rejected' || s.key_check?.state === 'no_credit'}<span class="block text-xs text-destructive">{s.key_check.message}</span>{/if}
              <span class="block text-xs text-muted-foreground">{#if s.problem}{s.problem}{:else if s.checked_at}Price read from {s.label} {ago(s.checked_at)}{:else}Price not read yet{/if}</span>
            </Table.Cell>
            <Table.Cell class="text-right tabular-nums">{usd(s.per_second, 3)}</Table.Cell>
            <Table.Cell class="text-right tabular-nums">{usd(s.total)}</Table.Cell>
            <Table.Cell class="whitespace-normal">
              <div class="flex flex-wrap items-center gap-2">
                {#if s.in_use}<Badge>In use</Badge>{/if}
                {#if s.key_check?.state === 'rejected'}<Badge variant="destructive">Key rejected</Badge>{/if}
                {#if s.key_check?.state === 'no_credit'}<Badge variant="destructive">No balance</Badge>
                  <Button size="sm" variant="outline" disabled={!!busy} onclick={() => checkAgain(s.provider)}>{busy === s.provider ? 'Checking…' : 'Check again'}</Button>{/if}
                {#if s.cheapest && !s.in_use}<Badge variant="outline">Cheapest connected</Badge>{/if}
                {#if s.connected && !s.in_use && !s.cheapest}<Badge variant="outline">Connected</Badge>{/if}
                {#if s.usable && !s.in_use && data.routing !== 'cheapest'}
                  <Button size="sm" variant="outline" disabled={!!busy} onclick={() => route('manual', s.provider)}>Use this</Button>
                {/if}
                {#if !s.connected && adding !== s.provider}
                  <Button size="sm" variant="ghost" onclick={() => { adding = s.provider; key = ''; }}>{s.key_check?.state === 'rejected' ? 'Paste a new key' : 'Add API key'}</Button>
                {/if}
              </div>
              {#if adding === s.provider}
                <form class="mt-2 flex flex-wrap items-center gap-2" onsubmit={(e) => { e.preventDefault(); void saveKey(s.provider); }}>
                  <Input type="password" class="h-9 w-56" bind:value={key} placeholder="API key" aria-label={`API key for ${s.label}`} autocomplete="off" />
                  <Button size="sm" type="submit" disabled={busy === s.provider || !key.trim()}>Save</Button>
                  <Button size="sm" variant="ghost" type="button" onclick={() => (adding = '')}>Cancel</Button>
                </form>
              {/if}
            </Table.Cell>
          </Table.Row>
        {/each}
      </Table.Body>
    </Table.Root>
    {#if data.services.length === 0}<p class="text-sm text-muted-foreground">No service lists a price for this model at {resolution}.</p>{/if}
    <p class="text-xs text-muted-foreground">Prices are read from each service's own price feed or pricing page whenever this page opens and again before a video is made, at most every 15 minutes. Discounts on your own account are not included.</p>
  {/if}
  {#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
</section>
