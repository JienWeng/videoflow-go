<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { get, post, del, mediaUrl } from '$lib/api';
  import { subscribeJobs } from '$lib/sse';
  import { Button } from '$lib/components/ui/button';
  import { Images, Plus, X, Check, TriangleAlert } from '@lucide/svelte';
  import type { Asset, Character, SceneReferenceState } from '$lib/api-types';
  let {
    sceneId,
    characters = [],
    onchanged,
    onstate
  }: {
    sceneId: string;
    characters?: Character[];
    onchanged?: () => void | Promise<void>;
    onstate?: (value: SceneReferenceState) => void;
  } = $props();
  let referenceState = $state<SceneReferenceState | null>(null),
    library: Asset[] = $state([]),
    error = $state(''),
    loading = $state(true),
    busy = $state(false),
    picker = $state(false),
    query = $state('');
  let alive = true,
    sequence = 0;
  onDestroy(() => {
    alive = false;
    sequence++;
  });
  async function load() {
    const request = ++sequence;
    try {
      const [next, assets] = await Promise.all([
        get<SceneReferenceState>(`/scenes/${sceneId}/references`),
        get<Asset[]>('/assets')
      ]);
      if (!alive || request !== sequence) return;
      referenceState = next;
      onstate?.(next);
      library = assets;
      error = '';
    } catch (e) {
      if (alive && request === sequence) error = String(e);
    } finally {
      if (alive && request === sequence) loading = false;
    }
  }
  export async function reload() {
    await load();
  }
  onMount(() => {
    void load();
    const focus = () => {
      if (!busy) void load();
    };
    window.addEventListener('focus', focus);
    const unsubscribe = subscribeJobs(e => {
      if (!busy && (!e.scene_id || e.scene_id === sceneId)) void load();
    });
    return () => {
      window.removeEventListener('focus', focus);
      unsubscribe();
    };
  });
  const selected = $derived(new Set<string>(referenceState?.asset_ids ?? []));
  const candidates = $derived(
    library.filter(
      a =>
        ['prop', 'location'].includes(a.type) &&
        !selected.has(a.id) &&
        a.metadata_json?.consistency_review !== 'rejected' &&
        [a.name, a.description].join(' ').toLowerCase().includes(query.trim().toLowerCase())
    )
  );
  const board = $derived(
    referenceState?.storyboards?.find(a => a.metadata_json?.consistency_review !== 'rejected')
  );
  const currentBoard = $derived(
    referenceState?.storyboards?.find(
      a =>
        a.metadata_json?.consistency_review !== 'rejected' &&
        (!referenceState?.context_revision ||
          a.metadata_json?.director_context_revision === referenceState.context_revision)
    )
  );
  const group = (asset: Asset) => asset.metadata_json?.variant_group_id ?? asset.id;
  const replacement = (asset: Asset) =>
    referenceState?.references?.find(a => selected.has(a.id) && group(a) === group(asset))?.id;
  async function change(id: string, attach: boolean, replace?: string) {
    if (busy || !referenceState) return;
    busy = true;
    error = '';
    try {
      const body = { expected_updated_at: referenceState.updated_at, replace_asset_id: replace ?? '' };
      const next = attach
        ? await post<SceneReferenceState>(`/scenes/${sceneId}/references/${id}`, body)
        : await del<SceneReferenceState>(`/scenes/${sceneId}/references/${id}`, body);
      if (!alive) return;
      referenceState = next;
      onstate?.(next);
      await onchanged?.();
    } catch (e) {
      if (alive) {
        error = String(e);
        await load();
        error = String(e);
      }
    } finally {
      if (alive) busy = false;
    }
  }
