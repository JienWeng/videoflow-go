<script lang="ts">
 import {mediaUrl} from '$lib/api';
 import {mediaKind} from './catalog.mjs';
 import {UserRound,Image,Film,AudioLines,File} from '@lucide/svelte';
 let {asset,portrait=false,class:className=''}:{asset:{file_path?:string|null;type?:string|null;name?:string}|null;portrait?:boolean;class?:string}=$props();
 let failed=$state(false);
 const kind=$derived(mediaKind(asset??{}));
 const url=$derived(mediaUrl(asset?.file_path));
 $effect(()=>{void asset?.file_path;failed=false});
</script>
<div class="relative flex items-center justify-center overflow-hidden bg-muted/35 {className}">
 {#if url&&!failed&&kind==='image'}<img src={url} alt={asset?.name||'Reference image'} loading="lazy" class="h-full w-full object-cover" onerror={()=>failed=true}/>
 {:else if url&&!failed&&kind==='video'}
  <!-- svelte-ignore a11y_media_has_caption -->
  <video src={url} muted preload="metadata" aria-hidden="true" class="pointer-events-none h-full w-full object-contain" onerror={()=>failed=true}></video><span class="absolute bottom-3 left-3 rounded-md bg-background/90 px-2 py-1 text-xs">Video</span>
 {:else}<div class="flex flex-col items-center gap-3 p-5 text-muted-foreground">{#if portrait}<UserRound class="size-10"/>{:else if kind==='video'}<Film class="size-9"/>{:else if kind==='audio'}<AudioLines class="size-9"/>{:else if kind==='image'}<Image class="size-9"/>{:else}<File class="size-9"/>{/if}<span class="text-xs">{failed?'Preview unavailable':portrait?'No reference image':kind==='audio'?'Audio':kind==='video'?'Video':kind==='image'?'Image':'File'}</span></div>{/if}
</div>
