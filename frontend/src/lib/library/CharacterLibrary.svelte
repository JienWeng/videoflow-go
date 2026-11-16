<script lang="ts">
  let { embedded = false,oncounts }:{embedded?:boolean;oncounts?:(characters:number,media:number)=>void}=$props();
  import LibraryPreview from './LibraryPreview.svelte';
  import {characterMatches} from './catalog.mjs';
  import { protectDraft } from '$lib/protect-draft';
  import { REFERENCE_IMAGE_HINT, uploadProblem } from '$lib/upload-limits';
  import { onMount,tick } from 'svelte';
  import { get, post, patch, del, upload, mediaUrl, isImage } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import { Badge } from '$lib/components/ui/badge';
  import { Card, CardContent } from '$lib/components/ui/card';
  import { Skeleton } from '$lib/components/ui/skeleton';
  import { Input } from '$lib/components/ui/input';
  import { Label } from '$lib/components/ui/label';
  import { Textarea } from '$lib/components/ui/textarea';
  import * as Dialog from '$lib/components/ui/dialog';
  import { toast } from 'svelte-sonner';
  import {
    UserPlus,
    BookOpen,
    Images,
    Upload,
    ChevronDown,
    Pencil,
    Trash2,
    Save,
    X,
    ChevronLeft,
    ChevronRight
  } from '@lucide/svelte';

  let search=$state(''),referenceFilter=$state(''),sortBy=$state('name');

  let characters: any[] = $state([]);
  let assets: Record<string, any> = $state({});
  let error = $state('');
  let busy = $state('');
  let loaded = $state(false);
  let loadError = $state('');
  let refreshing = $state(false);
  async function retryCharacters() { refreshing=true; try { await refresh(); loadError=''; } catch(e:any) { loadError=e.message; } finally { refreshing=false; } }
  let expanded: Record<string, boolean> = $state({});

  let createOpen=$state(false);
  function changeCreateOpen(value:boolean){if(!value&&(busy==='create'||((name.trim()||description.trim())&&!window.confirm('Discard this character draft?'))))return;if(value&&!createOpen)error='';createOpen=value;if(!value){name='';description='';}}
  protectDraft(()=>createOpen&&!!(name.trim()||description.trim()),'Discard this character draft and leave?');
  let name = $state('');
  let description = $state('');

  // --- inline edit ---
  let editBaseline='';
  export function hasDraft(){return !!(createOpen&&(name.trim()||description.trim()))||!!(editing&&JSON.stringify(editForm)!==editBaseline);}
  protectDraft(()=>!!editing&&JSON.stringify(editForm)!==editBaseline);
  let editing: string | null = $state(null);
  let editForm = $state({
    name: '',
    description: '',
    appearance: '',
    personality: '',
    visual_rules: '', // one rule per line
    voice_rules: '', // one rule per line
    sample_dialogue: ''
  });

  const filteredCharacters=$derived(characters.filter(c=>editing===c.id&&hasDraft()||characterMatches(c,search)&&(!referenceFilter||(referenceFilter==='with'?(c.reference_asset_ids_json??[]).some((id:string)=>assets[id]&&isImage(assets[id].file_path)):!(c.reference_asset_ids_json??[]).some((id:string)=>assets[id]&&isImage(assets[id].file_path))))).slice().sort((a,b)=>sortBy==='newest'?(b.created_at??'').localeCompare(a.created_at??''):(a.name??'').localeCompare(b.name??'')));

  // --- delete confirm ---
  let deleteTarget: any = $state(null);

  // --- reference-sheet lightbox ---
  let lightbox: { items: any[]; index: number; charName: string } | null = $state(null);

  let loadSequence=0;
  export async function reload(){await retryCharacters();}
  async function refresh() {
    const sequence=++loadSequence;
    try{
    const [nextCharacters,all] = await Promise.all([get('/characters'),get('/assets')]);
    if(sequence!==loadSequence)return;
    characters = nextCharacters;
    assets = Object.fromEntries(all.map((a: any) => [a.id, a]));oncounts?.(characters.length,all.length);
    }catch(e){if(sequence===loadSequence)throw e;}
  }
  onMount(() =>
    refresh().then(() => {
      const id = new URL(window.location.href).searchParams.get('character_id');
      const character = characters.find((c: any) => c.id === id);
      if (character) startEdit(character);
    })
      .catch((e) => (loadError = e.message))
      .finally(() => (loaded = true))
  );

  async function run(key: string, fn: () => Promise<unknown>) {
    busy = key;
    error = '';
    try {
      await fn();
      await refresh();
    } catch (e: any) {
      error = e.message;
      if(key!=='create'||!createOpen)toast.error(e.message);
    } finally {
      busy = '';
    }
  }

  const createCharacter = (e: Event) => {
    e.preventDefault();
    if(!name.trim()||busy)return;
    run('create', async () => {
      await post('/characters', { name:name.trim(), description:description.trim() });
      createOpen=false;search='';referenceFilter='';
      toast.success('Character created');
      name = '';
      description = '';
    });
  };

  function generateBible(c: any) {
    const notes = prompt(`Notes for ${c.name}'s character bible:`, c.description || '');
    if (notes === null) return;
    run(`bible-${c.id}`, () => post(`/characters/${c.id}/bible`, { notes }));
  }

  const generateSheets = (c: any) =>
    run(`sheets-${c.id}`, () => post(`/characters/${c.id}/reference-sheets`, {}));

  function uploadPhoto(c: any, e: Event) {
    const input = e.target as HTMLInputElement;
    if (!input.files?.length) return;
    const files = Array.from(input.files);
    const problem=uploadProblem(files,true);if(problem){toast.error(problem);input.value='';return;}
    run(`photo-${c.id}`, async () => {
      let uploaded = 0;
      try {
        for (const file of files) {
          const form = new FormData();
          form.append('file', file);
          form.append('character_id', c.id);
          await upload('/assets/upload', form);
          uploaded++;
        }
        toast.success(`${uploaded} reference photo${uploaded === 1 ? '' : 's'} uploaded.`);
      } finally {
        // Show completed uploads even when a later file fails.
        if (uploaded) await refresh();
      }
    });
    input.value = '';
  }

  function toggleDetails(id:string){expanded[id]=!expanded[id];void tick().then(()=>document.getElementById((expanded[id]?'character-details-':'character-open-')+id)?.focus());}
  // ---- inline edit ----
  function startEdit(c: any) {
    if(editing&&editing!==c.id&&JSON.stringify(editForm)!==editBaseline&&!window.confirm('Discard the current character edits?'))return;
    editing = c.id;
    expanded[c.id] = true;
    editForm = {
      name: c.name ?? '',
      description: c.description ?? '',
      appearance: c.appearance ?? '',
      personality: c.personality ?? '',
      visual_rules: (c.visual_rules_json ?? []).join('\n'),
      voice_rules: (c.voice_rules_json ?? []).join('\n'),
      sample_dialogue: c.sample_dialogue ?? ''
    };
    editBaseline=JSON.stringify(editForm);
  }

  function cancelEdit() {
    editing = null;
  }

  function saveEdit(c: any) {
    const body = {
      name: editForm.name.trim(),
      description: editForm.description,
      appearance: editForm.appearance,
      personality: editForm.personality,
      visual_rules_json: editForm.visual_rules
        .split('\n')
        .map((s) => s.trim())
        .filter(Boolean),
      voice_rules_json: editForm.voice_rules
        .split('\n')
        .map((s) => s.trim())
        .filter(Boolean),
      sample_dialogue: editForm.sample_dialogue.trim()
    };
    if (!body.name) {
      toast.error('Name cannot be empty.');
      return;
    }
    run(`save-${c.id}`, async () => {
      await patch(`/characters/${c.id}`, body);
      editing = null;
      toast.success('Character saved.');
    });
  }

  // ---- delete ----
  async function confirmDelete() {
    const c = deleteTarget;
    if (!c) return;
    busy = `delete-${c.id}`;
    try {
      const r = await del(`/characters/${c.id}`);
      deleteTarget = null;
      toast.success(
        `Character deleted (detached from ${r.detached_from} place${r.detached_from === 1 ? '' : 's'}; reference images kept).`
      );
      await refresh();
    } catch (e: any) {
      toast.error(e.message);
    } finally {
      busy = '';
    }
  }

  // ---- lightbox ----
  function openLightbox(c: any, startIndex = 0) {
    const items = (c.reference_asset_ids_json ?? [])
      .map((id: string) => assets[id])
      .filter((a: any) => a && isImage(a.file_path));
    if (!items.length) return;
    lightbox = { items, index: Math.min(startIndex, items.length - 1), charName: c.name };
  }
  function lightboxStep(delta: number) {
    if (!lightbox) return;
    const n = lightbox.items.length;
    lightbox.index = (lightbox.index + delta + n) % n;
  }
  function onLightboxKey(e: KeyboardEvent) {
    if (!lightbox) return;
    if (e.key === 'ArrowRight') lightboxStep(1);
    else if (e.key === 'ArrowLeft') lightboxStep(-1);
    else if (e.key === 'Escape') lightbox = null;
  }
