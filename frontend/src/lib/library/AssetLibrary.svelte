<script lang="ts">
  let { embedded = false,oncounts }:{embedded?:boolean;oncounts?:(characters:number,media:number)=>void}=$props();
  import LibraryPreview from './LibraryPreview.svelte';
  import {mediaKind,typeLabel} from './catalog.mjs';
  import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
  import {Ellipsis,LayoutGrid,List} from '@lucide/svelte';
  let kindFilter=$state(''),view=$state<'grid'|'list'>('grid'),refreshing=$state(false);
  async function retryMedia(){refreshing=true;try{await refresh();error='';}catch(e){error=String(e)}finally{refreshing=false}}
 let identityTraits: {name:string;value:string}[] = $state([]);
 let identityBusy = $state(false);
 let reviewBusy=$state(false);let reviewScene=$state('');
 async function reviewReference(approved:boolean){if(!detail)return;reviewBusy=true;try{const updated=await post(`/assets/${detail.id}/review`,{approved,expected_updated_at:detail.updated_at});assets=assets.map((a:any)=>a.id===updated.id?updated:a);detail=updated;toast.success(approved?'Reference approved for reuse':'Reference rejected');}catch(e){toast.error((e as Error).message)}finally{reviewBusy=false}}

 function closeDetail(){if(identityBusy||reviewBusy)return;if(detail&&JSON.stringify(identityTraits)!==JSON.stringify(Object.entries(detail.metadata_json?.locked_traits??{}).map(([name,value])=>({name,value:String(value)})))&&!window.confirm('Discard unsaved identity changes?'))return;detail=null;}
 function openDetail(asset:any){detail=asset;identityTraits=Object.entries(asset.metadata_json?.locked_traits??{}).map(([name,value])=>({name,value:String(value)}));}
 async function saveIdentity(){if(!detail)return;identityBusy=true;try{const traits:Record<string,string>={};for(const row of identityTraits){if(!row.name.trim()||!row.value.trim())throw new Error('Each identity trait needs a name and value');if(traits[row.name.trim()])throw new Error('Trait names must be unique');traits[row.name.trim()]=row.value.trim()}const updated=await put(`/assets/${detail.id}/identity`,{expected_updated_at:detail.updated_at,locked_traits:traits});assets=assets.map((a:any)=>a.id===updated.id?updated:a);detail=updated;toast.success('Identity locks saved');}catch(e){toast.error((e as Error).message)}finally{identityBusy=false}}

  import { protectDraft } from '$lib/protect-draft';
  import { UPLOAD_LIMIT_HINT, uploadProblem } from '$lib/upload-limits';
  import { onMount } from 'svelte';
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
  let styleBaseline=JSON.stringify({style_prompt:'',palette:'',lighting:'',audience:'',tone:''});
  export function hasDraft(){return !!(createOpen&&(files?.length||assetType||characterId))||(!!styleBaseline&&JSON.stringify(styleForm)!==styleBaseline)||!!(detail&&JSON.stringify(identityTraits)!==JSON.stringify(Object.entries(detail.metadata_json?.locked_traits??{}).map(([name,value])=>({name,value:String(value)}))));}
  protectDraft(()=>hasDraft()&&!createOpen);
  let style: any = $state(null);
  let styleLoaded = $state(false);
  let styleEditing = $state(false); // "Create manually" with no row yet
  let styleBusy = $state('');
  let styleForm = $state({ style_prompt: '', palette: '', lighting: '', audience: '', tone: '' });
  let reingestOpen = $state(false);
  let deleteTarget: any = $state(null);
  // Ingest derives the style FROM the story — gate it until one exists.
  let hasStory = $state(true);

  // --- lightbox / detail ---
  let detail: any = $state(null);

  // --- describe with AI ---
  let describeTarget: any = $state(null);
  let describeText = $state('');
  let describeBusy = $state(false);

  async function loadStoryPresence() {
    try {
      const [scenes, scripts] = await Promise.all([get('/scenes'), get('/scripts')]);
      hasStory = (scenes?.length ?? 0) > 0 || (scripts?.length ?? 0) > 0;
    } catch {
      hasStory = true; // fail open — the backend will still reject sensibly
    }
  }

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
    styleBaseline=JSON.stringify(styleForm);
  }

  async function loadStyle() {
    try {
      style = await get('/style');
      if (style) seedStyleForm(style);
    } catch (e: any) {
      toast.error(e.message);
    } finally {
      styleLoaded = true;
    }
  }

  async function ingestStyle() {
    styleBusy = 'ingest';
    reingestOpen = false;
    try {
      await runBackgroundOp('/style/ingest', undefined, {
        label: 'Style ingest',
        onDone: async () => {
          styleBusy = '';
          await loadStyle(); // reloads the panel and reseeds the form
          styleEditing = false;
        },
        onFail: () => (styleBusy = '')
      });
    } catch (e: any) {
      styleBusy = '';
      toast.error(e.message);
    }
  }

  async function saveStyle() {
    styleBusy = 'save';
    try {
      style = await patch('/style', styleForm);styleBaseline=JSON.stringify(styleForm);
      styleEditing = false;
      toast.success('Style saved.');
    } catch (e: any) {
      toast.error(e.message);
    } finally {
      styleBusy = '';
    }
  }

  const styleRefs = $derived(
    ((style?.reference_asset_ids_json ?? []) as string[]).map(
      (id) => assets.find((a) => a.id === id) ?? { id, name: id, file_path: null }
    )
  );

  async function patchRefs(ids: string[], doneMsg: string) {
    styleBusy = 'refs';
    try {
      style = await patch('/style', { reference_asset_ids: ids });
      toast.success(doneMsg);
    } catch (e: any) {
      toast.error(e.message);
    } finally {
      styleBusy = '';
    }
  }

  function addStyleRef(asset: any) {
    const cur: string[] = style?.reference_asset_ids_json ?? [];
    if (cur.includes(asset.id)) {
      toast.info('Already a style reference.');
      return;
    }
    patchRefs([...cur, asset.id], `"${asset.name || asset.id}" pinned as style reference.`);
  }

  const removeStyleRef = (id: string) =>
    patchRefs(
      ((style?.reference_asset_ids_json ?? []) as string[]).filter((r) => r !== id),
      'Style reference removed.'
    );

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
    } catch (e: any) {
      toast.error(e.message);
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
    } catch (e: any) {
      toast.error(e.message);
    } finally {
      describeBusy = false;
    }
  }

  let createError=$state('');
  let createOpen=$state(false);
  function changeCreateOpen(value:boolean){if(!value&&(busy||((files?.length||assetType||characterId)&&!window.confirm('Discard this asset draft?'))))return;if(value&&!createOpen)createError='';createOpen=value;if(!value){files=null;assetType='';characterId='';}}
  protectDraft(()=>createOpen&&!!(files?.length||assetType||characterId),'Discard this asset draft and leave?');
  let files: FileList | null = $state(null);
  let assetType = $state('');
  let characterId = $state('');

  let loadSequence=0;
  export async function reload(){await retryMedia();}
  async function refresh() {
    const sequence=++loadSequence;
    try{const [nextAssets,nextCharacters]=await Promise.all([get('/assets'),get('/characters')]);if(sequence!==loadSequence)return;assets=nextAssets;characters=nextCharacters;oncounts?.(characters.length,assets.length);}catch(e){if(sequence===loadSequence)throw e;}
  }
  onMount(() => {
    refresh().then(()=>{const params=new URL(window.location.href).searchParams;reviewScene=params.get('scene_id')??'';const id=params.get('asset_id');if(id){const asset=assets.find((a:any)=>a.id===id);if(asset)openDetail(asset)}})
      .catch((e) => (error = e.message))
      .finally(() => (loaded = true));
    loadStyle();
    loadStoryPresence();
    loadTypes();
  });

  async function doUpload(e: Event) {
    e.preventDefault();
    if (!files?.length || busy) return;
    const problem=uploadProblem(Array.from(files));if(problem){createError=problem;return;}
    busy = true;
    createError = '';
    try {
      const form = new FormData();
      for (const f of Array.from(files)) form.append('file', f);
      if (assetType) form.append('asset_type', assetType);
      if (characterId) form.append('character_id', characterId);
      const r = await upload('/assets/upload', form);
      const n = Array.isArray(r) ? r.length : 1;
      files = null;assetType='';characterId='';createOpen=false;search='';typeFilter='';kindFilter='';sortBy='newest';
      toast.success(`Uploaded ${n} file${n === 1 ? '' : 's'}.`);
      await refresh();
    } catch (e: any) {
      createError=e.message;if(!createOpen)toast.error(e.message);
    } finally {
      busy = false;
    }
  }

  // Map any stored type onto the canonical vocabulary for coherent filtering —
  // legacy/unknown values collapse to "other" (mirrors the backend canonicaliser).
  const canonical = (t: string | null | undefined) =>
    t && canonicalTypes.includes(t) ? t : 'other';

  // Distinct types actually present (canonicalised), for the filter chips.
  const presentTypes = $derived(
    Array.from(new Set(assets.map((a) => canonical(a.type)))).sort(
      (x, y) => canonicalTypes.indexOf(x) - canonicalTypes.indexOf(y)
    )
  );

  let filtered = $derived.by(() => {
    let out = reviewScene?assets.filter((a:any)=>a.metadata_json?.scene_id===reviewScene):assets;
    if (kindFilter) out=out.filter(a=>mediaKind(a)===kindFilter);
    if (search.trim()) {
      const q = search.trim().toLowerCase();
      out = out.filter((a) =>
        [a.name, a.type, a.description, ...(a.tags_json ?? [])]
          .join(' ')
          .toLowerCase()
          .includes(q)
      );
    }
    if (typeFilter) out = out.filter((a) => canonical(a.type) === typeFilter);

    const sorted = out.slice();
    if (sortBy === 'newest')
      sorted.sort((a, b) => (b.created_at ?? '').localeCompare(a.created_at ?? ''));
    else if (sortBy === 'oldest')
      sorted.sort((a, b) => (a.created_at ?? '').localeCompare(b.created_at ?? ''));
    else if (sortBy === 'name')
      sorted.sort((a, b) => (a.name ?? a.id).localeCompare(b.name ?? b.id));
    else if (sortBy === 'type')
      sorted.sort((a, b) => canonical(a.type).localeCompare(canonical(b.type)));
    return sorted;
  });
