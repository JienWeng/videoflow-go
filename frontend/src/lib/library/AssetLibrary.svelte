<script lang="ts">
  import { errorMessage } from '$lib/errors';
  import StyleReference from '$lib/brief/StyleReference.svelte';
  let { embedded = false, oncounts }: { embedded?: boolean; oncounts?: (characters: number, media: number) => void } =
    $props();
  import { nodeTools } from '$lib/canvas/tools.mjs';
  import LibraryPreview from './LibraryPreview.svelte';
  import AssetDescription from '$lib/assets/AssetDescription.svelte';
  import AssetDescriptionEditor from '$lib/assets/AssetDescriptionEditor.svelte';
  import { parseDescription } from '$lib/assets/description.mjs';
  import { mediaKind, typeLabel, assetReadiness, latestAssetVersions } from './catalog.mjs';
  import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
  import { Ellipsis, LayoutGrid, List } from '@lucide/svelte';
  let readinessFilter = $state(''),
    latestOnly = $state(false);
  let kindFilter = $state(''),
    view = $state<'grid' | 'list'>('grid'),
    refreshing = $state(false);
  async function retryMedia() {
    refreshing = true;
    try {
      await refresh();
      error = '';
    } catch (e) {
      error = String(e);
    } finally {
      refreshing = false;
    }
  }
  let identityTraits: { name: string; value: string }[] = $state([]);
  let identityBusy = $state(false);
  let assetNameDraft = $state(''),
    assetDescriptionDraft = $state('');
  function assetDetailsDirty() {
    return (
      !!detail &&
      (assetNameDraft !== detail.name ||
        assetDescriptionDraft !== (detail.description ?? '') ||
        JSON.stringify(identityTraits) !==
          JSON.stringify(
            Object.entries(detail.metadata_json?.locked_traits ?? {}).map(([name, value]) => ({
              name,
              value: String(value)
            }))
          ))
    );
  }
  let reviewBusy = $state(false);
  let reviewScene = $state('');
  let regenerateTarget: any = $state(null),
    regenerating = $state('');
  const versionGroup = (asset: any) => asset.metadata_json?.variant_group_id ?? asset.id;
  async function regenerateAsset() {
    const source = regenerateTarget;
    if (!source || regenerating) return;
    regenerateTarget = null;
    regenerating = source.id;
    try {
      await runBackgroundOp(
        `/assets/${source.id}/regenerate`,
        { expected_updated_at: source.updated_at },
        {
          label: 'Asset version',
          onDone: async op => {
            if (!usageAlive) return;
            regenerating = '';
            await refresh();
            if (!usageAlive) return;
            const next = assets.find(a => op.result_json?.asset_ids?.includes(a.id));
            if (next && detail?.id === source.id) openDetail(next);
            toast.success('New version saved and ready to use.');
          },
          onFail: () => {
            if (usageAlive) regenerating = '';
          }
        }
      );
    } catch (e) {
      if (usageAlive) {
        regenerating = '';
        toast.error(String(e));
      }
    }
  }
  function requestRegeneration(asset: any) {
    if (detail && assetDetailsDirty()) {
      toast.error('Save asset details before generating a version.');
      return;
    }
    regenerateTarget = asset;
  }
  let libraryScenes: any[] = $state([]),
    usage: Record<string, any> = $state({}),
    usageError: Record<string, string> = $state({});
  let usageAlive = true;
  onDestroy(() => (usageAlive = false));
  async function loadUsage(id: string) {
    try {
      const result = await get(`/assets/${id}/usage`);
      if (!usageAlive) return;
      if (!Array.isArray(result?.uses)) throw Error('Usage information is unavailable. Reload before deleting.');
      usage[id] = result;
      usageError[id] = '';
    } catch (e) {
      if (usageAlive) usageError[id] = String(e);
    }
  }
  function prepareDelete(asset: any) {
    deleteTarget = asset;
    void loadUsage(asset.id);
  }
  function clearSceneFilter() {
    reviewScene = '';
    const url = new URL(page.url);
    url.searchParams.delete('scene_id');
    replaceState(url, { ...page.state });
  }

  async function reviewReference(approved: boolean) {
    if (!detail) return;
    reviewBusy = true;
    try {
      const updated = await post(`/assets/${detail.id}/review`, { approved, expected_updated_at: detail.updated_at });
      assets = assets.map((a: any) => (a.id === updated.id ? updated : a));
      detail = updated;
      toast.success(approved ? 'Image available for reuse' : 'Image excluded from generation');
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      reviewBusy = false;
    }
  }

  function closeDetail() {
    if (identityBusy || reviewBusy) return;
    if (detail && assetDetailsDirty() && !window.confirm('Discard unsaved identity changes?')) return;
    detail = null;
  }
  function openDetail(asset: any) {
    if (detail && detail.id !== asset.id && assetDetailsDirty() && !window.confirm('Discard unsaved identity changes?'))
      return;
    if (detail?.id !== asset.id) detailedView = false;
    detail = asset;
    assetNameDraft = asset.name;
    assetDescriptionDraft = asset.description ?? '';
    void loadUsage(asset.id);
    identityTraits = Object.entries(asset.metadata_json?.locked_traits ?? {}).map(([name, value]) => ({
      name,
      value: String(value)
    }));
  }
  async function saveIdentity() {
    if (!detail) return;
    identityBusy = true;
    try {
      const traits: Record<string, string> = {};
      for (const row of identityTraits) {
        if (!row.name.trim() || !row.value.trim()) throw new Error('Each identity trait needs a name and value');
        if (traits[row.name.trim()]) throw new Error('Trait names must be unique');
        traits[row.name.trim()] = row.value.trim();
      }
      const updated = await patch(`/assets/${detail.id}`, {
        expected_updated_at: detail.updated_at,
        name: assetNameDraft,
        description: assetDescriptionDraft,
        locked_traits: traits
      });
      assets = assets.map((a: any) => (a.id === updated.id ? updated : a));
      detail = updated;
      assetNameDraft = updated.name;
      assetDescriptionDraft = updated.description ?? '';
      identityTraits = Object.entries(updated.metadata_json?.locked_traits ?? {}).map(([name, value]) => ({
        name,
        value: String(value)
      }));
      toast.success('Asset details saved');
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      identityBusy = false;
    }
  }

  import { protectDraft } from '$lib/protect-draft';
  import { UPLOAD_LIMIT_HINT, uploadProblem } from '$lib/upload-limits';
  import { onMount, onDestroy } from 'svelte';
  import { replaceState } from '$app/navigation';
  import { page } from '$app/state';
  import { get, post, put, patch, del, upload, mediaUrl, isImage, isVideo } from '$lib/api';
  import { runBackgroundOp } from '$lib/ops';
  import { Button } from '$lib/components/ui/button';
  import { Badge } from '$lib/components/ui/badge';
  import { Card, CardContent } from '$lib/components/ui/card';
  import { Skeleton } from '$lib/components/ui/skeleton';
  import * as Dialog from '$lib/components/ui/dialog';
  import { Input } from '$lib/components/ui/input';
  import { Label } from '$lib/components/ui/label';
  import { Textarea } from '$lib/components/ui/textarea';
  import { toast } from 'svelte-sonner';
  import { Upload, Palette, Wand2, Pin, Trash2, X, Sparkles, Maximize2 } from '@lucide/svelte';

  const isAudio = (path?: string | null) => !!path && /\.(mp3|m4a|aac|wav|ogg|flac|opus)$/i.test(path);

  let assets: any[] = $state([]);
  let characters: any[] = $state([]);
  let error = $state('');
  let busy = $state(false);
  let search = $state('');
  let loaded = $state(false);

  // canonical asset vocabulary — fetched from the backend so the upload dropdown
  // and the type filter never drift from the AssetType enum. Falls back to the
  // known set if the endpoint is unavailable.
  let canonicalTypes: string[] = $state([
    'prop',
    'background',
    'character_reference',
    'storyboard',
    'video',
    'effect',
    'tool',
    'other'
  ]);

  // --- filter + sort ---
  let typeFilter = $state(''); // '' = all
  let sortBy = $state('newest'); // newest | oldest | name | type

  // --- Project style guide ---
  let styleBaseline = JSON.stringify({ style_prompt: '', palette: '', lighting: '', audience: '', tone: '' });
  export function hasDraft() {
    return (
      !!(createOpen && (files?.length || assetType || characterId)) ||
      (!!styleBaseline && JSON.stringify(styleForm) !== styleBaseline) ||
      !!(detail && assetDetailsDirty())
    );
  }
  protectDraft(() => hasDraft() && !createOpen);
  let style: any = $state(null);
  let styleLoaded = $state(false);
  let styleEditing = $state(false); // "Create manually" with no row yet
  let styleBusy = $state('');
  let styleSource: any = $state(null);
  function deriveStyleFrom(asset: any) {
    styleSource = asset;
    reingestOpen = true;
  }
  let styleForm = $state({ style_prompt: '', palette: '', lighting: '', audience: '', tone: '' });
  let reingestOpen = $state(false);
  let deleteTarget: any = $state(null);

  // --- lightbox / detail ---
  let detail: any = $state(null);
  let detailedView = $state(false);

  // --- describe with AI ---
  let describeTarget: any = $state(null);
  let describeText = $state('');
  let describeBusy = $state(false);

  async function loadTypes() {
    try {
      const r = await get('/assets/types');
      if (Array.isArray(r?.types) && r.types.length) canonicalTypes = r.types;
    } catch {
      /* keep the built-in fallback */
    }
  }

  function seedStyleForm(s: any) {
    styleForm = {
      style_prompt: s?.style_prompt ?? '',
      palette: s?.palette ?? '',
      lighting: s?.lighting ?? '',
      audience: s?.audience ?? '',
      tone: s?.tone ?? ''
    };
    styleBaseline = JSON.stringify(styleForm);
  }

  async function loadStyle() {
    try {
      style = await get('/style');
      if (style) seedStyleForm(style);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      styleLoaded = true;
    }
  }

  function ingestStyle() {
    styleSource = null;
    reingestOpen = true;
  }

  async function saveStyle() {
    styleBusy = 'save';
    try {
      style = await patch('/style', styleForm);
      styleBaseline = JSON.stringify(styleForm);
      styleEditing = false;
      toast.success('Style saved.');
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      styleBusy = '';
    }
  }

  async function confirmDeleteAsset() {
    const a = deleteTarget;
    if (!a) return;
    busy = true;
    try {
      const r = await del(`/assets/${a.id}`);
      deleteTarget = null;
      if (detail?.id === a.id) detail = null;
      toast.success(`Asset deleted (detached from ${r.detached_from} place${r.detached_from === 1 ? '' : 's'}).`);
      await Promise.all([refresh(), loadStyle()]);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      busy = false;
    }
  }

  // ---- describe with AI (structures uploader-provided text into metadata) ----
  function openDescribe(asset: any) {
    describeTarget = asset;
    describeText = asset.description ?? '';
  }

  async function runDescribe() {
    const a = describeTarget;
    if (!a) return;
    const text = describeText.trim();
    if (!text) {
      toast.error('Add a few words describing the asset first.');
      return;
    }
    describeBusy = true;
    try {
      const updated = await post(`/assets/${a.id}/recognise`, { description: text });
      describeTarget = null;
      if (detail?.id === a.id) detail = updated;
      toast.success('Asset described — name, type and tags updated.');
      await refresh();
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      describeBusy = false;
    }
  }

  let createError = $state('');
  let createOpen = $state(false);
  function changeCreateOpen(value: boolean) {
    if (!value && (busy || ((files?.length || assetType || characterId) && !window.confirm('Discard this asset draft?'))))
      return;
    if (value && !createOpen) createError = '';
    createOpen = value;
    if (!value) {
      files = null;
      assetType = '';
      characterId = '';
    }
  }
  protectDraft(() => createOpen && !!(files?.length || assetType || characterId), 'Discard this asset draft and leave?');
  let files: FileList | null = $state(null);
  let assetType = $state('');
  let characterId = $state('');

  let loadSequence = 0;
  export async function reload() {
    await retryMedia();
  }
  async function refresh() {
    const sequence = ++loadSequence;
    try {
      const [nextAssets, nextCharacters, nextScenes] = await Promise.all([
        get('/assets'),
        get('/characters'),
        get('/scenes')
      ]);
      if (sequence !== loadSequence) return;
      assets = nextAssets;
      characters = nextCharacters;
      libraryScenes = nextScenes;
      oncounts?.(characters.length, assets.length);
    } catch (e) {
      if (sequence === loadSequence) throw e;
    }
  }
  onMount(() => {
    refresh()
      .then(() => {
        const params = new URL(window.location.href).searchParams;
        reviewScene = params.get('scene_id') ?? '';
        const id = params.get('asset_id');
        if (id) {
          const asset = assets.find((a: any) => a.id === id);
          if (asset) openDetail(asset);
        }
      })
      .catch(e => (error = e.message))
      .finally(() => (loaded = true));
    loadStyle();
    loadTypes();
  });

  async function doUpload(e: Event) {
    e.preventDefault();
    if (!files?.length || busy) return;
    const problem = uploadProblem(Array.from(files));
    if (problem) {
      createError = problem;
      return;
    }
    busy = true;
    createError = '';
    try {
      const form = new FormData();
      for (const f of Array.from(files)) form.append('file', f);
      if (assetType) form.append('asset_type', assetType);
      if (characterId) form.append('character_id', characterId);
      const r = await upload('/assets/upload', form);
      const n = Array.isArray(r) ? r.length : 1;
      files = null;
      assetType = '';
      characterId = '';
      createOpen = false;
      search = '';
      typeFilter = '';
      kindFilter = '';
      readinessFilter = '';
      latestOnly = false;
      sortBy = 'newest';
      toast.success(`Uploaded ${n} file${n === 1 ? '' : 's'}.`);
      await refresh();
    } catch (e) {
      createError = errorMessage(e);
      if (!createOpen) toast.error(errorMessage(e));
    } finally {
      busy = false;
    }
  }

  // Map any stored type onto the canonical vocabulary for coherent filtering —
  // legacy/unknown values collapse to "other" (mirrors the backend canonicaliser).
  const canonical = (t: string | null | undefined) => (t && canonicalTypes.includes(t) ? t : 'other');

  // Distinct types actually present (canonicalised), for the filter chips.
  const presentTypes = $derived(
    Array.from(new Set(assets.map(a => canonical(a.type)))).sort(
      (x, y) => canonicalTypes.indexOf(x) - canonicalTypes.indexOf(y)
    )
  );

  let filtered = $derived.by(() => {
    const scene = libraryScenes.find(s => s.id === reviewScene);
    const associated = new Set<string>([
      ...(scene?.asset_ids_json ?? []),
      ...(scene?.scene_json?.story_asset_ids ?? []),
      ...(scene?.character_ids_json ?? []).flatMap(
        (id: string) => characters.find(c => c.id === id)?.reference_asset_ids_json ?? []
      )
    ]);
    let out = reviewScene
      ? assets.filter((a: any) => associated.has(a.id) || a.metadata_json?.scene_id === reviewScene)
      : assets;
    if (latestOnly) out = latestAssetVersions(out);
    if (readinessFilter) out = out.filter(a => assetReadiness(a) === readinessFilter);
    if (kindFilter) out = out.filter(a => mediaKind(a) === kindFilter);
    if (search.trim()) {
      const q = search.trim().toLowerCase();
      out = out.filter(a => [a.name, a.type, a.description, ...(a.tags_json ?? [])].join(' ').toLowerCase().includes(q));
    }
    if (typeFilter) out = out.filter(a => canonical(a.type) === typeFilter);

    const sorted = out.slice();
    if (sortBy === 'newest') sorted.sort((a, b) => (b.created_at ?? '').localeCompare(a.created_at ?? ''));
    else if (sortBy === 'oldest') sorted.sort((a, b) => (a.created_at ?? '').localeCompare(b.created_at ?? ''));
    else if (sortBy === 'name') sorted.sort((a, b) => (a.name ?? a.id).localeCompare(b.name ?? b.id));
    else if (sortBy === 'type') sorted.sort((a, b) => canonical(a.type).localeCompare(canonical(b.type)));
    return sorted;
  });