</script>

<svelte:head>{#if !embedded}<title>Characters · VideoFlow</title>{/if}</svelte:head>

<svelte:window onkeydown={onLightboxKey} />

<div class="px-4 py-6 sm:px-8">
  <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
    <div>{#if embedded}<h2 class="text-lg font-semibold">Characters</h2>{:else}<h1 class="text-2xl font-semibold">Characters</h1>{/if}<p class="mt-1 text-sm text-muted-foreground">{loadError?'Collection unavailable':loaded?`${characters.length} ${characters.length===1?'character':'characters'}`:'Loading collection…'}</p></div>
    <Dialog.Root open={createOpen} onOpenChange={changeCreateOpen}>
      <Dialog.Trigger>{#snippet child({props})}<Button {...props} disabled={!loaded||!!loadError}>Create character</Button>{/snippet}</Dialog.Trigger>
      <Dialog.Content class="max-h-[90dvh] overflow-y-auto sm:max-w-md" showCloseButton={false} overlayClass="bg-black/55 supports-backdrop-filter:backdrop-blur-none" onEscapeKeydown={e=>{e.preventDefault();changeCreateOpen(false);}} onInteractOutside={e=>{e.preventDefault();changeCreateOpen(false);}}>
<Dialog.Header><Dialog.Title>Create character</Dialog.Title><Dialog.Description class="sr-only">Add a named character to this project.</Dialog.Description></Dialog.Header>
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
        <Button type="button" variant="ghost" size="icon-sm" class="absolute right-2 top-2" aria-label="Close" disabled={busy==='create'} onclick={()=>changeCreateOpen(false)}><X class="size-4" /></Button>
      </Dialog.Content>
    </Dialog.Root>
  </div>

  <div class="mb-6 flex flex-wrap gap-3" aria-label="Character filters"><input type="search" aria-label="Search characters" placeholder="Search characters…" bind:value={search} class="min-h-11 min-w-0 flex-1 basis-60 rounded-lg border border-input bg-background px-3 text-sm"/><select aria-label="Character references" bind:value={referenceFilter} class="min-h-11 max-w-full rounded-lg border border-input bg-background px-3 text-sm"><option value="">All characters</option><option value="with">With reference images</option><option value="missing">Needs reference images</option></select><select aria-label="Sort characters" bind:value={sortBy} class="min-h-11 rounded-lg border border-input bg-background px-3 text-sm"><option value="name">Name A–Z</option><option value="newest">Newest first</option></select></div>
  {#if loaded&&characters.length&&!filteredCharacters.length}<div role="status" class="py-16 text-center"><p class="font-medium">No matching characters</p><p class="mt-2 text-sm text-muted-foreground">Try another name or clear the filters.</p><Button variant="outline" class="mt-4" onclick={()=>{search='';referenceFilter='';}}>Clear filters</Button></div>{/if}
  {#if loadError}<div role="alert" class="mb-4 space-y-2 rounded-lg border border-destructive p-4 text-sm"><p>Could not load characters: {loadError}</p>{#if characters.length}<p>Showing the last loaded characters.</p>{/if}<Button variant="outline" disabled={refreshing} onclick={retryCharacters}>{refreshing ? 'Loading…' : 'Try again'}</Button></div>{/if}

  {#if loaded&&!loadError&&!characters.length}<div class="py-16 text-center"><Images class="mx-auto size-9 text-muted-foreground"/><h3 class="mt-4 font-semibold">Add your first character</h3><p class="mt-2 text-sm text-muted-foreground">Create a character, then add reference images to keep their appearance consistent.</p></div>{/if}

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
      {@const refs=(c.reference_asset_ids_json??[]).map((id:string)=>assets[id]).filter((a:any)=>a&&isImage(a.file_path))}
      <Card class="self-start py-0 {expanded[c.id]?'col-span-full':''}">
        <CardContent class="p-0">
          {#if !expanded[c.id]}<button type="button" class="block w-full text-left focus-visible:outline-2 focus-visible:outline-ring" id={'character-open-'+c.id} aria-label={'Open '+c.name} onclick={()=>toggleDetails(c.id)}><LibraryPreview portrait asset={refs[0]??null} class="aspect-[4/3]"/></button>{/if}
          <div class="p-4"><div class="flex items-start gap-3">
            {#if expanded[c.id]}<LibraryPreview portrait asset={refs[0]??null} class="size-16 shrink-0 rounded-lg"/>{/if}
            <div class="min-w-0 flex-1"><h3 class="truncate text-base font-semibold">{c.name}</h3><p class="mt-1 line-clamp-2 min-h-8 text-xs leading-relaxed text-muted-foreground">{c.description||c.appearance||'Add a description to define this character.'}</p><p class="mt-3 text-xs text-muted-foreground">{refs.length?`${refs.length} reference ${refs.length===1?'image':'images'}`:'Needs reference images'}</p></div>
            <Button variant="ghost" size="sm" class="shrink-0 px-2" id={'character-details-'+c.id} aria-expanded={!!expanded[c.id]} onclick={()=>toggleDetails(c.id)}><ChevronDown class="size-4 {expanded[c.id]?'rotate-180':''}"/><span>Details</span></Button>
          </div>

          {#if expanded[c.id]}
            <div class="mt-3 space-y-2 border-t border-border pt-3">
              <Button variant="outline" href={`/voices?character_id=${encodeURIComponent(c.id)}`}>Voice & dialogue</Button>
              {#if editing === c.id}
                <!-- ---- inline edit form ---- -->
                <div class="grid gap-2">
                  <div class="grid gap-1.5">
                    <Label for="edit-name-{c.id}">Name</Label>
                    <Input id="edit-name-{c.id}" bind:value={editForm.name} />
                  </div>
                  <div class="grid gap-1.5">
                    <Label for="edit-desc-{c.id}">Description</Label>
                    <Input id="edit-desc-{c.id}" bind:value={editForm.description} />
                  </div>
                  <div class="grid gap-1.5">
                    <Label for="edit-appear-{c.id}">Appearance</Label>
                    <Textarea id="edit-appear-{c.id}" rows={2} bind:value={editForm.appearance} />
                  </div>
                  <div class="grid gap-1.5">
                    <Label for="edit-pers-{c.id}">Personality</Label>
                    <Textarea id="edit-pers-{c.id}" rows={2} bind:value={editForm.personality} />
                  </div>
                  <div class="grid gap-1.5">
                    <Label for="edit-rules-{c.id}">Visual rules (one per line)</Label>
                    <Textarea id="edit-rules-{c.id}" rows={3} bind:value={editForm.visual_rules}
                      placeholder="always wears red hoodie&#10;short black hair" />
                  </div>
                  <div class="grid gap-1.5">
                    <Label for="edit-voice-{c.id}">Voice rules (one per line)</Label>
                    <Textarea id="edit-voice-{c.id}" rows={3} bind:value={editForm.voice_rules}
                      placeholder="cheerful high-pitched voice&#10;speaks quickly" />
                  </div>
                  <div class="grid gap-1.5">
                    <Label for="edit-sample-dialogue-{c.id}">Sample dialogue</Label>
                    <Textarea id="edit-sample-dialogue-{c.id}" rows={2} bind:value={editForm.sample_dialogue}
                      placeholder="A short line that sounds like this character" />
                  </div>
                  <div class="flex gap-2 pt-1">
                    <Button size="sm" disabled={busy === `save-${c.id}`} onclick={() => saveEdit(c)}>
                      <Save class="size-3 mr-1" />{busy === `save-${c.id}` ? 'Saving…' : 'Save'}
                    </Button>
                    <Button variant="outline" size="sm" disabled={!!busy} onclick={cancelEdit}>
                      <X class="size-3 mr-1" />Cancel
                    </Button>
                  </div>
                </div>
              {:else}
                <!-- ---- read view ---- -->
                <div class="text-xs text-muted-foreground">{c.id} · {c.reference_asset_ids_json?.length ?? 0} reference images</div>
                {#if c.appearance}
                  <div class="text-xs"><span class="text-muted-foreground">Appearance:</span> {c.appearance}</div>
                {/if}
                {#if c.personality}
                  <div class="text-xs"><span class="text-muted-foreground">Personality:</span> {c.personality}</div>
                {/if}
                {#if c.sample_dialogue}
                  <div class="text-xs"><span class="text-muted-foreground">Sample dialogue:</span> 「{c.sample_dialogue}」</div>
                {/if}
                {#if c.visual_rules_json?.length}
                  <div class="flex flex-wrap gap-1">
                    {#each c.visual_rules_json as rule}
                      <Badge variant="outline" class="max-w-full" title={rule}>
                        <span class="truncate">{rule}</span>
                      </Badge>
                    {/each}
                  </div>
                {/if}
                {#if c.reference_asset_ids_json?.length}
                  <div class="grid grid-cols-3 gap-2">
                    {#each c.reference_asset_ids_json as refId, i (refId)}
                      {@const ref = assets[refId]}
                      {#if ref && isImage(ref.file_path)}
                        <button type="button" class="block" title="View {ref.name || 'reference'}"
                          onclick={() => openLightbox(c, i)}>
                          <img class="w-full aspect-square object-cover rounded-md bg-muted cursor-zoom-in"
                            src={mediaUrl(ref.file_path)} alt={ref.name || c.name} loading="lazy" />
                        </button>
                      {/if}
                    {/each}
                  </div>
                {/if}

                <div class="flex flex-wrap gap-1 pt-1">
                  <Button variant="outline" size="sm" disabled={!!busy} onclick={() => startEdit(c)}>
                    <Pencil class="size-3 mr-1" />Edit
                  </Button>
                  <Button variant="outline" size="sm" disabled={!!busy} onclick={() => generateBible(c)}>
                    <BookOpen class="size-3 mr-1" />{busy === `bible-${c.id}` ? 'Generating…' : 'Generate bible'}
                  </Button>
                  <Button variant="outline" size="sm" disabled={!!busy} onclick={() => generateSheets(c)}>
                    <Images class="size-3 mr-1" />{busy === `sheets-${c.id}` ? 'Generating…' : 'Generate reference sheets'}
                  </Button>
                  <label class="inline-flex items-center gap-1 cursor-pointer rounded-md border border-border px-2 py-1 text-xs hover:bg-accent">
                    <Upload class="size-3" />{busy === `photo-${c.id}` ? 'Uploading…' : 'Upload reference photo'}
                    <input type="file" aria-describedby={`reference-limit-${c.id}`} class="hidden" multiple disabled={!!busy} accept=".jpg,.jpeg,.png,.gif,.webp" onchange={(e) => uploadPhoto(c, e)} />
                  </label>
                  <p id={`reference-limit-${c.id}`} class="text-xs text-muted-foreground">{REFERENCE_IMAGE_HINT}</p>
                  <Button variant="ghost" size="sm" class="text-destructive hover:text-destructive"
                    disabled={!!busy} onclick={() => (deleteTarget = c)}>
                    <Trash2 class="size-3 mr-1" />Delete
                  </Button>
                </div>
              {/if}
            </div>
          {/if}
          </div>
        </CardContent>
      </Card>
    {/each}
  </div>
</div>

<!-- Delete confirm dialog (app confirm pattern) -->
<Dialog.Root open={deleteTarget !== null} onOpenChange={(open) => !open && (deleteTarget = null)}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>Delete character?</Dialog.Title>
      <Dialog.Description>
        "{deleteTarget?.name}" will be removed and detached from every scene cast.
        Its reference images stay in the asset library.
      </Dialog.Description>
    </Dialog.Header>
    <Dialog.Footer>
      <Button variant="outline" size="sm" onclick={() => (deleteTarget = null)}>Cancel</Button>
      <Button variant="destructive" size="sm" disabled={!!busy} onclick={confirmDelete}>
        <Trash2 class="size-3 mr-1" />{busy === `delete-${deleteTarget?.id}` ? 'Deleting…' : 'Delete'}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<!-- Reference-sheet lightbox -->
<Dialog.Root open={lightbox !== null} onOpenChange={(open) => !open && (lightbox = null)}>
  <Dialog.Content class="max-w-3xl sm:max-w-3xl">
    <Dialog.Header>
      <Dialog.Title class="truncate">
        {lightbox?.charName} — reference {(lightbox?.index ?? 0) + 1} / {lightbox?.items.length ?? 0}
      </Dialog.Title>
      {#if lightbox?.items[lightbox.index]?.name}
        <Dialog.Description class="truncate">{lightbox.items[lightbox.index].name}</Dialog.Description>
      {/if}
    </Dialog.Header>
    {#if lightbox}
      <div class="relative flex items-center justify-center bg-muted rounded-md">
        <img class="max-h-[70vh] w-auto max-w-full rounded-md object-contain"
          src={mediaUrl(lightbox.items[lightbox.index].file_path)}
          alt={lightbox.items[lightbox.index].name || lightbox.charName} />
        {#if lightbox.items.length > 1}
          <button type="button" aria-label="Previous"
            class="absolute left-2 grid size-9 place-items-center rounded-full bg-background/80 border border-border shadow hover:bg-background"
            onclick={() => lightboxStep(-1)}>
            <ChevronLeft class="size-4" />
          </button>
          <button type="button" aria-label="Next"
            class="absolute right-2 grid size-9 place-items-center rounded-full bg-background/80 border border-border shadow hover:bg-background"
            onclick={() => lightboxStep(1)}>
            <ChevronRight class="size-4" />
          </button>
        {/if}
      </div>
    {/if}
  </Dialog.Content>
</Dialog.Root>
