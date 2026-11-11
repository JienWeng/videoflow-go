<script lang="ts">
  import { onMount } from 'svelte';
  import type { Node } from '@xyflow/svelte';
  import EntityCanvas from '$lib/canvas/EntityCanvas.svelte';
  import NodePanel from '$lib/canvas/NodePanel.svelte';
  import ChatPanel from '$lib/chat/ChatPanel.svelte';
  import SessionSelector from '$lib/shell/SessionSelector.svelte';
  import Onboarding from '$lib/shell/Onboarding.svelte';
  import { Button } from '$lib/components/ui/button';
  import { palette } from '$lib/shell/palette.svelte';
  import { get } from '$lib/api';
  import { subscribeJobs } from '$lib/sse';
  let canvas:EntityCanvas|undefined=$state(), details:NodePanel|undefined=$state(), chat:ChatPanel|undefined=$state();
  let selected=$state<Node|null>(null);
  let tab=$state('chat'), mobileView=$state('canvas'), nodeCount=$state(-1), error=$state('');
  const context=$derived(selected?{id:selected.id,kind:String(selected.data?.kind??''),label:String(selected.data?.label??'')}:null);
  async function probe(){try{const g=await get('/graph');nodeCount=g.nodes?.length??0;error=''}catch(e){error=String(e)}}
  async function refresh(){await canvas?.refresh();await probe();}
  function beforeSelect(next:Node|null){return next?.id===selected?.id || details?.canLeave()!==false;}
  function select(next:Node|null){const changed=next?.id!==selected?.id;selected=next;if(next&&changed){tab='details';mobileView='workbench';}}
  function clear(){canvas?.clearSelection();}
  onMount(()=>{probe();palette.setHandlers({onfocus:id=>canvas?.focusNode(id),onnewstory:()=>{tab='chat';mobileView='workbench';chat?.startNewStory()}});const stop=subscribeJobs(refresh,refresh);return()=>{stop();palette.clearHandlers()}});