</script>

<svelte:head>{#if !embedded}<title>Assets · VideoFlow</title>{/if}</svelte:head>

<div class="px-4 py-6 sm:px-8">
  <div class="mb-4 flex items-center justify-between gap-3">
    <div>{#if embedded}<h2 class="text-lg font-semibold">Media</h2>{:else}<h1
      class="text-2xl font-semibold">Assets</h1>{/if}<p
      class="mt-1 text-sm text-muted-foreground">{error
          ? 'Collection unavailable'
          : loaded
            ? `${assets.length} ${assets.length === 1 ? 'file' : 'files'}`
            : 'Loading collection…'}</p></div>
    <Dialog.Root open={createOpen} onOpenChange={changeCreateOpen}>
      <Dialog.Trigger>{#snippet child({props})}<Button {...props}
        disabled={!loaded||busy}>{embedded?'Upload media':'Add assets'}</Button>{/snippet}</Dialog.Trigger>
      <Dialog.Content
        class="max-h-[90dvh] overflow-y-auto sm:max-w-lg"
        showCloseButton={false}
        overlayClass="bg-black/55 supports-backdrop-filter:backdrop-blur-none"
        onEscapeKeydown={e=>{e.preventDefault();changeCreateOpen(false);}}
        onInteractOutside={e=>{e.preventDefault();changeCreateOpen(false);}}>
<Dialog.Header><Dialog.Title>{embedded?'Upload media':'Add assets'}</Dialog.Title><Dialog.Description
  class="sr-only">Upload files and optionally link them to a character.</Dialog.Description></Dialog.Header>
  <form class="space-y-4" onsubmit={doUpload}>
    {#if createError}<p role="alert" class="text-sm text-destructive">{createError}</p>{/if}
    <div class="grid gap-4">
      <div class="flex-1 min-w-[160px]">
        <label class="block text-xs text-muted-foreground mb-1" for="file">Files</label>
        <p id="upload-limit" class="mb-2 text-xs text-muted-foreground">Images, video, audio and other reference files. {UPLOAD_LIMIT_HINT}</p>
        <input id="file" aria-describedby="upload-limit" type="file" multiple bind:files disabled={busy}
          class="block w-full text-sm rounded-md border border-input bg-background px-2 py-1.5" />
      </div>
      <div class="flex-1 min-w-[160px]">
        <label class="block text-xs text-muted-foreground mb-1" for="type">Asset type</label>
        <select id="type" bind:value={assetType} disabled={busy}
          class="w-full rounded-md border border-input bg-background px-2 py-1.5 text-sm">
          <option value="">Detect automatically</option>
          {#each canonicalTypes as t}<option value={t}>{t}</option>{/each}
        </select>
      </div>
      <div class="flex-1 min-w-[160px]">
        <label class="block text-xs text-muted-foreground mb-1" for="char">Character (optional)</label>
        <select id="char" bind:value={characterId} disabled={busy}
          class="w-full rounded-md border border-input bg-background px-2 py-1.5 text-sm">
          <option value="">No character</option>
          {#each characters as c}<option value={c.id}>{c.name}</option>{/each}
        </select>
      </div>
      <div class="flex justify-end gap-2">
        <Button type="button" variant="outline" disabled={busy} onclick={()=>changeCreateOpen(false)}>Cancel</Button>
        <Button type="submit" disabled={busy || !files?.length} size="sm">
          {busy?'Uploading…':embedded?'Upload files':'Add assets'}
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
          disabled={busy}
          onclick={()=>changeCreateOpen(false)}><X class="size-4" /></Button>
      </Dialog.Content>
    </Dialog.Root>
  </div>





  <div
    class="mb-4 flex flex-wrap items-center gap-2"
    role="group"
    aria-label="Media kinds">{#each [
        { value: '', label: 'All files' },
        { value: 'image', label: 'Images' },
        { value: 'video', label: 'Video' },
        { value: 'audio', label: 'Audio' },
        { value: 'other', label: 'Other' }
      ] as kind}<Button
      variant={kindFilter===kind.value?'secondary':'ghost'}
      size="sm"
      aria-pressed={kindFilter===kind.value}
      onclick={()=>kindFilter=kind.value}>{kind.label}</Button>{/each}</div>
  <div class="mb-5 flex flex-wrap items-center gap-3"><input
    type="search"
    aria-label="Search media"
    placeholder="Search files, descriptions or tags…"
    bind:value={search}
    class="min-h-11 min-w-0 flex-1 basis-60 rounded-lg border border-input bg-background px-3 text-sm"/>
    <select
      aria-label="Asset type"
      bind:value={typeFilter}
      class="min-h-11 max-w-full rounded-lg border border-input bg-background px-3 text-sm"><option
        value="">All asset types</option>{#each presentTypes as type}<option value={type}>{typeLabel(type)}</option>{/each}</select>
    <select
      aria-label="Sort media"
      bind:value={sortBy}
      class="min-h-11 rounded-lg border border-input bg-background px-3 text-sm"><option
        value="newest">Newest first</option><option value="oldest">Oldest first</option><option
        value="name">Name A–Z</option><option value="type">Asset type</option></select>
    <div
      role="group"
      aria-label="Media view"
      class="flex rounded-lg border border-border p-1"><Button
      variant={view==='grid'?'secondary':'ghost'}
      size="icon"
      class="size-9"
      aria-label="Grid view"
      aria-pressed={view==='grid'}
      onclick={()=>view='grid'}><LayoutGrid class="size-4"/></Button><Button
      variant={view==='list'?'secondary':'ghost'}
      size="icon"
      class="size-9"
      aria-label="List view"
      aria-pressed={view==='list'}
      onclick={()=>view='list'}><List class="size-4"/></Button></div>
  </div>
  <details class="mb-4 rounded-lg border border-border px-3 py-2"><summary
    class="cursor-pointer py-1 text-sm">Production filters{readinessFilter||latestOnly?' · Active':''}</summary><div
    class="flex flex-wrap items-center gap-3 pt-3 pb-1"><select
    aria-label="Asset readiness"
    bind:value={readinessFilter}
    class="min-h-11 rounded-lg border border-input bg-background px-3 text-sm"><option
      value="">Any readiness</option><option value="has-file">Has file</option><option
      value="missing">Missing media</option><option
      value="excluded">Excluded from generation</option></select><label
      class="flex min-h-11 items-center gap-2 text-sm"><input type="checkbox"
      bind:checked={latestOnly}/>Latest version per asset</label><p
      class="text-xs text-muted-foreground">Version filtering only changes this view. Scenes keep their selected references.</p></div></details>
  {#if error}<div
    role="alert"
    class="mb-4 space-y-2 rounded-lg border border-destructive p-4 text-sm"><p>Could not load media: {error}</p>{#if assets.length}<p>Showing the last loaded files.</p>{/if}<Button
      variant="outline"
      disabled={refreshing}
      onclick={retryMedia}>{refreshing?'Loading…':'Try again'}</Button></div>{/if}
  {#if reviewScene}<p class="mb-4 text-sm">Media for {libraryScenes.find(s=>s.id===reviewScene)?.title??'this scene'}. <a
    class="inline-flex min-h-11 items-center underline underline-offset-4"
    href={`/?view=story&scene_id=${encodeURIComponent(reviewScene)}`}>Open scene studio</a> <button
    class="min-h-11 underline"
    onclick={clearSceneFilter}>Show all media</button></p>{/if}
  {#if !loaded}<div
    class="grid grid-cols-1 gap-5 min-[480px]:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">{#each Array(4) as _}<Skeleton
    class="aspect-[4/3] w-full rounded-xl"/>{/each}</div>
  {:else if !assets.length&&!error}<div class="py-16 text-center"><Upload
    class="mx-auto size-9 text-muted-foreground"/><h3
    class="mt-4 font-semibold">Build your media library</h3><p
    class="mx-auto mt-2 max-w-sm text-sm leading-relaxed text-muted-foreground">Upload reference images, video, audio or documents. Generated media will appear here too.</p></div>
  {:else if assets.length&&!filtered.length}<div role="status" class="py-16 text-center"><h3
    class="font-semibold">No matching media</h3><p class="mt-2 text-sm text-muted-foreground">Try another search or clear the filters.</p><Button
    variant="outline"
    class="mt-4"
    onclick={() => {
      search = '';
      typeFilter = '';
      kindFilter = '';
      readinessFilter = '';
      latestOnly = false;
    }}>Clear filters</Button></div>
  {:else if loaded&&assets.length}<p role="status" class="mb-4 text-xs text-muted-foreground">Showing {filtered.length} of {assets.length} files</p>{/if}
  <div
    role="region"
    aria-label="Media collection"
    class={view==='grid'?'grid grid-cols-1 gap-5 min-[480px]:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4':'flex flex-col gap-3'}>
    {#each filtered as asset (asset.id)}
      {@const name=asset.name||asset.file_path?.split('/').pop()||'Untitled file'}
      {@const kind=mediaKind(asset)}
      <Card class="py-0"><CardContent class={view==='list'?'flex min-w-0 items-center p-0':'p-0'}>
        <button
          type="button"
          class={view === 'list'
            ? 'block w-28 shrink-0 text-left focus-visible:outline-2 focus-visible:outline-ring'
            : 'block w-full text-left focus-visible:outline-2 focus-visible:outline-ring'}
          aria-label={'Preview '+name}
          onclick={()=>openDetail(asset)}><LibraryPreview {asset} class="aspect-[4/3]"/></button>
        <div class="min-w-0 flex-1 p-4"><div class="flex items-start gap-2"><div class="min-w-0 flex-1"><h3
          class="truncate text-sm font-semibold" title={name}>{name}</h3><p
          class="mt-1 text-xs text-muted-foreground">{typeLabel(kind)}{#if canonical(asset.type)!==kind&&canonical(asset.type)!=='other'} · {typeLabel(canonical(asset.type))}{/if}</p></div>
          <DropdownMenu.Root><DropdownMenu.Trigger
            class="inline-flex size-9 shrink-0 items-center justify-center rounded-lg hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring"
            aria-label={'Actions for '+name}><Ellipsis class="size-4"/></DropdownMenu.Trigger><DropdownMenu.Content align="end">
            <DropdownMenu.Item onclick={()=>openDetail(asset)}>Open detail</DropdownMenu.Item><DropdownMenu.Item
              disabled={busy}
              onclick={()=>openDescribe(asset)}>Describe with AI</DropdownMenu.Item>{#if isImage(asset.file_path)}<DropdownMenu.Item
              disabled={busy||!!styleBusy}
              onclick={()=>deriveStyleFrom(asset)}>Derive visual style</DropdownMenu.Item>{/if}<DropdownMenu.Separator/><DropdownMenu.Item
              disabled={busy}
              onclick={()=>prepareDelete(asset)}>Delete asset</DropdownMenu.Item>
          </DropdownMenu.Content></DropdownMenu.Root>
        </div>
        {#if asset.description&&view==='grid'}<p
          class="mt-2 line-clamp-2 text-xs leading-relaxed text-muted-foreground">{parseDescription(asset.description).summary}</p>{/if}
        {#if assetReadiness(asset)==='missing'}<p class="mt-2 text-xs text-muted-foreground">Missing media</p>{/if}
        {#if asset.metadata_json?.variant_group_id}<p
          class="mt-2 text-xs text-muted-foreground">{assets.filter(a=>versionGroup(a)===versionGroup(asset)).length} versions · Open detail to compare</p>{/if}
        {#if asset.metadata_json?.consistency_review||asset.tags_json?.length}<div
          class="mt-3 flex flex-wrap gap-1">{#if asset.metadata_json?.consistency_review}<Badge
          variant="outline">{asset.metadata_json.consistency_review==='rejected'?'Excluded':'Ready to use'}</Badge>{/if}{#each (asset.tags_json??[]).slice(0,3) as tag}<Badge
          variant="secondary">{tag}</Badge>{/each}{#if asset.tags_json?.length>3}<span
          class="px-1 text-xs text-muted-foreground">+{asset.tags_json.length-3}</span>{/if}</div>{/if}
        </div>
      </CardContent></Card>
    {/each}
  </div>
  <details class="mt-8 rounded-lg border border-border p-3"><summary class="cursor-pointer text-sm font-medium">Project style</summary>
  <div class="pt-3">
    <div class="space-y-3">


      {#if !styleLoaded}
        <p class="text-sm text-muted-foreground">Loading…</p>
      {:else if !style && !styleEditing}
        <p class="text-sm text-muted-foreground mb-2">Choose how to set the project style.</p>
        <div class="flex gap-2">
          <Button
            size="sm"
            disabled={!!styleBusy}
            onclick={ingestStyle}
          >
            <Wand2 class="size-3 mr-1" />Analyze a reference
          </Button>
          <Button variant="outline" size="sm" onclick={() => (styleEditing = true)}>
            Create manually
          </Button>
        </div>

      {:else}
        <div class="grid gap-3">
          <div class="grid gap-1.5">
            <Label for="style-prompt">Style prompt</Label>
            <Textarea id="style-prompt" rows={3} bind:value={styleForm.style_prompt}
              placeholder="e.g. warm watercolor children's book illustration, soft edges…" />
          </div>
          <div class="grid grid-cols-2 gap-3 md:grid-cols-4">
            <div class="grid gap-1.5">
              <Label for="style-palette">Palette</Label>
              <Input id="style-palette" bind:value={styleForm.palette} />
            </div>
            <div class="grid gap-1.5">
              <Label for="style-lighting">Lighting</Label>
              <Input id="style-lighting" bind:value={styleForm.lighting} />
            </div>
            <div class="grid gap-1.5">
              <Label for="style-audience">Audience</Label>
              <Input id="style-audience" bind:value={styleForm.audience} />
            </div>
            <div class="grid gap-1.5">
              <Label for="style-tone">Tone</Label>
              <Input id="style-tone" bind:value={styleForm.tone} />
            </div>
          </div>


          <div class="flex gap-2">
            <Button size="sm" disabled={!!styleBusy} onclick={saveStyle}>
              {styleBusy === 'save' ? 'Saving…' : 'Save'}
            </Button>
            <Button variant="outline" size="sm" disabled={!!styleBusy} onclick={() => {styleSource=null;reingestOpen=true}}>
              <Wand2 class="size-3 mr-1" />Analyze a reference
            </Button>
          </div>
        </div>
      {/if}
    </div>
  </div>
  </details>
</div>

<!-- Asset detail / lightbox -->
<Dialog.Root open={detail !== null} onOpenChange={(open) => {if(!open)closeDetail()}}>
  <Dialog.Content class="max-h-[90vh] overflow-y-auto max-w-4xl sm:max-w-4xl">
    <Dialog.Header>
      <Dialog.Title class="truncate">{detail?.name || detail?.id}</Dialog.Title>
      <Dialog.Description>{canonical(detail?.type).replaceAll('_',' ')}</Dialog.Description>
      <div
        class="flex gap-1 pt-2"
        role="group"
        aria-label="Asset view"><Button
        size="sm"
        variant={!detailedView?'secondary':'ghost'}
        aria-pressed={!detailedView}
        onclick={()=>detailedView=false}>Simple</Button><Button
        size="sm"
        variant={detailedView?'secondary':'ghost'}
        aria-pressed={detailedView}
        onclick={()=>detailedView=true}>Details{assetDetailsDirty()?' · Unsaved':''}</Button></div>
    </Dialog.Header>
    {#if detail}
      <div class="grid items-start gap-6 md:grid-cols-[minmax(0,1fr)_280px]">
        <div class="flex items-center justify-center bg-muted rounded-md h-[45dvh] md:h-[60dvh] overflow-hidden">
          {#if mediaKind(detail)==='video'}
            <!-- svelte-ignore a11y_media_has_caption -->
            <video controls preload="metadata" src={mediaUrl(detail.file_path)}
              class="h-full w-full rounded-md bg-black object-contain"></video>
          {:else if mediaKind(detail)==='audio'}
            {#if mediaUrl(detail.file_path)}
              <audio controls preload="metadata" src={mediaUrl(detail.file_path)} class="w-full"></audio>
            {:else}
              <p class="p-8 text-sm text-muted-foreground">This audio file has no playable local URL.</p>
            {/if}
          {:else if mediaUrl(detail.file_path) && mediaKind(detail)==='image'}
            <img class="h-full w-full rounded-md object-contain"
              src={mediaUrl(detail.file_path)} alt={detail.name || detail.id} />
          {:else}
            <div class="p-8 text-sm text-muted-foreground">This file type cannot be previewed here.</div>
          {/if}
        </div>
        <div class="space-y-3 text-xs">
 {#if !detailedView}<AssetDescription description={detail.description??''}/>{/if}
 {#if detail.metadata_json?.consistency_review==='rejected'}<p class="text-sm">Excluded from generation.</p>{/if}
 {#if detailedView&&detail.metadata_json?.consistency_review}<Button
   variant="outline"
   size="sm"
   disabled={reviewBusy||identityBusy||assetDetailsDirty()}
   onclick={()=>reviewReference(detail.metadata_json.consistency_review==='rejected')}>{detail.metadata_json.consistency_review==='rejected'?'Allow reuse':'Exclude from generation'}</Button>{/if}
 {#if detailedView}
 {#if ['prop','location'].includes(detail.type)}<section aria-label="Asset versions"><h3
   class="font-semibold">Versions</h3><p
   class="mt-2 text-xs text-muted-foreground">Generate a candidate with the same identity locks. Scenes keep their selected version until you explicitly replace it.</p><ul
   class="mt-2 space-y-1">{#each assets.filter(a=>versionGroup(a)===versionGroup(detail)) as version}<li><button
   type="button"
   class="min-h-11 text-left underline underline-offset-4"
   disabled={version.id===detail.id}
   onclick={()=>openDetail(version)}>{version.name} · {version.id===detail.id?'Viewing':version.metadata_json?.consistency_review==='rejected'?'Excluded':'Ready'}</button></li>{/each}</ul><Button
     variant="outline"
     size="sm"
     disabled={!!regenerating||identityBusy||reviewBusy}
     onclick={()=>requestRegeneration(detail)}>{regenerating===detail.id?'Generating version…':'Generate new version'}</Button></section>{/if}
 {/if}
 <details><summary class="cursor-pointer py-2 font-semibold">Used in this project</summary><section
   aria-label="Asset usage">{#if usageError[detail.id]}<p class="mt-2 text-muted-foreground">{usageError[detail.id]}</p><Button
   variant="outline"
   size="sm"
   onclick={()=>loadUsage(detail.id)}>Reload usage</Button>{:else if !usage[detail.id]}<p
     class="mt-2 text-muted-foreground"
     role="status">Checking usage…</p>{:else if usage[detail.id].uses.length}<ul
     class="mt-2 space-y-2">{#each usage[detail.id].uses as use}<li>{#if use.kind==='scene'}<a
   class="inline-flex min-h-11 items-center underline underline-offset-4"
   href={`/?view=story&scene_id=${encodeURIComponent(use.id)}`}>{use.name} · Scene studio</a>{:else if use.kind==='character'}<a
   class="inline-flex min-h-11 items-center underline underline-offset-4"
   href={`/characters/${encodeURIComponent(use.id)}`}>{use.name} · Character</a>{:else}<span>{use.name} · {use.kind}</span>{/if}</li>{/each}</ul><p
     class="mt-2 text-xs text-muted-foreground">This shared asset cannot be deleted while it is in use. Remove a scene binding in its studio; existing renders retain their original references.</p>{:else}<p
     class="mt-2 text-muted-foreground">No active bindings. This asset is available for reuse.</p>{/if}</section>
 </details>
 {#if detailedView}
 <section aria-label="Asset identity locks"><h3 class="font-semibold">Asset details &amp; identity</h3><div
   class="mt-3 space-y-3"><label class="block text-sm">Asset name<input
   class="mt-1 w-full rounded-md border border-input bg-background px-3 py-2"
   disabled={identityBusy||reviewBusy}
   bind:value={assetNameDraft}/></label><AssetDescriptionEditor
   bind:value={assetDescriptionDraft}
   disabled={identityBusy||reviewBusy}/></div><p
     class="text-muted-foreground">Keep specific traits across scenes, for example kind: car and color: red. Select this reference in each scene studio. Changed appearances may need a new storyboard image.</p>
 {#each identityTraits as row,i}<div class="mt-2 flex gap-2"><input
   aria-label={`Trait ${i+1} name`}
   class="min-w-0 w-1/3 rounded border p-2"
   disabled={identityBusy||reviewBusy}
   bind:value={row.name}
   placeholder="color"/><input
   aria-label={`Trait ${i+1} value`}
   class="min-w-0 flex-1 rounded border p-2"
   disabled={identityBusy||reviewBusy}
   bind:value={row.value}
   placeholder="red"/><button
   aria-label={`Remove trait ${i+1}`}
   disabled={identityBusy||reviewBusy}
   onclick={()=>identityTraits=identityTraits.filter((_,n)=>n!==i)}>Remove</button></div>{/each}
 <Button
   size="sm"
   variant="outline"
   disabled={identityBusy||identityTraits.length>=32}
   onclick={()=>identityTraits=[...identityTraits,{name:'',value:''}]}>Add trait</Button><Button
   size="sm"
   disabled={identityBusy||reviewBusy||!assetDetailsDirty()}
   onclick={saveIdentity}>{identityBusy?'Saving…':'Save asset details'}</Button></section>

          {#each nodeTools('asset',detail.id,detail).filter((tool:any)=>tool.href.startsWith('/?view=edit')) as tool}<Button
            variant="outline" href={tool.href}>{tool.label}</Button>{/each}
          {#if detail.character_id}
            <div>
              <div class="text-muted-foreground mb-0.5">Character</div>
              <div>{characters.find((c) => c.id === detail.character_id)?.name ?? detail.character_id}</div>
            </div>
          {/if}
          {#if detail.tags_json?.length}
            <div>
              <div class="text-muted-foreground mb-1">Tags</div>
              <div class="flex flex-wrap gap-1">
                {#each detail.tags_json as tag}<Badge variant="outline">{tag}</Badge>{/each}
              </div>
            </div>
          {/if}
          <details><summary class="cursor-pointer py-2 font-semibold">Technical information</summary><p
            class="mb-2 break-all text-muted-foreground">{detail.id}</p>
          <div>
            <div class="text-muted-foreground mb-0.5">File</div>
            <div class="break-all">{detail.file_path ?? '—'}</div>
          </div>
          {#if detail.created_at}
            <div>
              <div class="text-muted-foreground mb-0.5">Created</div>
              <div>{detail.created_at}</div>
            </div>
          {/if}
          {#if detail.metadata_json && Object.keys(detail.metadata_json).length}
            <div>
              <div class="text-muted-foreground mb-0.5">Metadata</div>
              <pre class="whitespace-pre-wrap break-all rounded-md bg-muted p-2 text-[11px]">{JSON.stringify(detail.metadata_json, null, 2)}</pre>
            </div>
          {/if}
          </details>
          <div class="flex flex-wrap gap-1 pt-1">
            <Button variant="outline" size="sm" onclick={() => openDescribe(detail)}>
              <Sparkles class="size-3 mr-1" />Describe with AI
            </Button>
            {#if isImage(detail.file_path)}
              <Button variant="outline" size="sm" disabled={!!styleBusy} onclick={() => deriveStyleFrom(detail)}>
                <Pin class="size-3 mr-1" />Derive visual style
              </Button>
            {/if}
            <Button variant="ghost" size="sm" class="text-destructive hover:text-destructive"
              onclick={() => prepareDelete(detail)}>
              <Trash2 class="size-3 mr-1" />Delete
            </Button>
          </div>
          {/if}
        </div>
      </div>
    {/if}
  </Dialog.Content>
</Dialog.Root>

<!-- Describe with AI -->
<Dialog.Root open={describeTarget !== null} onOpenChange={(open) => !open && (describeTarget = null)}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>Describe with AI</Dialog.Title>
      <Dialog.Description>
        The assistant structures the text you provide into a name, type and searchable
        tags. It reads your words, not the pixels — describe what's in the asset.
      </Dialog.Description>
    </Dialog.Header>
    <div class="grid gap-1.5">
      <Label for="describe-text">What is this asset?</Label>
      <Textarea id="describe-text" rows={4} bind:value={describeText}
        placeholder="e.g. a red lipstick bullet on a black background, product shot" />
    </div>
    <Dialog.Footer>
      <Button variant="outline" size="sm" onclick={() => (describeTarget = null)}>Cancel</Button>
      <Button size="sm" disabled={describeBusy} onclick={runDescribe}>
        <Sparkles class="size-3 mr-1" />{describeBusy ? 'Structuring…' : 'Structure metadata'}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<Dialog.Root open={reingestOpen} onOpenChange={(open) => !open && (reingestOpen = false)}>
 <Dialog.Content
   class="max-h-[90dvh] overflow-y-auto sm:max-w-xl"><Dialog.Header><Dialog.Title>Style reference</Dialog.Title><Dialog.Description>Extract editable visual direction from an image, video or public link.</Dialog.Description></Dialog.Header><StyleReference
     initialAssetID={styleSource?.id??''}
     onsaved={async()=>{reingestOpen=false;styleSource=null;await loadStyle()}}/></Dialog.Content>
</Dialog.Root>

<Dialog.Root open={deleteTarget !== null} onOpenChange={(open) => !open && (deleteTarget = null)}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>Delete asset?</Dialog.Title>
      <Dialog.Description>
        Delete "{deleteTarget?.name || deleteTarget?.id}" from the project Library? Assets used by characters, scenes, shots, renders or AI sessions must be unlinked first.
      </Dialog.Description>
    </Dialog.Header>
    <Dialog.Footer>
      <Button variant="outline" size="sm" onclick={() => (deleteTarget = null)}>Cancel</Button>
      {#if deleteTarget&&usageError[deleteTarget.id]}<p role="alert"
        class="text-sm">{usageError[deleteTarget.id]}</p>{:else if deleteTarget&&usage[deleteTarget.id]?.uses.length}<p
        role="status" class="text-sm">Used in {usage[deleteTarget.id].uses.length} places. Open its details to review the bindings.</p>{/if}
      <Button
        variant="destructive"
        size="sm"
        disabled={busy||!deleteTarget||!usage[deleteTarget.id]?.can_delete||!!usageError[deleteTarget.id]}
        onclick={confirmDeleteAsset}>
        <Trash2 class="size-3 mr-1" />{busy ? 'Deleting…' : 'Delete'}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<Dialog.Root
  open={regenerateTarget!==null}
  onOpenChange={open=>{if(!open)regenerateTarget=null}}><Dialog.Content><Dialog.Header><Dialog.Title>Generate a new asset version?</Dialog.Title><Dialog.Description>Use the saved description and identity locks for “{regenerateTarget?.name}”. Codex is used first when supported; generation may use account quota or provider credits. Existing scene references stay unchanged.</Dialog.Description></Dialog.Header><Dialog.Footer><Button
    variant="outline"
    onclick={()=>regenerateTarget=null}>Cancel</Button><Button disabled={!!regenerating}
    onclick={regenerateAsset}>Generate version</Button></Dialog.Footer></Dialog.Content></Dialog.Root>
