<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { beforeNavigate } from '$app/navigation';
  import { get, post, put } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import { ArrowRight, Check, ChevronDown, Plus, RefreshCw } from '@lucide/svelte';
  import { sessionState } from '$lib/director/session-state.mjs';

  type Session = { id: string; engine: string; native_id?: string; title: string; status: string; selected: boolean };
  type Turn = { id: string; kind: string; status: string; request_json: unknown; result_json: unknown; error?: string; created_at: string };
  let status: any = $state(null);
  let sessions: Session[] = $state([]), turns: Turn[] = $state([]);
  let characters: any[] = $state([]), assets: any[] = $state([]);
  let characterIDs: string[] = $state([]), assetIDs: string[] = $state([]);
  let savedCharacters: string[] = $state([]), savedAssets: string[] = $state([]);
  let engine = $state('codex'), title = $state('');
  let busy = $state(false), loaded = $state(false), error = $state(''), notice = $state('');
  let showCreate = $state(false), setupOpen = $state(false), referencesOpen = $state(false), historyOpen = $state(false);
  let warnings: string[] = $state([]);
  let detailReady = $state(false);
  let titleInput: HTMLInputElement | undefined = $state();
  let setupSummary: HTMLElement | undefined = $state(), historySummary: HTMLElement | undefined = $state();

  const selected = $derived(sessions.find(s => s.selected));
  const selectedState = $derived(sessionState(selected, status));
  const engineName = (value: string) => value === 'claude' ? 'Claude Code' : 'Codex';
  const selectionKey = (cast: string[], media: string[]) => JSON.stringify([[...cast].sort(), [...media].sort()]);
  const dirty = $derived(selectionKey(characterIDs, assetIDs) !== selectionKey(savedCharacters, savedAssets));
  const selectedNames = $derived([
    ...characters.filter(c => characterIDs.includes(c.id)).map(c => c.name || c.id),
    ...assets.filter(a => assetIDs.includes(a.id)).map(a => a.name || a.id)
  ]);
  const json = (value: unknown) => typeof value === 'string' ? value : JSON.stringify(value, null, 2);
  const date = (value: string) => new Date(value).toLocaleString();

  async function detail(id: string) {
    detailReady = false;
    const [history, selection] = await Promise.all([get(`/director/sessions/${id}/turns`), get(`/director/sessions/${id}/selection`)]);
    turns = history ?? []; characterIDs = selection?.character_ids ?? []; assetIDs = selection?.asset_ids ?? [];
    savedCharacters = [...characterIDs]; savedAssets = [...assetIDs]; warnings = []; detailReady = true;
  }
  async function refresh() {
    const [health, list, cast, media] = await Promise.all([get('/director/status'), get('/director/sessions'), get('/characters'), get('/assets')]);
    status = health; sessions = list ?? []; characters = cast ?? []; assets = media ?? [];
    if (!loaded) engine = health.engines?.find((item: any) => item.installed)?.engine ?? 'codex';
    const current = sessions.find(s => s.selected);
    if (current) await detail(current.id);
    else { turns = []; characterIDs = []; assetIDs = []; savedCharacters = []; savedAssets = []; detailReady = false; }
  }
  async function run(fn: () => Promise<void>) {
    if (busy) return;
    busy = true; error = ''; notice = '';
    try { await fn(); } catch (e: any) { error = e.message ?? String(e); } finally { busy = false; }
  }
  onMount(() => { void run(refresh).finally(() => { loaded = true; showCreate = sessions.length === 0; }); });
  beforeNavigate(nav => {
    if (dirty && !window.confirm('Leave without saving your reference changes?')) nav.cancel();
  });
  async function newSession() { showCreate = true; await tick(); titleInput?.focus(); }
  async function create(event: SubmitEvent) {
    event.preventDefault(); if (dirty) return;
    await run(async () => {
      await post('/director/sessions', { engine, title: title.trim() || 'Story planning' });
      title = ''; await refresh(); showCreate = false; referencesOpen = false; historyOpen = false; notice = 'Session saved.';
    });
  }
  async function choose(id: string) {
    if (!id || dirty) return;
    await run(async () => {
      await post(`/director/sessions/${id}/select`, {}); await refresh();
      showCreate = false; referencesOpen = false; historyOpen = false;
    });
  }
  async function saveContext() {
    if (!selected || !detailReady) return;
    const id = selected.id;
    await run(async () => {
      const snapshot = await put(`/director/sessions/${id}/selection`, { character_ids: characterIDs, asset_ids: assetIDs });
      savedCharacters = [...characterIDs]; savedAssets = [...assetIDs];
      warnings = snapshot.warnings ?? []; notice = 'References saved for your next prompt.';
    });
  }
  function discardReferences() { characterIDs = [...savedCharacters]; assetIDs = [...savedAssets]; warnings = []; }
  async function showSetup() { setupOpen = true; await tick(); setupSummary?.focus(); setupSummary?.scrollIntoView({ block: 'nearest' }); }
  async function showHistory() { historyOpen = true; await tick(); historySummary?.focus(); historySummary?.scrollIntoView({ block: 'nearest' }); }
  async function cancel() {
    if (!selected) return;
    const id = selected.id;
    await run(async () => { await post(`/director/sessions/${id}/cancel`, {}); await refresh(); notice = 'Stop requested. Refresh to check the final status.'; });
  }
  async function acknowledge() {
    if (!selected || dirty) return;
    const id = selected.id;
    await run(async () => { await post(`/director/sessions/${id}/acknowledge`, {}); await refresh(); notice = 'Review saved. You can continue this session.'; });
  }
