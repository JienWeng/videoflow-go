<script lang="ts">
 import {onMount,untrack} from 'svelte';
 import {page} from '$app/state';
 import {replaceState} from '$app/navigation';
 import {get} from '$lib/api';
 import {Users,Images} from '@lucide/svelte';
 import CharacterLibrary from './CharacterLibrary.svelte';
 import AssetLibrary from './AssetLibrary.svelte';
 const prefix=$props.id();
 const tab=$derived((page.state as {libraryTab?:'characters'|'media'}).libraryTab??(['assets','media'].includes(page.url.searchParams.get('tab')??'')?'media':'characters'));
 let projectName=$state(''),characterCount=$state<number|null>(null),mediaCount=$state<number|null>(null);
 let charactersMounted=$state(false),mediaMounted=$state(false);
 let characters:CharacterLibrary|undefined=$state(),media:AssetLibrary|undefined=$state();
 let lastTab='';
 const counts=(characters:number,media:number)=>{characterCount=characters;mediaCount=media;};
 $effect(()=>{const next=tab;if(next===lastTab)return;lastTab=next;untrack(()=>{if(next==='media'){if(mediaMounted)void media?.reload();else mediaMounted=true;}else{if(charactersMounted)void characters?.reload();else charactersMounted=true;}})});
 function select(value:string){const url=new URL(page.url);url.searchParams.set('tab',value);replaceState(url,{...page.state,libraryTab:value})}
 onMount(()=>{get('/projects/active').then(project=>projectName=project.name??'').catch(()=>{})});
</script>
<svelte:head><title>Library · VideoFlow</title></svelte:head>
<div class="mx-auto min-h-full w-full max-w-[1600px]">
 <header class="px-4 pt-6 sm:px-8 sm:pt-8"><div class="mb-6"><h1 class="text-2xl font-semibold tracking-tight">Library</h1>{#if projectName}<p class="mt-1 text-sm text-muted-foreground">{projectName}</p>{/if}</div>
 <div role="tablist" aria-label="Library collections" class="flex gap-5 border-b border-border">
 {#each ['characters','media'] as value}{@const count=value==='characters'?characterCount:mediaCount}<button id={prefix+'-'+value} role="tab" aria-selected={tab===value} aria-controls={prefix+'-panel-'+value} tabindex={tab===value?0:-1} class="flex min-h-12 items-center gap-2 border-b-2 px-1 pb-3 text-sm {tab===value?'border-primary font-semibold':'border-transparent text-muted-foreground hover:text-foreground'}" onclick={()=>select(value)} onkeydown={e=>{if(['ArrowLeft','ArrowRight','Home','End'].includes(e.key)){e.preventDefault();const next=e.key==='Home'?'characters':e.key==='End'?'media':value==='media'?'characters':'media';select(next);document.getElementById(prefix+'-'+next)?.focus()}}}>{#if value==='characters'}<Users class="size-4"/>{:else}<Images class="size-4"/>{/if}{value==='characters'?'Characters':'Media'}{#if count!==null}<span class="rounded-md bg-muted px-1.5 py-0.5 text-xs tabular-nums text-muted-foreground">{count}</span>{/if}</button>{/each}
 </div></header>
 <div role="tabpanel" tabindex="0" id={prefix+'-panel-characters'} aria-labelledby={prefix+'-characters'} class={tab==='characters'?'':'hidden'}>{#if charactersMounted}<CharacterLibrary embedded bind:this={characters} oncounts={counts}/>{/if}</div>
 <div role="tabpanel" tabindex="0" id={prefix+'-panel-media'} aria-labelledby={prefix+'-media'} class={tab==='media'?'':'hidden'}>{#if mediaMounted}<AssetLibrary embedded bind:this={media} oncounts={counts}/>{/if}</div>
</div>
