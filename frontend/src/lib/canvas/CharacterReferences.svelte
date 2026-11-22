<script lang="ts">
  import { onDestroy } from 'svelte';
  import { RefreshCw } from '@lucide/svelte';
  import * as Dialog from '$lib/components/ui/dialog';
  import { get, post, mediaUrl, isImage } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import LibraryPreview from '$lib/library/LibraryPreview.svelte';
  import { runBackgroundOp } from '$lib/ops';
  let {
    characterId,
    disabled = false,
    onchanged
  }: { characterId: string; disabled?: boolean; onchanged: () => void } = $props();
  let character: any = $state(null),
    assets: any[] = $state([]),
    loading = $state(true),
    error = $state(''),
    notice = $state(''),
    generatingFor = $state('');
  let sequence = 0;
  const scope = new AbortController();
  onDestroy(() => {
    sequence++;
    scope.abort();
  });
  const active = $derived(assets.filter(a => character?.reference_asset_ids_json?.includes(a.id)));
  const previous = $derived(
    assets.filter(
      a =>
        a.character_id === characterId &&
        a.type === 'character_reference' &&
        !character?.reference_asset_ids_json?.includes(a.id)
    )
  );
  async function refresh(id = characterId) {
    if (scope.signal.aborted) return;
    const token = ++sequence;
    loading = true;
    error = '';
    try {
      const [c, all] = await Promise.all([get(`/characters/${id}`), get('/assets')]);
      if (scope.signal.aborted || token !== sequence || characterId !== id) return;
      character = c;
      assets = all.filter(
        (a: any) => isImage(a.file_path) && (a.character_id === id || c.reference_asset_ids_json?.includes(a.id))
      );
    } catch (e) {
      if (token === sequence) error = String(e);
    } finally {
      if (token === sequence) loading = false;
    }
  }
  $effect(() => {
    const id = characterId;
    character = null;
    assets = [];
    notice = '';
    void refresh(id);
  });
  async function regenerate() {
    if (disabled || loading || generatingFor) return;
    const id = characterId;
    generatingFor = id;
    notice = '';
    error = '';
    try {
      await runBackgroundOp(
        `/characters/${id}/reference-sheets`,
        {},
        {
          signal: scope.signal,
          label: 'Character reference',
          onDone: async () => {
            if (scope.signal.aborted) return;
            generatingFor = '';
            if (characterId === id) {
              await refresh(id);
              if (!scope.signal.aborted && characterId === id) notice = 'New reference saved and ready to use.';
            }
            if (!scope.signal.aborted) onchanged();
          },
          onFail: op => {
            if (scope.signal.aborted) return;
            generatingFor = '';
            if (characterId === id)
              error = op.error ?? 'Reference generation needs attention. Check Activity before retrying.';
          }
        }
      );
    } catch (e) {
      if (scope.signal.aborted) return;
      generatingFor = '';
      if (characterId === id) error = String(e);
    }
  }
  const reviewLabel = (a: any) => (a.metadata_json?.consistency_review === 'rejected' ? 'Excluded' : 'Ready to use');
  let fullImage: any = $state(null),
    reviewing = $state(false);
  async function reviewFull(approved: boolean) {
    if (!fullImage || reviewing) return;
    reviewing = true;
    try {
      fullImage = await post(`/assets/${fullImage.id}/review`, { approved, expected_updated_at: fullImage.updated_at });
      await refresh();
      await onchanged();
      notice = approved ? 'Image available for reuse.' : 'Image excluded from generation.';
    } catch (e) {
      error = String(e);
    } finally {
      reviewing = false;
    }
  }
