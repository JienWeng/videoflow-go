<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { replaceState } from '$app/navigation';
  import { page } from '$app/state';
  import type { Node } from '@xyflow/svelte';
  import StoryWorkspace from '$lib/scenes/StoryWorkspace.svelte';
  import {storyCreation} from '$lib/create/story-creation.svelte';
  import EntityCanvas from '$lib/canvas/EntityCanvas.svelte';
  import NodePanel from '$lib/canvas/NodePanel.svelte';
  import ChatPanel from '$lib/chat/ChatPanel.svelte';
  import SessionSelector from '$lib/shell/SessionSelector.svelte';
  import Onboarding from '$lib/shell/Onboarding.svelte';
  import ProjectOverview from '$lib/dashboard/ProjectOverview.svelte';
  import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
  import MoreHorizontal from '@lucide/svelte/icons/ellipsis';
  import Maximize2 from '@lucide/svelte/icons/maximize-2';
  import X from '@lucide/svelte/icons/x';
  import MessageSquare from '@lucide/svelte/icons/message-square';
  import Minimize2 from '@lucide/svelte/icons/minimize-2';
  import { Button } from '$lib/components/ui/button';
  import { palette } from '$lib/shell/palette.svelte';
  import { get } from '$lib/api';
  import { subscribeJobs } from '$lib/sse';
  let canvas:EntityCanvas|undefined=$state(), details:NodePanel|undefined=$state(), chat:ChatPanel|undefined=$state();
  let assistantOpen=$state(true),openAssistantButton:HTMLButtonElement|null=$state(null);
  function openAssistant(){assistantOpen=true;mobileView='workbench';void tick().then(()=>{if(tab==='chat')document.querySelector<HTMLTextAreaElement>('[aria-label="Message story assistant"]')?.focus();else document.querySelector<HTMLElement>('#panel-details input, #panel-details textarea')?.focus();});}
  function closeAssistant(){assistantOpen=false;mobileView='canvas';void tick().then(()=>openAssistantButton?.focus());}
  let panelWidth=$state(420), panelExpanded=$state(false), restoreWidth=420;
  let studioElement: HTMLDivElement | null=$state(null), maxPanelWidth=$state(760);
  const minPanelWidth=$derived(Math.min(340,maxPanelWidth));
  $effect(()=>{
    if(!studioElement)return;
    const observer=new ResizeObserver(([entry])=>{
      if(entry.contentRect.width<1)return;
      maxPanelWidth=Math.max(240,Math.min(760,entry.contentRect.width-248));
      panelWidth=Math.max(Math.min(340,maxPanelWidth),Math.min(panelWidth,maxPanelWidth));
    });
    observer.observe(studioElement);return()=>observer.disconnect();
  });
  function resizePanel(value:number){panelWidth=Math.round(Math.max(minPanelWidth,Math.min(value,maxPanelWidth)));panelExpanded=false;}
  function togglePanelSize(){
    if(panelExpanded){resizePanel(restoreWidth);return;}
    restoreWidth=panelWidth;resizePanel(maxPanelWidth);panelExpanded=true;
  }
  function dragPanel(e:PointerEvent){
    const target=e.currentTarget as HTMLElement;
    target.setPointerCapture(e.pointerId);
    const start=e.clientX,width=panelWidth;
    const move=(event:PointerEvent)=>resizePanel(width+start-event.clientX);
    const end=()=>{target.removeEventListener('pointermove',move);target.removeEventListener('pointerup',end);target.removeEventListener('pointercancel',end)};
    target.addEventListener('pointermove',move);target.addEventListener('pointerup',end);target.addEventListener('pointercancel',end);
  }
  let selected=$state<Node|null>(null);
  let overview: ProjectOverview | undefined = $state();
  const workspace = $derived((page.state as { studioView?: 'overview' | 'studio' | 'story' }).studioView ?? (page.url.searchParams.get('view') === 'overview' ? 'overview' : page.url.searchParams.get('view') === 'story' ? 'story' : 'studio'));
  let studioMounted = $state(false), overviewMounted = $state(false),storyMounted=$state(false);
  let story:StoryWorkspace|undefined=$state();
  let canvasRefreshPaused = $state(false);
  $effect(() => { if(workspace==='studio')studioMounted=true;else if(workspace==='story')storyMounted=true;else overviewMounted=true; });
  let tab=$state('chat'), mobileView=$state('canvas'), nodeCount=$state(-1), error=$state('');
  const context=$derived(selected?{id:selected.id,kind:String(selected.data?.kind??''),label:String(selected.data?.label??'')}:null);
  async function probe(){try{const g=await get('/graph');nodeCount=g.nodes?.length??0;error=''}catch(e){error=String(e)}}
  $effect(() => { if(workspace==='studio'&&nodeCount===-1)void probe(); });
  async function refreshStudio(){
    if(details?.hasUnsavedChanges()){canvasRefreshPaused=true;return;}
    canvasRefreshPaused=false;await canvas?.refresh();await probe();
  }
  async function refresh(){if(workspace==='overview')await overview?.refresh();else if(workspace==='story')await story?.reload();else await refreshStudio();}
  function setWorkspace(value: 'overview' | 'studio' | 'story') {
    if(page.url.pathname!=='/')return;
    const changed=value!==workspace;
    const wasMounted=value==='studio'?studioMounted:value==='story'?storyMounted:overviewMounted;
    if(value==='studio')studioMounted=true;else if(value==='story')storyMounted=true;else overviewMounted=true;
    const url = new URL(page.url);
    if(value==='studio')url.searchParams.delete('view');else url.searchParams.set('view',value);
    replaceState(url, { ...page.state, studioView: value });
    if(changed&&wasMounted)void tick().then(()=>value==='overview'?overview?.refresh():value==='story'?story?.reload():refreshStudio());
  }
  async function focusNode(id: string) { const first=!studioMounted;setWorkspace('studio');await tick();if(first)await canvas?.refresh();canvas?.focusNode(id); }
  function startStory(){storyCreation.show();}
  async function editScene(id:string){setWorkspace('story');await tick();await story?.openScene(id);}
  function beforeSelect(next:Node|null){return next?.id===selected?.id || details?.canLeave()!==false;}
  function select(next:Node|null){const changed=next?.id!==selected?.id;selected=next;if(!next)tab='chat';if(next&&changed){setWorkspace('studio');tab='details';mobileView='workbench';assistantOpen=true;}}
  function clear(){canvas?.clearSelection();tab='chat';}
  onMount(()=>{palette.setHandlers({onfocus:focusNode,onnewstory:startStory});if(page.url.searchParams.has('new_story'))void tick().then(async()=>{await startStory();const url=new URL(window.location.href);url.searchParams.delete('new_story');replaceState(url,page.state);});const stop=subscribeJobs(refresh,refresh);window.addEventListener('videoflow-library-changed',refresh);return()=>{stop();window.removeEventListener('videoflow-library-changed',refresh);palette.clearHandlers()}});
