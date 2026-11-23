<script lang="ts">
  import { errorMessage } from '$lib/errors';
  let { embedded = false, oncounts }: { embedded?: boolean; oncounts?: (characters: number, media: number) => void } =
    $props();
  import LibraryPreview from './LibraryPreview.svelte';
  import { characterMatches } from './catalog.mjs';
  import { protectDraft } from '$lib/protect-draft';
  import { goto } from '$app/navigation';
  import { onDestroy, onMount } from 'svelte';
  import { runBackgroundOp } from '$lib/ops';
  import { get, post, del, isImage } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import { Card, CardContent } from '$lib/components/ui/card';
  import { Skeleton } from '$lib/components/ui/skeleton';
  import { Textarea } from '$lib/components/ui/textarea';
  import * as Dialog from '$lib/components/ui/dialog';
  import { toast } from 'svelte-sonner';
  import { Images, LoaderCircle, Sparkles, Trash2, X } from '@lucide/svelte';
  import type { Asset, Character, CharacterSheetPlan } from '$lib/api-types';

  let search = $state(''),
    referenceFilter = $state(''),
    sortBy = $state('name');

  let characters: Character[] = $state([]);
  let assets: Record<string, Asset> = $state({});
  let error = $state('');
  let busy = $state('');
  let loaded = $state(false);
  let loadError = $state('');
  let refreshing = $state(false);
  async function retryCharacters() {
    refreshing = true;
    try {
      await refresh();
      loadError = '';
    } catch (e) {
      loadError = errorMessage(e);
    } finally {
      refreshing = false;
    }
  }

  let createOpen = $state(false);
  function changeCreateOpen(value: boolean) {
    if (
      !value &&
      (busy === 'create' || ((name.trim() || description.trim()) && !window.confirm('Discard this character draft?')))
    )
      return;
    if (value && !createOpen) error = '';
    createOpen = value;
    if (!value) {
      name = '';
      description = '';
    }
  }
  protectDraft(() => createOpen && !!(name.trim() || description.trim()), 'Discard this character draft and leave?');
  let name = $state('');
  let description = $state('');

  export function hasDraft() {
    return !!(createOpen && (name.trim() || description.trim()));
  }
  const filteredCharacters = $derived(
    characters
      .filter(
        c =>
          characterMatches(c, search) &&
          (!referenceFilter ||
            (referenceFilter === 'with'
              ? (c.reference_asset_ids_json ?? []).some((id: string) => assets[id] && isImage(assets[id].file_path))
              : !(c.reference_asset_ids_json ?? []).some((id: string) => assets[id] && isImage(assets[id].file_path))))
      )
      .slice()
      .sort((a, b) =>
        sortBy === 'newest'
          ? (b.created_at ?? '').localeCompare(a.created_at ?? '')
          : (a.name ?? '').localeCompare(b.name ?? '')
      )
  );

  // --- pictures for every character without one, made side by side in one go ---
  const sheetScope = new AbortController();
  onDestroy(() => sheetScope.abort());
  let sheetPlan = $state<CharacterSheetPlan | null>(null);
  let sheetPlanError = $state('');
  let drawing = $state(false);
  let sheetNote = $state('');
  async function loadSheetPlan() {
    try {
      sheetPlan = await get<CharacterSheetPlan>('/characters/reference-sheets');
      sheetPlanError = '';
    } catch (e) {
      sheetPlan = null;
      sheetPlanError = errorMessage(e);
    }
  }
  const sheetReason = $derived(
    drawing
      ? ''
      : sheetPlanError
        ? `Can't check the picture service: ${sheetPlanError}`
        : sheetPlan && !sheetPlan.ready
          ? (sheetPlan.problem ?? '')
          : ''
  );
  async function drawAll() {
    const plan = sheetPlan;
    if (!plan?.ready || drawing || busy) return;
    drawing = true;
    sheetNote = '';
    const finish = async (text: string) => {
      if (sheetScope.signal.aborted) return;
      drawing = false;
      sheetNote = text;
      await retryCharacters();
    };
    try {
      await runBackgroundOp(
        '/characters/reference-sheets',
        { character_ids: plan.characters.map(c => c.character_id), expected_image_calls: plan.image_calls },
        {
          signal: sheetScope.signal,
          label: 'Character pictures',
          onDone: op => {
            const made = op.result_json?.made ?? [],
              failed = op.result_json?.failed ?? [];
            return finish(
              [
                made.length ? `Made pictures for ${made.map(c => c.name).join(', ')}.` : '',
                failed.length ? `Couldn't make ${failed.map(c => `${c.name} (${c.error})`).join(', ')}. Try again to redo only those.` : ''
              ]
                .filter(Boolean)
                .join(' ')
            );
          },
          onFail: op => finish(op.error ?? 'The pictures could not be made. Check Activity before trying again.')
        }
      );
    } catch (e) {
      await finish(errorMessage(e));
    }
  }

  // --- delete confirm ---
  let deleteTarget = $state<Character | null>(null);

  let loadSequence = 0;
  export async function reload() {
    await retryCharacters();
  }
  async function refresh() {
    const sequence = ++loadSequence;
    try {
      const [nextCharacters, all] = await Promise.all([get<Character[]>('/characters'), get<Asset[]>('/assets')]);
      if (sequence !== loadSequence) return;
      characters = nextCharacters;
      assets = Object.fromEntries(all.map(a => [a.id, a]));
      oncounts?.(characters.length, all.length);
      void loadSheetPlan();
    } catch (e) {
      if (sequence === loadSequence) throw e;
    }
  }
  onMount(() =>
    refresh()
      .then(() => {
        const id = new URL(window.location.href).searchParams.get('character_id');
        if (id) void goto(`/characters/${encodeURIComponent(id)}`, { replaceState: true });
      })
      .catch(e => (loadError = e.message))
      .finally(() => (loaded = true))
  );

  async function run(key: string, fn: () => Promise<unknown>) {
    busy = key;
    error = '';
    try {
      await fn();
      await refresh();
    } catch (e) {
      error = errorMessage(e);
      if (key !== 'create' || !createOpen) toast.error(errorMessage(e));
    } finally {
      busy = '';
    }
  }

  const createCharacter = (e: Event) => {
    e.preventDefault();
    if (!name.trim() || busy) return;
    run('create', async () => {
      await post('/characters', { name: name.trim(), description: description.trim() });
      createOpen = false;
      search = '';
      referenceFilter = '';
      toast.success('Character created');
      name = '';
      description = '';
    });
  };

  // ---- delete ----
  async function confirmDelete() {
    const c = deleteTarget;
    if (!c) return;
    busy = `delete-${c.id}`;
    try {
      await del(`/characters/${c.id}`);
      deleteTarget = null;
      toast.success('Character deleted; reference images kept.');
      await refresh();
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      busy = '';
    }
  }