</script>
<section
  id="np-character-references"
  tabindex="-1"
  aria-label="Character reference images"
  class="space-y-3 border-t border-border pt-4 focus-visible:outline-2 focus-visible:outline-ring">
  <div class="flex flex-wrap items-center justify-between gap-2"><h3 class="font-semibold">Reference images</h3><Button
    variant="outline"
    size="sm"
    disabled={disabled||loading||!!generatingFor}
    onclick={regenerate}><RefreshCw
      class="size-3.5 {generatingFor===characterId?'motion-safe:animate-spin':''}"/>{generatingFor === characterId
          ? 'Generating…'
          : active.length
            ? 'Regenerate reference image'
            : 'Generate reference image'}</Button></div>
  <p class="text-xs leading-relaxed text-muted-foreground">Uses saved character details and existing references. Saves a new version; image provider charges may apply.</p>
  {#if disabled}<p class="text-xs text-muted-foreground">Save or discard character changes before generating.</p>{/if}
  {#if loading}<p role="status"
    class="text-sm text-muted-foreground">Loading references…</p>{:else if error}<div role="alert"
    class="space-y-2 text-sm"><p class="break-words text-destructive">{error}</p><Button variant="ghost"
    size="sm" onclick={()=>refresh()}>Refresh references</Button></div>{/if}
  {#if notice}<p role="status" class="text-sm">{notice}</p>{/if}
  {#if generatingFor&&generatingFor!==characterId}<p role="status"
    class="text-xs text-muted-foreground">Another character reference is generating. Check Activity for its status.</p>{/if}
  {#if generatingFor===characterId}<p role="status"
    class="text-xs text-muted-foreground">Generating in the background. You can keep working; check Activity if you leave this view.</p>{/if}
  {#if !loading&&!active.length&&!error}<p class="py-2 text-sm text-muted-foreground">No reference images yet.</p>{/if}
  <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">{#each active as asset}<button
    type="button"
    onclick={()=>{error='';fullImage=asset}}
    class="min-w-0 rounded-lg border border-border p-2 text-left focus-visible:outline-2 focus-visible:outline-ring hover:bg-accent"
    aria-label={`View full ${asset.name}`}><LibraryPreview
    portrait
    {asset}
    class="aspect-square rounded-md"/><p class="mt-2 truncate text-xs">{asset.name}</p><p
      class="mt-1 text-xs text-muted-foreground">{reviewLabel(asset)}</p></button>{/each}</div>
  {#if previous.length}<details class="border-t border-border pt-2"><summary
    class="min-h-11 cursor-pointer py-3 text-sm">Previous versions ({previous.length})</summary><p
    class="mb-3 text-xs leading-relaxed text-muted-foreground">Kept in Library. These images are not active references for the character.</p><div
    class="grid grid-cols-2 gap-3">{#each previous as asset}<a
    href={`/library?tab=media&asset_id=${encodeURIComponent(asset.id)}`}
    class="rounded-lg border border-border p-2 focus-visible:outline-2 focus-visible:outline-ring"
    aria-label={`View previous ${asset.name}`}><LibraryPreview
    portrait
    {asset}
    class="aspect-square rounded-md"/><p class="mt-2 truncate text-xs">{asset.name}</p><p
      class="mt-1 text-xs text-muted-foreground">{reviewLabel(asset)}</p></a>{/each}</div></details>{/if}
  <a
    href={`/characters/${encodeURIComponent(characterId)}#reference-photos`}
    class="inline-flex min-h-11 items-center text-xs underline underline-offset-4">Manage reference photos</a>
</section>

<Dialog.Root
  open={!!fullImage}
  onOpenChange={open=>{if(!open&&!reviewing)fullImage=null}}><Dialog.Content
  class="max-h-[92dvh] overflow-y-auto sm:max-w-4xl"
  onEscapeKeydown={e=>{if(reviewing)e.preventDefault()}}
  onInteractOutside={e=>{if(reviewing)e.preventDefault()}}><Dialog.Header><Dialog.Title>{fullImage?.name??'Character reference'}</Dialog.Title><Dialog.Description>View the complete character reference image.</Dialog.Description></Dialog.Header>{#if error}<p
    role="alert" class="text-sm text-destructive">{error}</p>{/if}
{#if fullImage}<img
  class="max-h-[65dvh] w-full rounded-md object-contain"
  src={mediaUrl(fullImage.file_path)??''}
  alt={fullImage.name}/><p class="text-sm">{reviewLabel(fullImage)}</p><details><summary class="cursor-pointer py-2 text-sm">Image options</summary><Button
  variant="outline"
  disabled={reviewing||disabled}
  onclick={()=>reviewFull(fullImage.metadata_json?.consistency_review==='rejected')}>{fullImage.metadata_json?.consistency_review==='rejected'?'Allow reuse':'Exclude from generation'}</Button></details>{/if}</Dialog.Content></Dialog.Root>