</script>
<div class="flex h-full min-h-0 flex-col">
  <header class="flex min-h-16 shrink-0 flex-wrap items-center justify-between gap-3 border-b border-border px-4 py-3 sm:px-6">
    <div><h1 class="text-base font-semibold">Story studio</h1><p class="text-xs text-muted-foreground">Select an item. Shape the story. Review before generating.</p></div>
    <div class="flex gap-2"><Button variant="outline" size="sm" href="/create">New story</Button><Button variant="outline" size="sm" href="/render">View videos</Button></div>
  </header>
  <div class="flex shrink-0 border-b border-border md:hidden" aria-label="Studio view">
    <button class="min-h-11 flex-1 text-sm {mobileView==='canvas'?'bg-accent font-semibold':''}" aria-pressed={mobileView==='canvas'} onclick={()=>mobileView='canvas'}>Canvas</button>
    <button class="min-h-11 flex-1 text-sm {mobileView==='workbench'?'bg-accent font-semibold':''}" aria-pressed={mobileView==='workbench'} onclick={()=>mobileView='workbench'}>Workbench{context?' · '+context.kind:''}</button>
  </div>
  <div class="flex min-h-0 flex-1">
    <section aria-label="Story canvas" class="relative min-w-0 flex-1 {mobileView==='canvas'?'block':'hidden'} md:block">
      <EntityCanvas bind:this={canvas} externalPanel beforeselect={beforeSelect} onselect={select} />
      {#if error}<div role="alert" class="absolute inset-x-4 top-20 rounded-xl border border-destructive bg-background p-4 text-sm"><p>Could not load your story.</p><p class="mt-1 text-xs text-muted-foreground">{error}</p><Button variant="outline" class="mt-3" onclick={refresh}>Retry</Button></div>
      {:else if nodeCount===0}<div class="pointer-events-none absolute inset-0 flex items-center justify-center p-5"><div class="pointer-events-auto w-full max-w-sm rounded-2xl border border-border bg-background p-6"><h2 class="text-xl font-semibold tracking-tight">Your story starts here</h2><p class="mt-2 text-sm leading-relaxed text-muted-foreground">Start an AI session, then describe your idea in Chat. Characters, scenes and shots will appear on this canvas.</p><Button class="mt-5 w-full" onclick={()=>{tab='chat';mobileView='workbench'}}>Open chat</Button><a href="/create" class="mt-3 block text-center text-xs text-muted-foreground underline underline-offset-4">Prefer a guided start?</a></div></div>{/if}
    </section>
    <aside aria-label="Contextual workbench" class="min-h-0 w-full shrink-0 flex-col border-l border-border bg-background md:w-[380px] lg:w-[420px] {mobileView==='workbench'?'flex':'hidden md:flex'}">
      <div class="space-y-3 border-b border-border p-4"><div class="flex items-start justify-between gap-2"><div class="min-w-0"><p class="text-xs text-muted-foreground">{context?context.kind.replace('_',' '):'Project scope'}</p><h2 class="truncate text-sm font-semibold" title={context?.label}>{context?.label??'Story assistant'}</h2></div>{#if context}<Button size="sm" variant="ghost" onclick={clear}>Clear selection</Button>{/if}</div><SessionSelector />
        <nav aria-label="Workflow next steps" class="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground"><a class="underline underline-offset-4" href="/characters">Characters</a><a class="underline underline-offset-4" href={context?.kind==='scene'?`/assets?scene_id=${context.id}`:'/assets'}>Review references</a><a class="underline underline-offset-4" href="/render">Render progress</a><a class="underline underline-offset-4" href="/voices">Voices & captions</a></nav>
      </div>
      <div role="tablist" aria-label="Workbench" class="flex shrink-0 border-b border-border px-4">
        {#each ['details','chat'] as value}<button role="tab" id={'wb-'+value} aria-selected={tab===value} aria-controls={'panel-'+value} tabindex={tab===value?0:-1} class="min-h-11 flex-1 border-b-2 text-sm {tab===value?'border-primary font-semibold':'border-transparent text-muted-foreground'}" onclick={()=>tab=value} onkeydown={e=>{if(['ArrowLeft','ArrowRight','Home','End'].includes(e.key)){e.preventDefault();tab=e.key==='Home'?'details':e.key==='End'?'chat':value==='chat'?'details':'chat';document.getElementById('wb-'+tab)?.focus();}}}>{value==='details'?'Details':'Chat'}</button>{/each}
      </div>
      <div id="panel-details" role="tabpanel" aria-labelledby="wb-details" class="min-h-0 flex-1 {tab==='details'?'':'hidden'}"><NodePanel bind:this={details} embedded node={selected} onclose={clear} onsaved={refresh} />{#if !selected}<div class="p-6"><h3 class="font-medium">Select something to edit</h3><p class="mt-2 text-sm text-muted-foreground">Click a scene, shot, character or asset. Its current details and editing options appear here.</p><Button variant="outline" class="mt-4 md:hidden" onclick={()=>mobileView='canvas'}>Back to canvas</Button></div>{/if}</div>
      <div id="panel-chat" role="tabpanel" aria-labelledby="wb-chat" class="flex min-h-0 flex-1 flex-col {tab==='chat'?'':'hidden'}">
        {#if selected}<details class="shrink-0 border-b border-border px-4 py-2 text-xs"><summary class="cursor-pointer text-muted-foreground">Context: {context?.label}</summary><p class="mt-2 max-h-32 overflow-auto whitespace-pre-wrap">{String(selected.data?.prompt??selected.data?.summary??selected.data?.description??'Current record and related references are resolved by the server.')}</p></details>{/if}
        <div class="min-h-0 flex-1"><ChatPanel embedded bind:this={chat} selected={context} onfocus={id=>canvas?.focusNode(id)} onmutate={refresh} /></div>
      </div>
    </aside>
  </div>
</div>
<Onboarding onpalette={()=>palette.open=true} />