</script>
<svelte:head><title>{workspace==='overview'?'Project overview · VideoFlow':'VideoFlow Studio'}</title></svelte:head>
<div class="flex h-full min-h-0 flex-col">
  <h1 class="sr-only">VideoFlow Studio</h1>
  <header class="flex min-h-16 shrink-0 flex-wrap items-center justify-between gap-2 border-b border-border px-4 py-2 sm:px-6">
    <DropdownMenu.Root><DropdownMenu.Trigger class="inline-flex min-h-11 items-center gap-2 rounded-lg px-2 text-sm font-semibold hover:bg-accent" aria-label="Project menu">Studio <MoreHorizontal class="size-4" /></DropdownMenu.Trigger><DropdownMenu.Content align="start">
      <DropdownMenu.Item onclick={()=>setWorkspace('overview')}>Project summary</DropdownMenu.Item>
      <DropdownMenu.Item>{#snippet child({props})}<a {...props} href="/projects">Switch project</a>{/snippet}</DropdownMenu.Item>
    </DropdownMenu.Content></DropdownMenu.Root>
    <div role="group" aria-label="Workspace view" class="flex rounded-lg bg-muted/40 p-1">
      <button class="min-h-10 rounded-md px-3 text-sm {workspace==='studio'?'bg-accent font-semibold':'text-muted-foreground hover:text-foreground'}" aria-pressed={workspace==='studio'} onclick={()=>setWorkspace('studio')}>Canvas</button>
      <button class="min-h-10 rounded-md px-3 text-sm {workspace==='story'?'bg-accent font-semibold':'text-muted-foreground hover:text-foreground'}" aria-pressed={workspace==='story'} onclick={()=>setWorkspace('story')}>Story</button>
    </div>
    <div class="flex gap-1">{#if workspace==='story'}<Button variant="ghost" onclick={()=>{setWorkspace('studio');tab='chat';openAssistant()}}>Assistant</Button>{/if}<Button onclick={startStory}>Create story</Button></div>
  </header>
  {#if workspace==='studio'&&canvasRefreshPaused}<div role="status" class="flex flex-wrap items-center justify-between gap-2 border-b border-border px-4 py-2 text-xs text-muted-foreground"><p>Your editing draft is kept. Save or discard it before refreshing the canvas.</p><Button variant="ghost" size="sm" disabled={details?.hasUnsavedChanges()} onclick={refreshStudio}>Refresh canvas</Button></div>{/if}
  {#if overviewMounted}<section aria-label="Project overview" class="min-h-0 flex-1 overflow-auto {workspace==='overview'?'':'hidden'}"><ProjectOverview bind:this={overview} active={workspace==='overview'} onopenStudio={()=>setWorkspace('studio')} /></section>{/if}
  {#if storyMounted}<section aria-label="Story workspace" class="min-h-0 flex-1 {workspace==='story'?'':'hidden'}"><StoryWorkspace embedded bind:this={story}/></section>{/if}
  {#if studioMounted}
  <div class="shrink-0 border-b border-border {workspace==='studio'?'flex md:hidden':'hidden'}" aria-label="Studio view">
    <button class="min-h-11 flex-1 text-sm {mobileView==='canvas'?'bg-accent font-semibold':''}" aria-pressed={mobileView==='canvas'} onclick={()=>mobileView='canvas'}>Canvas</button>
    <button class="min-h-11 flex-1 text-sm {mobileView==='workbench'?'bg-accent font-semibold':''}" aria-pressed={mobileView==='workbench'} onclick={openAssistant}>Workbench{context?' · '+context.kind:''}</button>
  </div>
  <div bind:this={studioElement} style:--panel-width={panelWidth+'px'} class:assistant-visible={assistantOpen} class="studio-area relative min-h-0 flex-1 {workspace==='studio'?'flex':'hidden'}" aria-label="Studio editing workspace">
    <section aria-label="Story canvas" class="canvas-pane relative min-w-0 flex-1 {mobileView==='canvas'?'block':'hidden'} md:block">
      <EntityCanvas bind:this={canvas} externalPanel oninspect={()=>{tab='details';mobileView='workbench';assistantOpen=true;}} onopenscene={editScene} beforeselect={beforeSelect} onselect={select} />
      {#if error}<div role="alert" class="absolute inset-x-4 top-20 rounded-xl border border-destructive bg-background p-4 text-sm"><p>Could not load your story.</p><p class="mt-1 text-xs text-muted-foreground">{error}</p><Button variant="outline" class="mt-3" onclick={refresh}>Retry</Button></div>
      {:else if nodeCount===0}<div class="pointer-events-none absolute inset-0 flex items-center justify-center p-5"><div class="pointer-events-auto max-w-sm text-center"><h2 class="text-sm font-medium text-muted-foreground">Your story starts here</h2><p class="mt-2 max-w-64 text-xs leading-relaxed text-muted-foreground">Describe an idea in the assistant, or use Create story for a guided start.</p>{#if assistantOpen}<Button variant="outline" class="mt-5 md:hidden" onclick={()=>{tab='chat';openAssistant();}}>Open assistant</Button>{/if}</div></div>{/if}
    </section>
    <!-- A focusable WAI-ARIA window splitter is an interactive separator. -->
    <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
    <div role="separator" tabindex="0" aria-label="Resize assistant panel" aria-orientation="vertical" aria-controls="assistant-workbench" aria-valuemin={minPanelWidth} aria-valuemax={maxPanelWidth} aria-valuenow={panelWidth} class="assistant-resizer hidden w-2 cursor-col-resize touch-none items-center justify-center rounded-lg hover:bg-accent focus-visible:bg-accent focus-visible:outline-2 focus-visible:outline-ring {assistantOpen?'md:flex':''}" onpointerdown={dragPanel} onkeydown={e=>{if(['ArrowLeft','ArrowRight','Home','End'].includes(e.key)){e.preventDefault();resizePanel(e.key==='Home'?minPanelWidth:e.key==='End'?maxPanelWidth:panelWidth+(e.key==='ArrowLeft'?32:-32));}}}><span class="h-8 w-0.5 rounded-full bg-muted-foreground/50"></span></div>
    <aside id="assistant-workbench" aria-label="Contextual workbench" style:--panel-width={panelWidth+'px'} class="assistant-panel min-h-0 w-full shrink-0 flex-col overflow-hidden bg-card {assistantOpen?(mobileView==='workbench'?'flex':'hidden md:flex'):'hidden'}">
      <div class="flex shrink-0 items-start gap-1 px-3 pt-3 pb-2">
        <h2 class="sr-only">Story assistant</h2><div class="min-w-0 flex-1"><SessionSelector compact minimal /></div>
        <Button variant="ghost" size="icon" class="hidden size-9 shrink-0 md:inline-flex" aria-label={panelExpanded?'Restore assistant size':'Expand assistant'} onclick={togglePanelSize}>{#if panelExpanded}<Minimize2 class="size-4" />{:else}<Maximize2 class="size-4" />{/if}</Button>
        <Button variant="ghost" size="icon" class="size-9 shrink-0" aria-label="Close assistant" title="Close assistant; your draft is kept" onclick={closeAssistant}><X class="size-4" /></Button>
      </div>
      {#if selected}<div role="tablist" aria-label="Workbench" class="flex shrink-0 border-b border-border px-4">
        {#each (selected?['details','chat']:['chat']) as value}<button role="tab" id={'wb-'+value} aria-selected={tab===value} aria-controls={'panel-'+value} tabindex={tab===value?0:-1} class="min-h-11 flex-1 border-b-2 text-sm {tab===value?'border-primary font-semibold':'border-transparent text-muted-foreground'}" onclick={()=>tab=value} onkeydown={e=>{if(selected&&['ArrowLeft','ArrowRight','Home','End'].includes(e.key)){e.preventDefault();tab=e.key==='Home'?'details':e.key==='End'?'chat':value==='chat'?'details':'chat';document.getElementById('wb-'+tab)?.focus();}}}>{value==='details'?'Details':'Chat'}</button>{/each}
      </div>{/if}
      <div id="panel-details" role="tabpanel" aria-labelledby="wb-details" class="min-h-0 flex-1 {tab==='details'?'':'hidden'}"><NodePanel bind:this={details} embedded node={selected} onclose={clear} onsaved={refresh} onopenscene={editScene} />{#if !selected}<div class="p-6"><h3 class="font-medium">Select something to edit</h3><p class="mt-2 text-sm text-muted-foreground">Click a scene, shot, character or asset. Its current details and editing options appear here.</p><Button variant="outline" class="mt-4 md:hidden" onclick={()=>mobileView='canvas'}>Back to canvas</Button></div>{/if}</div>
      <div id="panel-chat" role={selected?'tabpanel':undefined} aria-label={selected?undefined:'Conversation'} aria-labelledby={selected?'wb-chat':undefined} class="flex min-h-0 flex-1 flex-col {tab==='chat'?'':'hidden'}">
        <div class="min-h-0 flex-1"><ChatPanel embedded bind:this={chat} selected={context} onclear={clear} onfocus={focusNode} onmutate={refresh} /></div>
      </div>
    </aside>
    {#if !assistantOpen}<Button bind:ref={openAssistantButton} class="absolute right-4 bottom-4 z-20 min-h-11 rounded-xl shadow-lg" onclick={openAssistant} aria-label="Open assistant"><MessageSquare class="size-4" />Open assistant</Button>{/if}
  </div>
  {/if}
</div>
{#if workspace==='studio'}<Onboarding onpalette={()=>palette.open=true} />{/if}

<style>
  @media (min-width:768px) {
    .studio-area {background-image:radial-gradient(var(--border) .7px,transparent .7px);background-size:20px 20px;}
    .assistant-visible .canvas-pane {margin-right:calc(var(--panel-width) + 32px);}
    .assistant-panel {position:absolute;right:16px;top:24px;bottom:16px;width:var(--panel-width);max-width:calc(100% - 248px);border:1px solid var(--border);border-radius:16px;box-shadow:0 12px 36px rgb(0 0 0 / .18);}
    .assistant-resizer {position:absolute;right:calc(var(--panel-width) + 18px);top:44px;bottom:36px;z-index:10;}
  }
</style>