</script>

<svelte:head>{#if !embedded}<title>Assets · VideoFlow</title>{/if}</svelte:head>

<div class="px-4 py-6 sm:px-8">
  <div class="mb-4 flex items-center justify-between gap-3">
    <div>{#if embedded}<h2 class="text-lg font-semibold">Media</h2>{:else}<h1 class="text-2xl font-semibold">Assets</h1>{/if}<p class="mt-1 text-sm text-muted-foreground">{error?'Collection unavailable':loaded?`${assets.length} ${assets.length===1?'file':'files'}`:'Loading collection…'}</p></div>
    <Dialog.Root open={createOpen} onOpenChange={changeCreateOpen}>
      <Dialog.Trigger>{#snippet child({props})}<Button {...props} disabled={!loaded||busy}>{embedded?'Upload media':'Add assets'}</Button>{/snippet}</Dialog.Trigger>
      <Dialog.Content class="max-h-[90dvh] overflow-y-auto sm:max-w-lg" showCloseButton={false} overlayClass="bg-black/55 supports-backdrop-filter:backdrop-blur-none" onEscapeKeydown={e=>{e.preventDefault();changeCreateOpen(false);}} onInteractOutside={e=>{e.preventDefault();changeCreateOpen(false);}}>
<Dialog.Header><Dialog.Title>{embedded?'Upload media':'Add assets'}</Dialog.Title><Dialog.Description class="sr-only">Upload files and optionally link them to a character.</Dialog.Description></Dialog.Header>
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
        <Button type="button" variant="ghost" size="icon-sm" class="absolute right-2 top-2" aria-label="Close" disabled={busy} onclick={()=>changeCreateOpen(false)}><X class="size-4" /></Button>
      </Dialog.Content>
    </Dialog.Root>
  </div>





  <div class="mb-4 flex flex-wrap items-center gap-2" role="group" aria-label="Media kinds">{#each [{value:'',label:'All files'},{value:'image',label:'Images'},{value:'video',label:'Video'},{value:'audio',label:'Audio'},{value:'other',label:'Other'}] as kind}<Button variant={kindFilter===kind.value?'secondary':'ghost'} size="sm" aria-pressed={kindFilter===kind.value} onclick={()=>kindFilter=kind.value}>{kind.label}</Button>{/each}</div>
  <div class="mb-5 flex flex-wrap items-center gap-3"><input type="search" aria-label="Search media" placeholder="Search files, descriptions or tags…" bind:value={search} class="min-h-11 min-w-0 flex-1 basis-60 rounded-lg border border-input bg-background px-3 text-sm"/>
    <select aria-label="Asset type" bind:value={typeFilter} class="min-h-11 max-w-full rounded-lg border border-input bg-background px-3 text-sm"><option value="">All asset types</option>{#each presentTypes as type}<option value={type}>{typeLabel(type)}</option>{/each}</select>
    <select aria-label="Sort media" bind:value={sortBy} class="min-h-11 rounded-lg border border-input bg-background px-3 text-sm"><option value="newest">Newest first</option><option value="oldest">Oldest first</option><option value="name">Name A–Z</option><option value="type">Asset type</option></select>
    <div role="group" aria-label="Media view" class="flex rounded-lg border border-border p-1"><Button variant={view==='grid'?'secondary':'ghost'} size="icon" class="size-9" aria-label="Grid view" aria-pressed={view==='grid'} onclick={()=>view='grid'}><LayoutGrid class="size-4"/></Button><Button variant={view==='list'?'secondary':'ghost'} size="icon" class="size-9" aria-label="List view" aria-pressed={view==='list'} onclick={()=>view='list'}><List class="size-4"/></Button></div>
  </div>
  {#if error}<div role="alert" class="mb-4 space-y-2 rounded-lg border border-destructive p-4 text-sm"><p>Could not load media: {error}</p>{#if assets.length}<p>Showing the last loaded files.</p>{/if}<Button variant="outline" disabled={refreshing} onclick={retryMedia}>{refreshing?'Loading…':'Try again'}</Button></div>{/if}
  {#if reviewScene}<p class="mb-4 text-sm">References for scene {reviewScene}. <button class="min-h-11 underline" onclick={()=>reviewScene=''}>Show all media</button></p>{/if}
  {#if !loaded}<div class="grid grid-cols-1 gap-5 min-[480px]:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">{#each Array(4) as _}<Skeleton class="aspect-[4/3] w-full rounded-xl"/>{/each}</div>
  {:else if !assets.length&&!error}<div class="py-16 text-center"><Upload class="mx-auto size-9 text-muted-foreground"/><h3 class="mt-4 font-semibold">Build your media library</h3><p class="mx-auto mt-2 max-w-sm text-sm leading-relaxed text-muted-foreground">Upload reference images, video, audio or documents. Generated media will appear here too.</p></div>
  {:else if assets.length&&!filtered.length}<div role="status" class="py-16 text-center"><h3 class="font-semibold">No matching media</h3><p class="mt-2 text-sm text-muted-foreground">Try another search or clear the filters.</p><Button variant="outline" class="mt-4" onclick={()=>{search='';typeFilter='';kindFilter='';}}>Clear filters</Button></div>
  {:else if loaded&&assets.length}<p role="status" class="mb-4 text-xs text-muted-foreground">Showing {filtered.length} of {assets.length} files</p>{/if}
  <div role="region" aria-label="Media collection" class={view==='grid'?'grid grid-cols-1 gap-5 min-[480px]:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4':'flex flex-col gap-3'}>
    {#each filtered as asset (asset.id)}
      {@const name=asset.name||asset.file_path?.split('/').pop()||'Untitled file'}
      {@const kind=mediaKind(asset)}
      <Card class="py-0"><CardContent class={view==='list'?'flex min-w-0 items-center p-0':'p-0'}>
        <button type="button" class={view==='list'?'block w-28 shrink-0 text-left focus-visible:outline-2 focus-visible:outline-ring':'block w-full text-left focus-visible:outline-2 focus-visible:outline-ring'} aria-label={'Preview '+name} onclick={()=>openDetail(asset)}><LibraryPreview {asset} class="aspect-[4/3]"/></button>
        <div class="min-w-0 flex-1 p-4"><div class="flex items-start gap-2"><div class="min-w-0 flex-1"><h3 class="truncate text-sm font-semibold" title={name}>{name}</h3><p class="mt-1 text-xs text-muted-foreground">{typeLabel(kind)}{#if canonical(asset.type)!==kind&&canonical(asset.type)!=='other'} · {typeLabel(canonical(asset.type))}{/if}</p></div>
          <DropdownMenu.Root><DropdownMenu.Trigger class="inline-flex size-9 shrink-0 items-center justify-center rounded-lg hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring" aria-label={'Actions for '+name}><Ellipsis class="size-4"/></DropdownMenu.Trigger><DropdownMenu.Content align="end">
            <DropdownMenu.Item onclick={()=>openDetail(asset)}>Open detail</DropdownMenu.Item><DropdownMenu.Item disabled={busy} onclick={()=>openDescribe(asset)}>Describe with AI</DropdownMenu.Item>{#if isImage(asset.file_path)}<DropdownMenu.Item disabled={busy||!!styleBusy} onclick={()=>addStyleRef(asset)}>Use as style reference</DropdownMenu.Item>{/if}<DropdownMenu.Separator/><DropdownMenu.Item disabled={busy} onclick={()=>deleteTarget=asset}>Delete asset</DropdownMenu.Item>
          </DropdownMenu.Content></DropdownMenu.Root>
        </div>
        {#if asset.description&&view==='grid'}<p class="mt-2 line-clamp-2 text-xs leading-relaxed text-muted-foreground">{asset.description}</p>{/if}
        {#if asset.metadata_json?.consistency_review||asset.tags_json?.length}<div class="mt-3 flex flex-wrap gap-1">{#if asset.metadata_json?.consistency_review}<Badge variant="outline">{asset.metadata_json.consistency_review==='pending'?'Needs review':asset.metadata_json.consistency_review==='approved'?'Approved reference':asset.metadata_json.consistency_review==='rejected'?'Rejected reference':typeLabel(asset.metadata_json.consistency_review)}</Badge>{/if}{#each (asset.tags_json??[]).slice(0,3) as tag}<Badge variant="secondary">{tag}</Badge>{/each}{#if asset.tags_json?.length>3}<span class="px-1 text-xs text-muted-foreground">+{asset.tags_json.length-3}</span>{/if}</div>{/if}
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
            disabled={styleBusy === 'ingest' || !hasStory}
            title={!hasStory
              ? 'Write a script first — the style is derived from your story (先写剧本，再从故事提炼风格)'
              : undefined}
            onclick={ingestStyle}
          >
            <Wand2 class="size-3 mr-1" />{styleBusy === 'ingest' ? 'Deriving…' : 'Ingest from story'}
          </Button>
          <Button variant="outline" size="sm" onclick={() => (styleEditing = true)}>
            Create manually
          </Button>
        </div>
        {#if !hasStory}
          <p class="text-xs text-muted-foreground mt-2">
            Write a script first — the style is derived from your story (先写剧本，再从故事提炼风格).
          </p>
        {/if}
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

          {#if styleRefs.length}
            <div>
              <div class="text-xs text-muted-foreground mb-1">Style reference images</div>
              <div class="flex flex-wrap gap-2">
                {#each styleRefs as ref (ref.id)}
                  <div class="relative w-[72px]">
                    {#if mediaUrl(ref.file_path)}
                      <img class="w-full aspect-square object-cover rounded-md border border-border"
                        src={mediaUrl(ref.file_path)} alt={ref.name} title={ref.name} loading="lazy" />
                    {:else}
                      <div class="w-full aspect-square rounded-md border border-border bg-muted grid place-items-center text-[10px] text-muted-foreground p-1 text-center"
                        title={ref.name}>{ref.name}</div>
                    {/if}
                    <button type="button" aria-label="Remove style reference"
                      class="absolute -top-1.5 -right-1.5 grid size-5 place-items-center rounded-full bg-background border border-border shadow hover:bg-muted"
                      disabled={styleBusy === 'refs'} onclick={() => removeStyleRef(ref.id)}>
                      <X class="size-3" />
                    </button>
                  </div>
                {/each}
              </div>
            </div>
          {:else}
            <p class="text-xs text-muted-foreground">
              No style reference images — pin image assets below with the pin button.
            </p>
          {/if}

          <div class="flex gap-2">
            <Button size="sm" disabled={!!styleBusy} onclick={saveStyle}>
              {styleBusy === 'save' ? 'Saving…' : 'Save'}
            </Button>
            <Button variant="outline" size="sm" disabled={!!styleBusy} onclick={() => (reingestOpen = true)}>
              <Wand2 class="size-3 mr-1" />{styleBusy === 'ingest' ? 'Deriving…' : 'Re-ingest from story'}
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
      <Dialog.Description>{canonical(detail?.type)} · {detail?.id}</Dialog.Description>
    </Dialog.Header>
    {#if detail}
      <div class="grid gap-4 md:grid-cols-[minmax(0,1fr)_260px]">
        <div class="flex items-center justify-center bg-muted rounded-md min-h-[240px]">
          {#if mediaKind(detail)==='video'}
            <!-- svelte-ignore a11y_media_has_caption -->
            <video controls preload="metadata" src={mediaUrl(detail.file_path)}
              class="max-h-[70vh] w-full rounded-md bg-black object-contain"></video>
          {:else if mediaKind(detail)==='audio'}
            {#if mediaUrl(detail.file_path)}
              <audio controls preload="metadata" src={mediaUrl(detail.file_path)} class="w-full"></audio>
            {:else}
              <p class="p-8 text-sm text-muted-foreground">This audio file has no playable local URL.</p>
            {/if}
          {:else if mediaUrl(detail.file_path) && mediaKind(detail)==='image'}
            <img class="max-h-[70vh] w-auto max-w-full rounded-md object-contain"
              src={mediaUrl(detail.file_path)} alt={detail.name || detail.id} />
          {:else}
            <div class="p-8 text-sm text-muted-foreground">This file type cannot be previewed here.</div>
          {/if}
        </div>
        <div class="space-y-3 text-xs">
 {#if detail.metadata_json?.consistency_review}<section aria-label="Reference consistency review"><h3 class="font-semibold">Reference review · {detail.metadata_json.consistency_review}</h3><p class="mt-2 text-muted-foreground">Compare this image with your selected characters, reference assets and locked traits. Approve only when their appearance is consistent. Rejection keeps the image for inspection and prevents reuse.</p>{#if detail.metadata_json.scene_id}<a class="mt-2 inline-block underline" href={`/scenes?scene_id=${encodeURIComponent(detail.metadata_json.scene_id)}`}>Open scene {detail.metadata_json.scene_id}</a>{/if}<div class="mt-3 flex gap-2"><Button size="sm" disabled={reviewBusy||identityBusy||detail.metadata_json.consistency_review==='approved'} onclick={()=>reviewReference(true)}>Approve reference</Button><Button size="sm" variant="outline" disabled={reviewBusy||identityBusy||detail.metadata_json.consistency_review==='rejected'} onclick={()=>reviewReference(false)}>Reject reference</Button></div></section>{/if}
 <section aria-label="Asset identity locks"><h3 class="font-semibold">Identity locks</h3><p class="text-muted-foreground">Keep specific traits across generated scenes, for example kind: car and color: red. Select this asset in Local sessions to use its reference.</p>
 {#each identityTraits as row,i}<div class="mt-2 flex gap-2"><input aria-label={`Trait ${i+1} name`} class="min-w-0 w-1/3 rounded border p-2" bind:value={row.name} placeholder="color"/><input aria-label={`Trait ${i+1} value`} class="min-w-0 flex-1 rounded border p-2" bind:value={row.value} placeholder="red"/><button aria-label={`Remove trait ${i+1}`} onclick={()=>identityTraits=identityTraits.filter((_,n)=>n!==i)}>Remove</button></div>{/each}
 <Button size="sm" variant="outline" disabled={identityBusy||identityTraits.length>=32} onclick={()=>identityTraits=[...identityTraits,{name:'',value:''}]}>Add trait</Button><Button size="sm" disabled={identityBusy} onclick={saveIdentity}>{identityBusy?'Saving…':'Save identity locks'}</Button></section>

          {#if detail.description}
            <div>
              <div class="text-muted-foreground mb-0.5">Description</div>
              <div>{detail.description}</div>
            </div>
          {/if}
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
          <div class="flex flex-wrap gap-1 pt-1">
            <Button variant="outline" size="sm" onclick={() => openDescribe(detail)}>
              <Sparkles class="size-3 mr-1" />Describe with AI
            </Button>
            {#if isImage(detail.file_path)}
              <Button variant="outline" size="sm" disabled={!!styleBusy} onclick={() => addStyleRef(detail)}>
                <Pin class="size-3 mr-1" />Pin as style
              </Button>
            {/if}
            <Button variant="ghost" size="sm" class="text-destructive hover:text-destructive"
              onclick={() => (deleteTarget = detail)}>
              <Trash2 class="size-3 mr-1" />Delete
            </Button>
          </div>
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
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>Re-ingest style from story?</Dialog.Title>
      <Dialog.Description>
        Overwrite the derived style fields from the current story?
        The name and reference images are kept.
      </Dialog.Description>
    </Dialog.Header>
    <Dialog.Footer>
      <Button variant="outline" size="sm" onclick={() => (reingestOpen = false)}>Cancel</Button>
      <Button size="sm" disabled={!!styleBusy} onclick={ingestStyle}>
        <Wand2 class="size-3 mr-1" />Re-ingest
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<Dialog.Root open={deleteTarget !== null} onOpenChange={(open) => !open && (deleteTarget = null)}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>Delete asset?</Dialog.Title>
      <Dialog.Description>
        "{deleteTarget?.name || deleteTarget?.id}" will be deleted and detached from
        scenes, shots, characters and the style guide.
      </Dialog.Description>
    </Dialog.Header>
    <Dialog.Footer>
      <Button variant="outline" size="sm" onclick={() => (deleteTarget = null)}>Cancel</Button>
      <Button variant="destructive" size="sm" disabled={busy} onclick={confirmDeleteAsset}>
        <Trash2 class="size-3 mr-1" />{busy ? 'Deleting…' : 'Delete'}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
