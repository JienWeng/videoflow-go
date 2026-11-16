<script lang="ts">
 import CharacterLibrary from './CharacterLibrary.svelte';
 import AssetLibrary from './AssetLibrary.svelte';
 import {page} from '$app/state';
 import {replaceState} from '$app/navigation';
 let {embedded=false}:{embedded?:boolean}=$props();
 const prefix=$props.id();
 let localTab=$state('characters');
 const tab=$derived(embedded?localTab:page.url.searchParams.get('tab')==='assets'?'assets':'characters');
 let characters:CharacterLibrary|undefined=$state(),assets:AssetLibrary|undefined=$state();
 export function canClose(){return !(characters?.hasDraft()||assets?.hasDraft())||window.confirm('Discard unsaved library changes?');}
 let charactersMounted=$state(false),assetsMounted=$state(false);
 $effect(()=>{if(tab==='assets')assetsMounted=true;else charactersMounted=true});
 function select(value:string){if(embedded){localTab=value;return;}const url=new URL(page.url);url.searchParams.set('tab',value);replaceState(url,page.state)}
</script>
<svelte:head>{#if !embedded}<title>Library · VideoFlow</title>{/if}</svelte:head>
<div class="flex min-h-full flex-col">
 <header class="flex flex-wrap items-center gap-4 border-b border-border px-6 py-3">{#if !embedded}<h1 class="mr-auto font-semibold">Library</h1>{/if}<div role="tablist" aria-label="Library">
 {#each ['characters','assets'] as value}<button id={prefix+'-library-'+value} role="tab" aria-selected={tab===value} aria-controls={prefix+'-library-panel-'+value} tabindex={tab===value?0:-1} class="min-h-11 rounded-lg px-4 text-sm capitalize {tab===value?'bg-accent font-semibold':'text-muted-foreground'}" onclick={()=>select(value)} onkeydown={e=>{if(['ArrowLeft','ArrowRight','Home','End'].includes(e.key)){e.preventDefault();const next=e.key==='Home'?'characters':e.key==='End'?'assets':value==='assets'?'characters':'assets';select(next);document.getElementById(prefix+'-library-'+next)?.focus()}}}>{value==='characters'?'Characters':'Assets'}</button>{/each}</div></header>
 {#if charactersMounted}<div role="tabpanel" tabindex="0" id={prefix+'-library-panel-characters'} aria-labelledby={prefix+'-library-characters'} class={tab==='characters'?'':'hidden'}><CharacterLibrary embedded bind:this={characters} /></div>{/if}
 {#if assetsMounted}<div role="tabpanel" tabindex="0" id={prefix+'-library-panel-assets'} aria-labelledby={prefix+'-library-assets'} class={tab==='assets'?'':'hidden'}><AssetLibrary embedded bind:this={assets} /></div>{/if}
</div>
