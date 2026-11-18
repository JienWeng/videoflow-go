<script lang="ts">
 import {onMount} from 'svelte';
 import {page} from '$app/state';
 import {invalidateAll,goto} from '$app/navigation';
 import {get,post} from '$lib/api';
 import {Button} from '$lib/components/ui/button';
 import {Badge} from '$lib/components/ui/badge';
 import ProjectStyle from '$lib/projects/ProjectStyle.svelte';
 import {ArrowLeft,ArrowUpRight,Folder} from '@lucide/svelte';
 let projects:any[]=$state([]),loaded=$state(false),error=$state(''),busy=$state(false);
 const id=$derived(page.url.searchParams.get('id')??'');
 const project=$derived(projects.find(p=>p.id===id));
 async function load(){error='';try{const result=await get('/projects');if(!Array.isArray(result))throw Error('Projects are unavailable');projects=result}catch(e){error=String(e)}finally{loaded=true}}
 onMount(()=>{void load()});
 async function activate(studio:boolean){if(busy||!project)return;busy=true;error='';try{if(!project.is_active)await post(`/projects/${project.id}/activate`);await invalidateAll();if(studio)await goto('/');else await load()}catch(e){error=String(e)}finally{busy=false}}
</script>
<svelte:head><title>{project?.name??'Project details'} · VideoFlow</title></svelte:head>
<div class="mx-auto w-full max-w-[1200px] p-4 sm:p-8">
 <a href="/projects" class="mb-6 inline-flex min-h-11 items-center gap-2 text-sm text-muted-foreground hover:text-foreground"><ArrowLeft class="size-4"/>All projects</a>
 {#if error}<div role="alert" class="mb-4 rounded-md border border-destructive p-4 text-sm"><p>{error}</p><Button class="mt-2" variant="outline" disabled={busy} onclick={load}>Reload project</Button></div>{/if}
 {#if !loaded}<p role="status">Loading project…</p>{:else if !project&&!error}<h1 class="text-2xl font-semibold">Project not found</h1><p class="mt-2 text-muted-foreground">It may have been deleted. Choose another project from the collection.</p>{:else if project}
 <header class="mb-8 flex flex-wrap items-start justify-between gap-4"><div class="min-w-0"><div class="flex items-center gap-3"><Folder class="size-7 shrink-0 text-muted-foreground"/><h1 class="break-words text-2xl font-semibold tracking-tight">{project.name}</h1>{#if project.is_active}<Badge variant="secondary">Active</Badge>{/if}</div>{#if project.description}<p class="mt-3 max-w-prose break-words text-sm text-muted-foreground">{project.description}</p>{/if}</div><Button disabled={busy} onclick={()=>activate(true)}>Open in Studio<ArrowUpRight class="size-4"/></Button></header>
 <dl class="mb-8 grid grid-cols-3 gap-4 border-y border-border py-5">{#each [['Scenes',project.counts?.scenes??0],['Characters',project.counts?.characters??0],['Assets',project.counts?.assets??0]] as [label,count]}<div><dt class="text-xs text-muted-foreground">{label}</dt><dd class="mt-1 text-2xl font-semibold tabular-nums">{count}</dd></div>{/each}</dl>
 {#if project.is_active}{#key project.id}<ProjectStyle projectId={project.id} projectName={project.name} disabled={busy} onSavingChange={saving=>busy=saving}/>{/key}{:else}<section class="rounded-lg border border-border p-5"><h2 class="text-base font-semibold">Project style</h2><p class="mt-2 text-sm text-muted-foreground">Make this your active project to edit its visual style.</p><Button variant="outline" class="mt-4" disabled={busy} onclick={()=>activate(false)}>Activate to edit style</Button></section>{/if}
 {/if}
</div>