</script>
<details class="mt-5 border-t border-border" open>
 <summary
   class="min-h-11 cursor-pointer py-3 text-sm font-semibold">Scene references{#if referenceState?.references?.length}<span
   class="ml-2 font-normal text-muted-foreground">{referenceState.references.length}</span>{/if}</summary>
 <section aria-label="Scene reference workspace" class="space-y-3 pb-4 text-sm">
 {#if loading}<p class="text-muted-foreground" role="status">Loading scene references…</p>{/if}
 {#if error}<div role="alert"><p class="break-words">{error}</p><Button
   variant="outline"
   size="sm"
   disabled={busy}
   onclick={load}>Reload references</Button></div>{/if}
 {#if referenceState}
 <p class="max-w-prose text-xs leading-relaxed text-muted-foreground">Shared from your project Library. Characters follow this scene’s cast; props and locations are selected here. Images are usable once saved unless excluded.</p>
 {#if referenceState.missing_character_ids?.length}<p role="status"
   class="text-xs">Add reference images for {#each referenceState.missing_character_ids as id,i}{#if i}, {/if}<a
   class="underline underline-offset-4"
   href={`/characters/${encodeURIComponent(id)}`}>{characters.find(c=>c.id===id)?.name??'this character'}</a>{/each}.</p>{/if}
 {#if referenceState.missing_asset_ids?.length}<div role="alert"
   class="text-xs"><p>Some saved references are unavailable. Remove missing scene references or repair the character’s images in Library.</p>{#each referenceState.missing_asset_ids as id}{#if selected.has(id)}<Button
     variant="ghost"
     size="sm"
     disabled={busy}
     onclick={()=>change(id,false)}>Remove missing reference</Button>{/if}{/each}</div>{/if}
 {#if referenceState.references?.length}
 <ul
   class="divide-y divide-border">{#each referenceState.references as asset (asset.id)}{@const approved=asset.metadata_json?.consistency_review!=='rejected'}<li
   class="flex flex-wrap items-center gap-3 py-3"><a
   class="flex min-h-11 min-w-0 flex-1 basis-44 items-center gap-3 underline underline-offset-4"
   href={`/library?tab=media&asset_id=${encodeURIComponent(asset.id)}&scene_id=${encodeURIComponent(sceneId)}`}><img
   class="size-12 shrink-0 rounded-md object-cover"
   src={mediaUrl(asset.file_path)??''}
   alt=""/><span class="min-w-0 break-words">{asset.name}<span
     class="block text-xs text-muted-foreground no-underline">{asset.type==='character_reference'?'Character reference':asset.type}</span></span></a><span
     class="text-xs text-muted-foreground">{approved?'Ready':'Excluded'}</span>{#if selected.has(asset.id)}<Button
   variant="ghost"
   size="sm"
   disabled={busy}
   onclick={()=>change(asset.id,false)}
   aria-label={`Remove ${asset.name} from scene`}>Remove</Button>{:else}<span
     class="text-xs text-muted-foreground">From cast</span>{/if}{#if Object.keys(asset.metadata_json?.locked_traits??{}).length}<p
     class="basis-full text-xs text-muted-foreground">Locked: {Object.entries(asset.metadata_json?.locked_traits ?? {})
         .map(([key, value]) => `${key}: ${value}`)
         .join(' · ')}</p>{/if}</li>{/each}</ul>
 {:else if !loading}<p class="text-xs text-muted-foreground">No references selected yet.</p>{/if}
 <div class="flex flex-wrap items-center gap-2"><Button
   variant="outline"
   size="sm"
   disabled={busy}
   aria-expanded={picker}
   onclick={()=>picker=!picker}><Plus class="mr-1 size-3"/>Choose from Library</Button><Button
   variant="ghost"
   size="sm"
   href={`/library?tab=media&scene_id=${encodeURIComponent(sceneId)}`}>View all scene media</Button></div>
 {#if picker}<div class="space-y-2 border-t border-border pt-3"><label
   for={`reference-search-${sceneId}`}
   class="block text-xs font-medium">Find a prop or location</label><input
   id={`reference-search-${sceneId}`}
   class="w-full rounded-md border border-input bg-background px-3 py-2"
   placeholder="Search project Library…"
   bind:value={query}/><ul
     class="max-h-64 divide-y divide-border overflow-y-auto">{#each candidates as asset (asset.id)}<li
     class="flex items-center justify-between gap-3 py-2"><span class="min-w-0 break-words">{asset.name}<span
     class="block text-xs text-muted-foreground">{asset.type} · {'Available'}</span></span><Button
   variant="outline"
   size="sm"
   disabled={busy}
   onclick={()=>change(asset.id,true,replacement(asset))}
   aria-label={`${replacement(asset)?'Replace with':'Use'} ${asset.name} in scene`}>{replacement(asset)?'Replace in scene':'Use in scene'}</Button></li>{:else}<li
     class="py-3 text-xs text-muted-foreground">No matching unused props or locations. Upload or generate one in Library or the Assets stage.</li>{/each}</ul></div>{/if}

 {#if referenceState.storyboards?.length}<p
   class="flex items-start gap-2 text-xs text-muted-foreground"
   role="status">{#if referenceState.storyboard_ready}<Check class="size-4 shrink-0"/>Storyboard matches the current references. <a
   class="underline underline-offset-4"
   href={`/library?tab=media&asset_id=${encodeURIComponent(referenceState.active_storyboard_id)}&scene_id=${encodeURIComponent(sceneId)}`}>View render storyboard</a>{:else}<TriangleAlert
     class="size-4 shrink-0"/>{board
         ? 'This scene changed since its storyboard was made. Rendering again updates the storyboard first; your current video stays.'
         : 'A storyboard is made automatically when you render.'}{/if}</p>{/if}
 {/if}
 </section>
</details>
