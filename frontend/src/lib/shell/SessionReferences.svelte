<script lang="ts">
  // Characters and reference pictures the chosen conversation uses in its next prompts.
  import {get,put} from '$lib/api';
  import {Button} from '$lib/components/ui/button';
  import * as Dialog from '$lib/components/ui/dialog';
  let {open=$bindable(false),session}:{open?:boolean;session:{id:string;title?:string;status?:string}}=$props();
  let characters:any[]=$state([]),assets:any[]=$state([]),characterIDs:string[]=$state([]),assetIDs:string[]=$state([]);
  let saved=$state(''),loading=$state(false),busy=$state(false),error=$state(''),notice=$state(''),warnings:string[]=$state([]);
  const key=(cast:string[],media:string[])=>JSON.stringify([[...cast].sort(),[...media].sort()]);
  const dirty=$derived(!loading&&key(characterIDs,assetIDs)!==saved);
  const running=$derived(session.status==='running');
  let generation=0;
  async function load(id:string){
    const current=++generation;loading=true;error='';notice='';warnings=[];
    try{
      const [selection,cast,media]=await Promise.all([get(`/director/sessions/${encodeURIComponent(id)}/selection`),get('/characters'),get('/assets')]);
      if(current!==generation)return;
      characters=cast??[];assets=media??[];characterIDs=selection?.character_ids??[];assetIDs=selection?.asset_ids??[];saved=key(characterIDs,assetIDs);
    }catch(e){if(current===generation)error=String(e)}finally{if(current===generation)loading=false}
  }
  const sessionId=$derived(session.id);
  $effect(()=>{if(open)void load(sessionId)});
  async function save(){
    if(busy||loading)return;busy=true;error='';notice='';
    try{const snapshot=await put(`/director/sessions/${encodeURIComponent(session.id)}/selection`,{character_ids:characterIDs,asset_ids:assetIDs});saved=key(characterIDs,assetIDs);warnings=snapshot?.warnings??[];notice='References saved for your next prompt.';}
    catch(e){error=String(e)}finally{busy=false}
  }
  function changeOpen(value:boolean){if(busy)return;if(!value&&dirty&&!window.confirm('Close without saving your reference changes?'))return;open=value;if(!value)generation++;}
</script>
<Dialog.Root {open} onOpenChange={changeOpen}>
  <Dialog.Content class="flex max-h-[85dvh] flex-col overflow-hidden sm:max-w-2xl" onEscapeKeydown={e=>{e.preventDefault();changeOpen(false)}} onInteractOutside={e=>{e.preventDefault();changeOpen(false)}}>
    <Dialog.Header><Dialog.Title>Characters &amp; references</Dialog.Title><Dialog.Description>Choose what “{session.title||'Untitled'}” uses in future prompts.</Dialog.Description></Dialog.Header>
    <div class="min-h-0 flex-1 space-y-4 overflow-y-auto py-2" aria-busy={loading}>
      {#if error}<div role="alert" class="space-y-2 text-sm"><p class="text-destructive">{error}</p>{#if !saved}<Button variant="outline" size="sm" onclick={()=>load(session.id)}>Retry</Button>{/if}</div>{/if}
      {#if loading}<p role="status" class="text-sm text-muted-foreground">Loading references…</p>
      {:else if saved}
        <div class="grid gap-6 sm:grid-cols-2">
          <fieldset class="min-w-0 space-y-1" disabled={busy||running}><legend class="mb-2 text-sm font-medium">Characters</legend>
            {#if !characters.length}<p class="text-sm text-muted-foreground">No characters yet. <a href="/library?tab=characters" class="underline underline-offset-4">Add a character</a></p>{/if}
            {#each characters as character (character.id)}<label class="flex min-h-11 cursor-pointer items-center gap-3 text-sm"><input type="checkbox" bind:group={characterIDs} value={character.id} /><span class="min-w-0 break-words">{character.name||character.id}</span></label>{/each}
          </fieldset>
          <fieldset class="min-w-0 space-y-1" disabled={busy||running}><legend class="mb-2 text-sm font-medium">Reference assets</legend>
            {#if !assets.length}<p class="text-sm text-muted-foreground">No assets yet. <a href="/library?tab=media" class="underline underline-offset-4">Upload references</a></p>{/if}
            {#each assets as asset (asset.id)}<label class="flex min-h-11 cursor-pointer items-center gap-3 text-sm"><input type="checkbox" bind:group={assetIDs} value={asset.id} /><span class="min-w-0 break-words">{asset.name||asset.id}</span></label>{/each}
          </fieldset>
        </div>
        {#if running}<p class="text-sm text-muted-foreground">References can be changed after the current request finishes.</p>{/if}
        {#each warnings as warning}<p class="text-sm text-muted-foreground">{warning}</p>{/each}
        {#if notice}<p role="status" class="text-sm">{notice}</p>{/if}
      {/if}
    </div>
    <Dialog.Footer><Button variant="outline" disabled={busy} onclick={()=>changeOpen(false)}>Close</Button><Button disabled={busy||!dirty||running} onclick={save}>{busy?'Saving…':'Save references'}</Button></Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
