<script lang="ts">
  import { onMount } from 'svelte';
  import ConversationHistory from './ConversationHistory.svelte';
  import SessionReferences from './SessionReferences.svelte';
  import { get, post } from '$lib/api';
  import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import { Button } from '$lib/components/ui/button';
  import type { DirectorSession, DirectorStatus } from '$lib/api-types';
  let { compact = false, minimal = false }: { compact?: boolean; minimal?: boolean } = $props();
  let sessions: DirectorSession[] = $state([]),
    engines: DirectorStatus['engines'] = $state([]),
    selected = $state(''),
    engine = $state('codex'),
    title = $state(''),
    menuOpen = $state(false),
    referencesOpen = $state(false),
    busy = $state(false),
    error = $state(''),
    loaded = $state(false);
  import { put } from '$lib/api';
  let catalog = $state<Record<string, { models: string[]; efforts: string[] }>>({});
  const current = $derived(sessions.find(s => s.id === selected));
  const EFFORT: Record<string, string> = {
    low: 'Quick',
    medium: 'Balanced',
    high: 'Careful',
    xhigh: 'Very careful',
    max: 'Maximum'
  };
  async function setModel(model: string, effort: string) {
    if (!current) return;
    await run(async () => {
      await put(`/director/sessions/${current.id}/model`, { model, effort });
      await refresh();
    });
  }
  async function refresh() {
    get<Record<string, { models: string[]; efforts: string[] }>>('/director/models')
      .then(c => (catalog = c))
      .catch(() => {});
    const [list, status] = await Promise.all([
      get<DirectorSession[]>('/director/sessions'),
      get<DirectorStatus>('/director/status')
    ]);
    sessions = list;
    engines = status.engines ?? [];
    if (!engines.some(e => e.engine === engine && e.signed_in)) engine = engines.find(e => e.signed_in)?.engine ?? engine;
    selected = list.find(s => s.selected)?.id ?? '';
  }
  async function run(fn: () => Promise<void>) {
    busy = true;
    error = '';
    try {
      await fn();
      window.dispatchEvent(new Event('videoflow-session-changed'));
    } catch (e) {
      error = String(e);
    } finally {
      busy = false;
    }
  }
  const start = () =>
    run(async () => {
      await post('/director/sessions', { engine, title: title.trim() || 'Story planning' });
      title = '';
      menuOpen = false;
      await refresh();
    });
  onMount(() => {
    refresh()
      .catch(e => (error = String(e)))
      .finally(() => (loaded = true));
  });
  const available = $derived(engines.some(e => e.engine === engine && e.installed));
  const connected = $derived(engines.some(e => e.engine === engine && e.signed_in));