</script>

<svelte:head><title>AI sessions · VideoFlow</title></svelte:head>

<div class="mx-auto w-full max-w-4xl px-4 py-6 sm:px-8 sm:py-8">
  <header class="mb-8 flex items-start justify-between gap-4">
    <div><h1 class="text-2xl font-semibold tracking-tight">AI sessions</h1><p class="mt-1 text-sm text-muted-foreground">Choose who helps plan your story.</p></div>
    <Button variant="ghost" class="min-h-11 shrink-0" disabled={busy || dirty} onclick={() => run(refresh)} aria-label="Refresh sessions"><RefreshCw class="size-4" /><span class="hidden sm:inline">Refresh</span></Button>
  </header>
  {#if error}<p role="alert" class="mb-5 rounded-lg border border-destructive p-3 text-sm text-destructive">{error}</p>{/if}
  {#if notice}<p role="status" class="mb-5 flex items-center gap-2 text-sm"><Check class="size-4 shrink-0 text-muted-foreground" />{notice}</p>{/if}
  {#if !loaded}
    <p role="status" class="py-8 text-sm text-muted-foreground">Loading your sessions…</p>
  {:else}
    <section aria-label="Your sessions" class="mb-8">
      {#if sessions.length}
        <div class="flex flex-wrap items-end gap-3">
          <label class="grid min-w-0 flex-1 gap-2 text-sm font-medium" for="project-session">Your sessions
            <select id="project-session" class="min-h-11 w-full min-w-0 rounded-lg border border-input bg-background px-3 text-sm font-normal" value={selected?.id ?? ''} disabled={busy || dirty} onchange={e => choose(e.currentTarget.value)}>
              <option value="" disabled>Choose a session</option>
              {#each sessions as session (session.id)}<option value={session.id}>{session.title || 'Untitled session'} · {engineName(session.engine)}</option>{/each}
            </select>
          </label>
          <Button variant="outline" class="min-h-11" disabled={busy || dirty} onclick={newSession} aria-expanded={showCreate} aria-controls="new-session-form"><Plus class="size-4" />New session</Button>
        </div>
      {/if}
      {#if showCreate}
        <form id="new-session-form" class="mt-5 space-y-4 rounded-lg bg-muted/30 p-4 sm:p-5" onsubmit={create}>
          <div><h2 class="font-medium">{sessions.length ? 'New session' : 'Start your first session'}</h2>{#if !sessions.length}<p class="mt-1 text-sm text-muted-foreground">Keep each story’s planning and references together.</p>{/if}</div>
          <div class="grid gap-4 sm:grid-cols-[minmax(140px,1fr)_minmax(0,2fr)]">
            <label class="grid gap-2 text-sm" for="session-engine">AI assistant<select id="session-engine" class="min-h-11 w-full rounded-lg border border-input bg-background px-3" bind:value={engine} disabled={busy}><option value="codex">Codex</option><option value="claude">Claude Code</option></select></label>
            <label class="grid gap-2 text-sm" for="session-title">Session name <span class="sr-only">(optional)</span><input id="session-title" bind:this={titleInput} class="min-h-11 w-full min-w-0 rounded-lg border border-input bg-background px-3" bind:value={title} placeholder="Story planning" maxlength="120" disabled={busy} /></label>
          </div>
          {#if !status?.engines?.some((item: any) => item.engine === engine && item.installed)}<p class="text-sm text-muted-foreground">{engineName(engine)} needs setup. You can save a session now and set it up next.</p>{/if}
          <div class="flex flex-wrap gap-2"><Button type="submit" class="min-h-11" disabled={busy || dirty}>{busy ? 'Saving…' : 'Create session'}</Button>{#if sessions.length}<Button variant="ghost" class="min-h-11" disabled={busy} onclick={() => showCreate = false}>Cancel</Button>{/if}</div>
        </form>
      {/if}
    </section>

    {#if selected}
      <section aria-label="Selected session" class="mb-8 flex flex-wrap items-start justify-between gap-5">
        <div class="min-w-0 flex-1 basis-64">
          <h2 class="break-words text-xl font-semibold">{selected.title || 'Untitled session'}</h2>
          <p class="mt-1 text-sm text-muted-foreground">{engineName(selected.engine)}<span aria-hidden="true" class="mx-1">·</span><span class="font-medium text-foreground">{selectedState.label}</span></p>
          <p class="mt-3 max-w-prose text-sm text-muted-foreground">{selectedState.help}</p>
        </div>
        {#if !showCreate}
          {#if selected.status === 'running'}<Button variant="outline" class="min-h-11" disabled={busy} onclick={cancel}>Stop request</Button>
          {:else if selected.status === 'review_required'}<Button class="min-h-11" onclick={showHistory}>Review history<ArrowRight class="size-4" /></Button>
          {:else if selectedState.needsSetup}<Button class="min-h-11" onclick={showSetup}>Set up AI<ArrowRight class="size-4" /></Button>
          {:else if selectedState.canContinue}<Button href="/?view=studio" class="min-h-11" disabled={busy || dirty || !detailReady || !!error}>Continue in Studio<ArrowRight class="size-4" /></Button>{/if}
        {/if}
      </section>
      {#if dirty}<p role="status" class="mb-4 text-sm">You have unsaved reference changes. Save or discard them before switching sessions or continuing.</p>{/if}
    {/if}

    <div class="divide-y divide-border border-y border-border">
      <details id="ai-setup" bind:open={setupOpen} class="group">
        <summary bind:this={setupSummary} class="flex min-h-16 cursor-pointer list-none items-center justify-between gap-3 py-4 text-sm font-medium">AI setup<ChevronDown class="size-4 shrink-0 transition-transform group-open:rotate-180" /></summary>
        <div class="space-y-4 pb-6 text-sm">
          <ul class="divide-y divide-border">{#each status?.engines ?? [] as item (item.engine)}<li class="flex items-center justify-between gap-4 py-3"><span>{engineName(item.engine)}</span><span class="text-muted-foreground">{item.installed ? 'Detected' : 'Not installed'}</span></li>{/each}</ul>
          <p class="max-w-prose text-muted-foreground">Install Codex or Claude Code on the computer running VideoFlow’s backend, then sign in there. Use Refresh above to check again.</p>
          <p class="max-w-prose text-muted-foreground">Using Docker? A CLI installed on your computer is not available inside the container. Use a native backend or configure a host integration.</p>
          <p class="max-w-prose text-muted-foreground">Detection only checks installation. Sign-in and account access are checked on your first prompt.</p>
          {#if status?.all_text_local === false}<p role="alert" class="text-destructive">Local planning is disabled in the backend configuration.</p>{/if}
        </div>
      </details>
      {#if selected && detailReady}
        <details bind:open={referencesOpen} class="group">
          <summary class="flex min-h-16 cursor-pointer list-none items-center justify-between gap-3 py-4">
            <span class="min-w-0"><span class="block text-sm font-medium">Characters &amp; references{dirty ? ' · Unsaved changes' : ''}</span><span class="mt-1 block break-words text-xs text-muted-foreground">{characterIDs.length} {characterIDs.length === 1 ? 'character' : 'characters'} · {assetIDs.length} {assetIDs.length === 1 ? 'reference' : 'references'}{#if selectedNames.length}{' · '}{selectedNames.slice(0, 3).join(', ')}{selectedNames.length > 3 ? '…' : ''}{/if}</span></span>
            <ChevronDown class="size-4 shrink-0 transition-transform group-open:rotate-180" />
          </summary>
          <div class="space-y-5 pb-6">
            <p class="text-sm text-muted-foreground">Choose what this session uses in future prompts.</p>
            <div class="grid gap-6 sm:grid-cols-2">
              <fieldset class="min-w-0 space-y-1" disabled={busy || selected.status === 'running'}><legend class="mb-2 text-sm font-medium">Characters</legend>
                {#if !characters.length}<p class="text-sm text-muted-foreground">No characters yet. <a href="/characters" class="underline underline-offset-4">Add a character</a></p>{/if}
                {#each characters as character (character.id)}<label class="flex min-h-11 cursor-pointer items-center gap-3 text-sm"><input type="checkbox" bind:group={characterIDs} value={character.id} /><span class="min-w-0 break-words">{character.name || character.id}</span></label>{/each}
              </fieldset>
              <fieldset class="min-w-0 space-y-1" disabled={busy || selected.status === 'running'}><legend class="mb-2 text-sm font-medium">Reference assets</legend>
                {#if !assets.length}<p class="text-sm text-muted-foreground">No assets yet. <a href="/assets" class="underline underline-offset-4">Upload references</a></p>{/if}
                {#each assets as asset (asset.id)}<label class="flex min-h-11 cursor-pointer items-center gap-3 text-sm"><input type="checkbox" bind:group={assetIDs} value={asset.id} /><span class="min-w-0 break-words">{asset.name || asset.id}</span></label>{/each}
              </fieldset>
            </div>
            {#if selected.status === 'running'}<p class="text-sm text-muted-foreground">References can be changed after the current request finishes.</p>{/if}
            <div class="flex flex-wrap gap-2"><Button class="min-h-11" disabled={busy || !dirty || selected.status === 'running'} onclick={saveContext}>Save references</Button>{#if dirty}<Button variant="ghost" class="min-h-11" disabled={busy} onclick={discardReferences}>Discard changes</Button>{/if}</div>
            {#each warnings as warning}<p class="text-sm text-muted-foreground">{warning}</p>{/each}
          </div>
        </details>
        <details bind:open={historyOpen} class="group">
          <summary bind:this={historySummary} class="flex min-h-16 cursor-pointer list-none items-center justify-between gap-3 py-4 text-sm font-medium"><span>History <span class="ml-2 font-normal text-muted-foreground">{turns.length} {turns.length === 1 ? 'request' : 'requests'}</span></span><ChevronDown class="size-4 shrink-0 transition-transform group-open:rotate-180" /></summary>
          <div class="space-y-4 pb-6">
            {#if !turns.length}<p class="text-sm text-muted-foreground">Your planning and writing requests will appear here.</p>{/if}
            {#each turns as turn (turn.id)}
              <details class="rounded-lg border border-border px-3"><summary class="min-h-11 cursor-pointer break-words py-3 text-sm">{turn.kind.replaceAll('_', ' ')} · {turn.status} <span class="text-muted-foreground">· {date(turn.created_at)}</span></summary>
                <div class="space-y-3 pb-4 text-sm">{#if turn.error}<p role="alert" class="break-words text-destructive">{turn.error}</p>{/if}<h3 class="font-medium">Request &amp; references</h3><pre class="max-h-72 overflow-auto whitespace-pre-wrap break-words rounded bg-muted p-3 text-xs">{json(turn.request_json)}</pre><h3 class="font-medium">Result</h3><pre class="max-h-72 overflow-auto whitespace-pre-wrap break-words rounded bg-muted p-3 text-xs">{json(turn.result_json)}</pre></div>
              </details>
            {/each}
            {#if selected.status === 'review_required'}<div class="space-y-3 pt-2 text-sm"><p>Review the interrupted request above before continuing. It may have left partial work in the AI conversation.</p>{#if dirty}<p class="text-muted-foreground">Save or discard your reference changes first.</p>{/if}<Button class="min-h-11 max-w-full whitespace-normal text-left" disabled={busy || dirty || !turns.length} onclick={acknowledge}>I reviewed this request — continue</Button></div>{/if}
          </div>
        </details>
      {/if}
      <details class="group">
        <summary class="flex min-h-16 cursor-pointer list-none items-center justify-between gap-3 py-4 text-sm font-medium">Technical details<ChevronDown class="size-4 shrink-0 transition-transform group-open:rotate-180" /></summary>
        <div class="space-y-3 pb-6 text-sm text-muted-foreground">{#if selected}<p class="break-all">Conversation ID: {selected.native_id || 'Assigned after your first prompt.'}</p>{/if}<p>{status?.host_requirement}</p>{#each status?.engines ?? [] as item (item.engine)}{#if item.reason}<p class="break-words">{engineName(item.engine)}: {item.reason}</p>{/if}{/each}</div>
      </details>
    </div>
  {/if}
</div>

<style>
  summary::-webkit-details-marker { display: none; }
  summary:focus-visible { outline: 2px solid var(--ring); outline-offset: 4px; border-radius: 4px; }
  @media (prefers-reduced-motion: reduce) { summary :global(svg) { transition: none; } }
</style>