</script>

<svelte:head>{#if !embedded}<title>Characters · VideoFlow</title>{/if}</svelte:head>



<div class="px-4 py-6 sm:px-8">
  <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
    <div>{#if embedded}<h2 class="text-lg font-semibold">Characters</h2>{:else}<h1
      class="text-2xl font-semibold">Characters</h1>{/if}<p
      class="mt-1 text-sm text-muted-foreground">{loadError
          ? 'Collection unavailable'
          : loaded
            ? `${characters.length} ${characters.length === 1 ? 'character' : 'characters'}`
            : 'Loading collection…'}</p></div>
    <div class="flex flex-wrap items-center gap-2"><Button
      variant="outline"
      disabled={!loaded||!!loadError||!sheetPlan?.ready||drawing||!!busy}
      title={sheetReason||undefined}
      onclick={drawAll}>{#if drawing}<LoaderCircle class="size-4 motion-safe:animate-spin"/>{:else}<Sparkles
        class="size-4"/>{/if}{drawing?'Making pictures…':'Make pictures for all characters'}</Button>
    <Dialog.Root open={createOpen} onOpenChange={changeCreateOpen}>
      <Dialog.Trigger>{#snippet child({props})}<Button {...props} disabled={!loaded||!!loadError}>Create character</Button>{/snippet}</Dialog.Trigger>
      <Dialog.Content
        class="max-h-[90dvh] overflow-y-auto sm:max-w-md"
        showCloseButton={false}
        overlayClass="bg-black/55 supports-backdrop-filter:backdrop-blur-none"
        onEscapeKeydown={e=>{e.preventDefault();changeCreateOpen(false);}}
        onInteractOutside={e=>{e.preventDefault();changeCreateOpen(false);}}>
<Dialog.Header><Dialog.Title>Create character</Dialog.Title><Dialog.Description
  class="sr-only">Add a named character to this project.</Dialog.Description></Dialog.Header>
  <form class="space-y-4" onsubmit={createCharacter}>
    {#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
    <div class="grid gap-4">
      <div class="flex-1 min-w-[160px]">
        <label class="block text-xs text-muted-foreground mb-1" for="name">Name</label>
        <input id="name" bind:value={name} required disabled={busy==='create'}
          class="w-full rounded-md border border-input bg-background px-2 py-1.5 text-sm" />
      </div>
      <div class="flex-1 min-w-[160px]">
        <label class="block text-xs text-muted-foreground mb-1" for="desc">Description (optional)</label>
        <Textarea id="desc" rows={3} bind:value={description} disabled={busy==='create'} />
      </div>
      <div class="flex justify-end gap-2">
        <Button type="button" variant="outline" disabled={busy==='create'} onclick={()=>changeCreateOpen(false)}>Cancel</Button>
        <Button type="submit" disabled={!!busy || !name.trim()} size="sm">
          {busy==='create'?'Creating…':'Create character'}
        </Button>
      </div>
    </div>
  </form>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          class="absolute right-2 top-2"
          aria-label="Close"
          disabled={busy==='create'}
          onclick={()=>changeCreateOpen(false)}><X class="size-4" /></Button>
      </Dialog.Content>
    </Dialog.Root></div>
  </div>
  {#if loaded&&!loadError&&characters.length}<div class="-mt-2 mb-4 space-y-1 text-xs text-muted-foreground" aria-live="polite">
    {#if sheetReason}<p>{sheetReason}</p>{:else if sheetPlan?.ready}<p>Makes {sheetPlan.characters.length===1?'a reference picture':'reference pictures'} for {sheetPlan.characters.map(c=>c.name).join(', ')}, side by side: {sheetPlan.image_calls} picture {sheetPlan.image_calls===1?'call':'calls'} (one each, with the face close-up and full body in one picture). Uses image credits.</p>{/if}
    {#if sheetNote}<p role="status" class="text-foreground">{sheetNote}</p>{/if}
  </div>{/if}

  <div
    class="mb-6 flex flex-wrap gap-3"
    aria-label="Character filters"><input
    type="search"
    aria-label="Search characters"
    placeholder="Search characters…"
    bind:value={search}
    class="min-h-11 min-w-0 flex-1 basis-60 rounded-lg border border-input bg-background px-3 text-sm"/><select
    aria-label="Character references"
    bind:value={referenceFilter}
    class="min-h-11 max-w-full rounded-lg border border-input bg-background px-3 text-sm"><option
      value="">All characters</option><option value="with">With reference images</option><option value="missing">Needs reference images</option></select><select
    aria-label="Sort characters"
    bind:value={sortBy}
    class="min-h-11 rounded-lg border border-input bg-background px-3 text-sm"><option
      value="name">Name A–Z</option><option value="newest">Newest first</option></select></div>
  {#if loaded&&characters.length&&!filteredCharacters.length}<div role="status" class="py-16 text-center"><p
    class="font-medium">No matching characters</p><p class="mt-2 text-sm text-muted-foreground">Try another name or clear the filters.</p><Button
    variant="outline"
    class="mt-4"
    onclick={()=>{search='';referenceFilter='';}}>Clear filters</Button></div>{/if}
  {#if loadError}<div
    role="alert"
    class="mb-4 space-y-2 rounded-lg border border-destructive p-4 text-sm"><p>Could not load characters: {loadError}</p>{#if characters.length}<p>Showing the last loaded characters.</p>{/if}<Button
      variant="outline"
      disabled={refreshing}
      onclick={retryCharacters}>{refreshing ? 'Loading…' : 'Try again'}</Button></div>{/if}

  {#if loaded&&!loadError&&!characters.length}<div class="py-16 text-center"><Images
    class="mx-auto size-9 text-muted-foreground"/><h3
    class="mt-4 font-semibold">Add your first character</h3><p
    class="mt-2 text-sm text-muted-foreground">Create a character, then add reference images to keep their appearance consistent.</p></div>{/if}

  {#if !loaded}
    <div class="grid grid-cols-1 min-[480px]:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
      {#each Array(3) as _, i (i)}
        <div class="rounded-lg border border-border p-3 space-y-2">
          <Skeleton class="w-full aspect-[4/3] rounded-md" />
          <Skeleton class="h-4 w-24" />
          <Skeleton class="h-3 w-40" />
        </div>
      {/each}
    </div>
  {/if}

  <div class="grid grid-cols-1 min-[480px]:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
    {#each filteredCharacters as c (c.id)}
      {@const refs=(c.reference_asset_ids_json??[]).map(id=>assets[id]).filter(a=>a&&isImage(a.file_path))}
      <Card class="self-start py-0">
        <CardContent class="p-0">
          <a
            href={`/characters/${encodeURIComponent(c.id)}`}
            class="block focus-visible:outline-2 focus-visible:outline-ring"
            aria-label={'Open '+c.name}><LibraryPreview
            portrait
            asset={refs[0]??null}
            class="aspect-[4/3]"/></a>
          <div class="p-4">
            <h3 class="truncate text-base font-semibold"><a href={`/characters/${encodeURIComponent(c.id)}`} class="hover:underline">{c.name}</a></h3>
            <p class="mt-1 line-clamp-2 min-h-8 text-xs leading-relaxed text-muted-foreground">{c.description||c.appearance||'Add a description to define this character.'}</p>
            <p class="mt-3 text-xs text-muted-foreground">{refs.length?`${refs.length} reference ${refs.length===1?'image':'images'}`:'Needs reference images'}</p>
            <div class="mt-4 flex items-center justify-between gap-2"><Button
              variant="outline"
              size="sm"
              href={`/characters/${encodeURIComponent(c.id)}`}>Details</Button><Button
              variant="ghost"
              size="sm"
              class="text-destructive hover:text-destructive"
              disabled={!!busy}
              onclick={() => (deleteTarget = c)}><Trash2 class="size-3"/>Delete</Button></div>
          </div>
        </CardContent>
      </Card>
    {/each}
  </div>
</div>

<!-- Delete confirm dialog (app confirm pattern) -->
<Dialog.Root open={deleteTarget !== null} onOpenChange={(open) => !open && !busy && (deleteTarget = null)}>
  <Dialog.Content onEscapeKeydown={e=>{if(busy)e.preventDefault();}} onInteractOutside={e=>{if(busy)e.preventDefault();}}>
    <Dialog.Header>
      <Dialog.Title>Delete character?</Dialog.Title>
      <Dialog.Description>
        "{deleteTarget?.name}" will be removed and detached from every scene cast.
        Its reference images stay in the asset library.
      </Dialog.Description>
    </Dialog.Header>
    <Dialog.Footer>
      <Button variant="outline" size="sm" disabled={!!busy} onclick={() => (deleteTarget = null)}>Cancel</Button>
      <Button variant="destructive" size="sm" disabled={!!busy} onclick={confirmDelete}>
        <Trash2 class="size-3 mr-1" />{busy === `delete-${deleteTarget?.id}` ? 'Deleting…' : 'Delete'}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