</script>
<div class="space-y-2" data-testid="session-selector">
  <div class="flex flex-wrap items-center gap-2">
    <DropdownMenu.Root bind:open={menuOpen}>
      <DropdownMenu.Trigger
        class="flex min-h-10 min-w-0 flex-1 items-center justify-between gap-2 rounded-lg {minimal?'border border-transparent bg-transparent px-2 font-semibold':'border border-input bg-background px-3'} text-left text-sm focus-visible:outline-2 focus-visible:outline-ring disabled:opacity-50"
        aria-label="AI session"
        title="Switch conversation"
        disabled={busy||!loaded}>
        <span
          class="truncate">{!loaded
              ? 'Loading sessions…'
              : current
                ? minimal
                  ? current.title || 'Untitled'
                  : `${current.title || 'Untitled'} · ${current.engine}`
                : 'Choose conversation'}</span><ChevronDown
          class="size-4 shrink-0" />
      </DropdownMenu.Trigger>
      <DropdownMenu.Content align="start" class="w-80 max-w-[min(420px,90vw)]">
        <DropdownMenu.RadioGroup
          value={selected}
          onValueChange={id => {
            if (id !== selected)
              void run(async () => {
                await post(`/director/sessions/${id}/select`, {});
                await refresh();
              });
          }}>
          {#each sessions as session}<DropdownMenu.RadioItem value={session.id} closeOnSelect><span
            class="min-w-0"><span class="block truncate">{session.title||'Untitled'}</span><span
            class="block text-xs text-muted-foreground">{session.engine==='claude'?'Claude Code':'Codex'} · {({ready:'Ready',running:'Working…'} as Record<string,string>)[session.status]??session.status}</span></span></DropdownMenu.RadioItem>{/each}
        </DropdownMenu.RadioGroup>
        {#if !sessions.length}<p class="px-2 py-2 text-sm text-muted-foreground">No saved conversations</p>{/if}
        {#if current}{@const options=catalog[current.engine]}
        <DropdownMenu.Separator />
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div class="grid gap-2 px-2 py-2" onkeydown={e=>e.stopPropagation()}>
          <p class="text-xs text-muted-foreground">How {current.engine==='claude'?'Claude Code':'Codex'} thinks in this conversation</p>
          <label class="grid gap-1 text-xs font-medium">Model
            <select
              class="min-h-9 rounded-md border border-input bg-background px-2 text-sm font-normal"
              value={current.model??''}
              disabled={busy}
              onchange={e=>setModel(e.currentTarget.value,current.effort??'')}>
              <option value="">Default (recommended)</option>
              {#each options?.models??[] as m}<option value={m}>{m}</option>{/each}
              {#if current.model&&!(options?.models??[]).includes(current.model)}<option value={current.model}>{current.model}</option>{/if}
            </select>
          </label>
          <label class="grid gap-1 text-xs font-medium">Thinking effort
            <select
              class="min-h-9 rounded-md border border-input bg-background px-2 text-sm font-normal"
              value={current.effort??''}
              disabled={busy}
              onchange={e=>setModel(current.model??'',e.currentTarget.value)}>
              <option value="">Default</option>
              {#each options?.efforts??[] as level}<option value={level}>{EFFORT[level]??level}</option>{/each}
            </select>
          </label>
          <p class="text-xs text-muted-foreground">More effort gives better plans but slower replies and uses more of your AI plan.</p>
        </div>{/if}
        <DropdownMenu.Separator />
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div class="grid gap-2 px-2 py-2" onkeydown={e=>e.stopPropagation()}>
          <p class="text-xs font-medium">New conversation</p>
          <div class="flex flex-wrap items-center gap-2">
            <select
              aria-label="New conversation AI"
              bind:value={engine}
              class="min-h-9 rounded-md border border-input bg-background px-2 text-sm"
              disabled={busy}>{#each [['codex','Codex'],['claude','Claude Code']] as [id,name]}<option
                value={id}>{name}{engines.some(e=>e.engine===id&&e.signed_in)?' · connected':''}</option>{/each}</select>
            <input
              aria-label="New conversation name"
              bind:value={title}
              placeholder="Story planning"
              maxlength="120"
              class="min-h-9 min-w-0 flex-1 rounded-md border border-input bg-background px-2 text-sm"
              disabled={busy} />
            <Button size="sm" disabled={busy||!available} onclick={start}>{busy?'Starting…':'Start'}</Button>
          </div>
          {#if !connected}<p
            class="text-xs text-muted-foreground">{available?'Not connected yet.':'Not available on this server.'} <a
            class="underline underline-offset-4" href="/settings#ai">Connect your AI</a></p>{/if}
        </div>
        <DropdownMenu.Separator />
        {#if current}<DropdownMenu.Item onclick={()=>referencesOpen=true}>Characters &amp; references</DropdownMenu.Item>{/if}
        <DropdownMenu.Item>{#snippet child({props})}<a {...props} href="/settings#ai">AI setup</a>{/snippet}</DropdownMenu.Item>
      </DropdownMenu.Content>
    </DropdownMenu.Root>
    <ConversationHistory
      disabled={busy||!loaded}
      oncontinue={async id => {
        await post(`/director/sessions/${encodeURIComponent(id)}/select`, {});
        await refresh();
        window.dispatchEvent(new Event('videoflow-session-changed'));
      }} />
  </div>
  {#if loaded&&!selected}
    <div class="flex flex-wrap items-center gap-2">
      <select
        aria-label="New session engine"
        bind:value={engine}
        class="min-h-10 rounded-lg border border-input bg-background px-2 text-sm"
        disabled={busy}><option value="codex">Codex</option><option value="claude">Claude Code</option></select>
      <Button size="sm" disabled={busy||!available} onclick={start}>{busy?'Creating…':'Start session'}</Button>
    </div>
    <p class="text-xs text-muted-foreground">{connected?'Connected and ready.':available?'Not connected yet.':'Not available on this server.'} {#if !connected}<a
      class="underline underline-offset-4"
      href="/settings#ai">Connect your AI</a>{/if}</p>
  {/if}
  {#if current}<SessionReferences bind:open={referencesOpen} session={current} />{/if}
  {#if error}<p role="alert" class="text-xs text-destructive">{error} <button class="underline" onclick={()=>run(refresh)}>Retry</button></p>{/if}
</div>
