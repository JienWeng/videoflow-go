<script lang="ts">
  import { onMount, untrack, tick } from 'svelte';
  import {
    SvelteFlow,
    ConnectionMode,
    Background,
    Controls,
    MiniMap,
    Panel,
    useSvelteFlow,
    type Node,
    type Edge,
    type Connection
  } from '@xyflow/svelte';
  import '@xyflow/svelte/dist/style.css';
  import { toast } from 'svelte-sonner';
  import {
    LayoutGrid,
    Plus,
    Clapperboard,
    Users,
    Image,
    ChevronLeft,
    X, Search, Maximize2, Undo2, Focus
  } from '@lucide/svelte';
  import { Button } from '$lib/components/ui/button';
  import { Input } from '$lib/components/ui/input';
  import { get, post, del } from '$lib/api';
  import {
    toFlow,
    loadPositions,
    savePositions,
    clearPositions,
    kindColor,
    edgeColor,
    LEGEND_KINDS,
    KIND_LABELS,
    type ApiGraph
  } from './transform';
  import { layout, layoutFresh } from './layout';
  import EntityNode from './EntityNode.svelte';
  import FlowHelper from './FlowHelper.svelte';
  import CanvasMenu from './CanvasMenu.svelte';
  import NodePanel from './NodePanel.svelte';

  let { onselect, externalPanel = false, beforeselect }: { onselect: (node: Node | null) => void; externalPanel?: boolean; beforeselect?: (node: Node|null)=>boolean } = $props();

  let nodes = $state.raw<Node[]>([]);
  let edges = $state.raw<Edge[]>([]);
  let flow: ReturnType<typeof useSvelteFlow> | undefined;
  let canvasRoot: HTMLDivElement | undefined = $state();

  // Right-click context menu (CanvasMenu) — null when closed. `view` lets the
  // Delete/Backspace gesture open it straight on the confirm-delete step.
  let menu = $state<{ node: Node; x: number; y: number; view?: 'root' | 'confirm-delete' } | null>(
    null
  );
  // "+" add menu in the top-right panel.
  let addOpen = $state(false);
  let addMode = $state<'root' | 'scene' | 'character'>('root');
  let addName = $state('');
  let addBusy = $state(false);
  let addEl: HTMLDivElement | undefined = $state();

  // Header / legend / coachmark UI state.
  let legendOpen = $state(false);
  let finderOpen = $state(false);
  let finderQuery = $state('');
  let finderKind = $state('');
  let finderInput: HTMLInputElement | null = $state(null);
  let finderButton: HTMLButtonElement | null = $state(null);
  let colorMode = $state<'light' | 'dark'>('dark');
  let reducedMotion = $state(false);
  let graphError = $state('');
  let loading = $state(false);
  let previousLayout = $state<Record<string,{x:number;y:number}> | null>(null);
  const finderItems = $derived(nodes.filter(node => (!finderKind || node.data?.kind===finderKind) && `${node.data?.label ?? ''} ${KIND_LABELS[String(node.data?.kind)] ?? ''}`.toLocaleLowerCase().includes(finderQuery.trim().toLocaleLowerCase())).sort((a,b)=>String(a.data?.label).localeCompare(String(b.data?.label))));
  $effect(()=>{if(finderOpen)void tick().then(()=>finderInput?.focus());});
  function closeFinder(){finderOpen=false;finderButton?.focus();}
  function findItem(id:string){if(focusNode(id))closeFinder();}
  function fitAll(){flow?.fitView({duration:reducedMotion?0:250,padding:.2,maxZoom:1.1,minZoom:.1});}
  function focusConnections(){if(!panelNode)return;const ids=new Set([panelNode.id]);for(const e of edges){if(e.source===panelNode.id)ids.add(e.target);if(e.target===panelNode.id)ids.add(e.source);}flow?.fitView({nodes:[...ids].map(id=>({id})),duration:reducedMotion?0:250,padding:.25,maxZoom:1.2});}
  function undoArrange(){if(!previousLayout)return;const positions=previousLayout;nodes=nodes.map(n=>({...n,position:positions[n.id]??n.position}));savePositions(nodes);previousLayout=null;fitAll();}
  // The node whose detail panel is open (docked inside this canvas pane). Driven
  // by clicks/context-menu so the panel never covers the chat or the minimap.
  let panelNode = $state<Node | null>(null);
  // True once /graph has answered at least once (so the header reads honestly).
  let loaded = $state(false);

  const nodeTypes = { entity: EntityNode };

  // Currently hovered edge id — used to reveal that edge's relationship label
  // even when nothing is selected.
  let hoveredEdge = $state<string | null>(null);
  // Last edge clicked — tracked here so the highlight/label + keyboard-detach
  // gesture don't depend on xyflow's internal edge selection (which our restyle
  // effect already overwrites with our own `selected` flag).
  let selectedEdge = $state<string | null>(null);

  // Which node's relationships to highlight. Driven by the docked panel's node.
  const selectedNodeId = $derived(panelNode?.id ?? null);

  /** Pure restyle pass: edges incident to the highlighted node stay vivid +
   * labelled + animated, the rest dim; a hovered/clicked edge reveals its
   * label. Color/arrowheads come from the relationship type. Operates on the
   * given list so it can run both reactively and right after a refresh. */
  function styleEdges(list: Edge[], sel: string | null, hov: string | null, selE: string | null): Edge[] {
    return list.map((e) => {
      const rel = String((e.data as any)?.rel ?? '');
      const color = edgeColor(rel || undefined);
      const incident = !sel || e.source === sel || e.target === sel;
      const dim = sel != null && !incident;
      const active = e.id === hov || e.id === selE;
      const showLabel = ((sel != null && incident) || active) && !!rel;
      return {
        ...e,
        selected: e.id === selE,
        label: showLabel ? rel : undefined,
        // The Svelte edge label is an HTML div; labelStyle is applied as its
        // style string, so the chip background lives here (React-flow's
        // labelBgStyle/labelBgPadding props are ignored by Svelte Flow).
        labelStyle:
          'color: var(--foreground); font-size: 12px; padding: 2px 6px; border-radius: 6px; ' +
          'background: var(--popover); border: 1px solid ' +
          color +
          '; white-space: nowrap;',
        animated: false,
        style: `stroke: ${color}; stroke-opacity: ${dim ? 0.12 : 1}; stroke-width: ${
          (incident && sel != null) || active ? 2 : 1.5
        }px;`,
        markerEnd: { ...(e.markerEnd as any), color, width: 16, height: 16 }
      } as Edge;
    });
  }

  // Restyle edges reactively WITHOUT replacing the bound array out from under
  // xyflow: this effect depends only on the highlight inputs (selection / hover
  // / clicked edge) and reads `edges` via untrack, so xyflow's own writes to
  // `edges` (geometry, internal selection) never retrigger it — no feedback
  // loop.
  $effect(() => {
    const sel = selectedNodeId;
    const hov = hoveredEdge;
    const selE = selectedEdge;
    untrack(() => {
      if (edges.length) edges = styleEdges(edges, sel, hov, selE);
    });
  });

  export async function refresh() {
    loading=true;graphError='';
    try {
      const g: ApiGraph = await get('/graph');
      const { nodes: n, edges: e } = toFlow(g);
      const firstLoad = !loaded;
      // Pinned (saved) nodes keep their spots; fresh nodes get collision-free
      // dagre positions. After autoArrange clears the saved store, this is a
      // pure dagre layout until the user drags a node again.
      nodes = layoutFresh(n, e, loadPositions()).map(node=>({...node,selected:node.id===selectedNodeId}));
      // Re-apply the current highlight so a background refresh (SSE job update)
      // while a node panel is open doesn't flash the dimming/labels away.
      edges = styleEdges(e, selectedNodeId, hoveredEdge, selectedEdge);
      if(panelNode){const fresh=nodes.find(n=>n.id===panelNode?.id)??null;panelNode=fresh;if(externalPanel)onselect(fresh);}
      loaded = true;
      if(firstLoad && n.length)focusNewestScene();
    } catch (err: any) {
      graphError = `Could not load the canvas. ${err.message}`;
      loaded = true;
    } finally { loading=false; }
  }

  export function autoArrange() {
    if(!nodes.length)return;
    previousLayout=Object.fromEntries(nodes.map(n=>[n.id,{...n.position}]));
    clearPositions();
    nodes = layout(nodes, edges);
    savePositions(nodes);
    fitAll();
  }

  export function focusNode(id: string): boolean {
    const node=nodes.find(n=>n.id===id);
    if(!node || !selectNode(node))return false;
    flow?.fitView({nodes:[{id}],duration:reducedMotion?0:250,maxZoom:1.2,padding:.3});
    return true;
  }

  /** Open the canvas centred on the most recent scene cluster rather than
   * fit-all (which shrinks a busy graph into illegible specks). Falls back to
   * the plain fitView if there are no scenes yet. */
  function focusNewestScene() {
    const scenes = nodes.filter((n) => String(n.data?.kind ?? '') === 'scene');
    if (!scenes.length) return;
    // "Newest" = the scene placed lowest-then-rightmost by the layout, a decent
    // proxy for most-recently-added without a created_at on the graph node.
    const newest = scenes.reduce((a, b) =>
      b.position.y > a.position.y || (b.position.y === a.position.y && b.position.x > a.position.x)
        ? b
        : a
    );
    // Pull in the scene plus its direct neighbours so the cluster reads as a unit.
    const around = new Set<string>([newest.id]);
    for (const e of edges) {
      if (e.source === newest.id) around.add(e.target);
      if (e.target === newest.id) around.add(e.source);
    }
    flow?.fitView({
      nodes: [...around].map((id) => ({ id })),
      duration: reducedMotion ? 0 : 250,
      padding: 0.25,
      maxZoom: 1.1
    });
  }

  onMount(() => {
    const syncTheme=()=>colorMode=document.documentElement.classList.contains('dark')?'dark':'light';
    syncTheme();const observer=new MutationObserver(syncTheme);observer.observe(document.documentElement,{attributes:true,attributeFilter:['class']});
    const motion=window.matchMedia('(prefers-reduced-motion: reduce)');const syncMotion=()=>reducedMotion=motion.matches;syncMotion();motion.addEventListener('change',syncMotion);
    refresh();
    return()=>{observer.disconnect();motion.removeEventListener('change',syncMotion);};
  });

  export function clearSelection(){selectNode(null);}

  function selectNode(node: Node | null) {
    if (beforeselect && !beforeselect(node)) {nodes=nodes.map(n=>({...n,selected:n.id===panelNode?.id}));return false;}
    panelNode = node;
    nodes=nodes.map(n=>({...n,selected:n.id===node?.id}));
    onselect(node);
    return true;
  }

  function kindOf(id: string): string {
    return String(nodes.find((n) => n.id === id)?.data?.kind ?? '');
  }

  async function handleConnect(conn: Connection) {
    const sk = kindOf(conn.source);
    const tk = kindOf(conn.target);
    try {
      if (sk === 'character' && tk === 'scene') {
        await post(`/scenes/${conn.target}/cast/${conn.source}`);
      } else if (sk === 'scene' && tk === 'character') {
        await post(`/scenes/${conn.source}/cast/${conn.target}`);
      } else if (sk === 'asset' && tk === 'shot') {
        await post(`/shots/${conn.target}/assets/${conn.source}`);
      } else if (sk === 'shot' && tk === 'asset') {
        await post(`/shots/${conn.source}/assets/${conn.target}`);
      } else {
        // Invalid pair: the dropped edge was never committed to the DOM by
        // xyflow's onconnect path the way a node delete is, so there's nothing
        // to undo — just tell the user. No refresh() (it would needlessly
        // re-fetch and reset the viewport on a no-op).
        toast.error('Connect character↔scene or asset↔shot');
        return;
      }
      toast.success('Connected');
      await refresh();
    } catch (err: any) {
      toast.error(err.message);
      await refresh();
    }
  }

  /** Detach a single relationship edge. Shared by the keyboard gesture and the
   * native ondelete path. Returns true if it mapped to a real detach. */
  async function detachEdge(edge: Edge): Promise<boolean> {
    const sk = kindOf(edge.source);
    const tk = kindOf(edge.target);
    if (sk === 'scene' && tk === 'character') {
      await del(`/scenes/${edge.source}/cast/${edge.target}`);
    } else if (sk === 'character' && tk === 'scene') {
      await del(`/scenes/${edge.target}/cast/${edge.source}`);
    } else if (sk === 'shot' && tk === 'asset') {
      await del(`/shots/${edge.source}/assets/${edge.target}`);
    } else if (sk === 'asset' && tk === 'shot') {
      await del(`/shots/${edge.target}/assets/${edge.source}`);
    } else {
      return false;
    }
    return true;
  }

  async function handleDelete({
    nodes: deletedNodes,
    edges: deleted
  }: {
    nodes: Node[];
    edges: Edge[];
  }) {
    // With deleteKey disabled, this path only fires for programmatic deletes; we
    // keep edge handling here so the relationship-detach UX stays intact.
    if (deletedNodes.length) {
      await refresh();
      return;
    }
    let changed = false;
    for (const edge of deleted) {
      try {
        if (await detachEdge(edge)) changed = true;
        else toast.error('Only character casts and shot assets can be detached');
      } catch (err: any) {
        toast.error(err.message);
      }
    }
    if (changed) toast.success('Detached');
    await refresh();
  }

  function openContextMenu({ node, event }: { node: Node; event: MouseEvent }) {
    event.preventDefault();
    menu = { node, x: event.clientX, y: event.clientY };
  }

  /** Delete/Backspace gesture: route the selected node into the SAME
   * confirm-delete flow the context menu uses (no error toast), and let a
   * selected edge fall through to the detach flow. Ignored while typing. */
  function onWindowKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      closeAddMenu();
      if(finderOpen)closeFinder();
      legendOpen=false;
      return;
    }
    if (e.key !== 'Delete' && e.key !== 'Backspace') return;
    if (!canvasRoot?.offsetWidth || !canvasRoot?.offsetHeight || finderOpen) return;
    const t = e.target as HTMLElement | null;
    if (t && t !== document.body && !canvasRoot.contains(t)) return;
    if (
      t &&
      (t.tagName === 'SELECT' || t.closest('button,a') || t.tagName === 'INPUT' ||
        t.tagName === 'TEXTAREA' ||
        t.isContentEditable ||
        t.closest('[data-testid="canvas-context-menu"]'))
    ) {
      return;
    }
    const selNode = nodes.find((n) => n.selected) ?? panelNode ?? null;
    if (selNode) {
      e.preventDefault();
      const kind = String(selNode.data?.kind ?? '');
      // render_job / output have no delete affordance in the context menu, so
      // the gesture is a no-op for them rather than a confusing error.
      if (kind === 'render_job' || kind === 'output') return;
      menu = {
        node: selNode,
        x: window.innerWidth / 2,
        y: window.innerHeight / 2,
        view: 'confirm-delete'
      };
      return;
    }
    if (selectedEdge) {
      const edge = edges.find((ed) => ed.id === selectedEdge);
      if (edge) {
        e.preventDefault();
        selectedEdge = null;
        void handleDelete({ nodes: [], edges: [edge] });
      }
    }
  }

  function closeAddMenu() {
    addOpen = false;
    addMode = 'root';
    addName = '';
  }

  async function createEntity() {
    const name = addName.trim();
    if (!name || addBusy) return;
    addBusy = true;
    try {
      const created =
        addMode === 'scene'
          ? await post('/scenes', { title: name })
          : await post('/characters', { name });
      toast.success(`${addMode === 'scene' ? 'Scene' : 'Character'} "${name}" created`);
      closeAddMenu();
      await refresh();
      focusNode(created.id);
    } catch (err: any) {
      toast.error(err.message);
    } finally {
      addBusy = false;
    }
  }

  function addClickAway(e: PointerEvent) {
    if (addOpen && addEl && !addEl.contains(e.target as globalThis.Node)) closeAddMenu();
  }
</script>

<!-- relative wrapper: anchors the docked NodePanel as an absolute card *inside*
  the canvas pane, so it never overlaps the chat composer or the minimap. -->
<div bind:this={canvasRoot} class="canvas-surface relative h-full w-full" aria-busy={loading}>
  <SvelteFlow
    bind:nodes
    bind:edges
    {nodeTypes}
    connectionMode={ConnectionMode.Loose}
    fitView
    fitViewOptions={{ padding: 0.2, maxZoom: 1.1 }}
    minZoom={0.1}
    deleteKey={null}
    {colorMode}
    proOptions={{ hideAttribution: true }}
    onconnect={handleConnect}
    ondelete={handleDelete}
    onnodeclick={({ node }) => {
      selectedEdge = null;
      selectNode(node);
    }}
    onnodecontextmenu={openContextMenu}
    onpaneclick={() => {
      selectedEdge = null;
      selectNode(null);
    }}
    onedgeclick={({ edge }) => (selectedEdge = edge.id)}
    onedgepointerenter={({ edge }) => (hoveredEdge = edge.id)}
    onedgepointerleave={() => (hoveredEdge = null)}
    onnodedragstop={() => savePositions(nodes)}
  >
    <FlowHelper register={(f) => (flow = f)} />

    <Panel position="top-left" class="canvas-toolbar z-10">
      <div class="rounded-lg border border-border bg-card p-2">
        <div class="flex flex-wrap items-center gap-2">
          <span class="mr-auto text-sm font-semibold">Story canvas <span class="font-normal text-muted-foreground">{loaded ? `· ${nodes.length}` : ''}</span></span>
          <Button bind:ref={finderButton} variant="ghost" size="sm" aria-label="Find canvas items" aria-expanded={finderOpen} aria-controls="canvas-finder" onclick={()=>{finderOpen=!finderOpen;closeAddMenu();}}><Search class="size-4" />Find</Button>
          <div class="relative" bind:this={addEl}>
            <Button
              variant="ghost"
              size="sm"
              title="Add entity" aria-label="Add canvas item" aria-expanded={addOpen} aria-controls="canvas-add-menu"
              data-testid="canvas-add"
              onclick={() => (addOpen ? closeAddMenu() : (addOpen = true))}
            >
              <Plus class="size-4" />Add
            </Button>
            {#if addOpen}
              <div
                class="absolute right-0 top-10 z-20 w-52 rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-md"
                id="canvas-add-menu" data-testid="canvas-add-menu"
              >
                {#if addMode === 'root'}
                  <button
                    class="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-xs hover:bg-accent"
                    onclick={() => (addMode = 'scene')}
                  >
                    <Clapperboard class="size-3.5" />New scene
                  </button>
                  <button
                    class="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-xs hover:bg-accent"
                    onclick={() => (addMode = 'character')}
                  >
                    <Users class="size-3.5" />New character
                  </button>
                  <a
                    class="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-xs hover:bg-accent"
                    href="/assets"
                  >
                    <Image class="size-3.5" />Upload asset
                  </a>
                {:else}
                  <button
                    class="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-xs hover:bg-accent"
                    onclick={() => {
                      addMode = 'root';
                      addName = '';
                    }}
                  >
                    <ChevronLeft class="size-3.5" />Back
                  </button>
                  <div class="flex items-center gap-1 p-1">
                    <!-- svelte-ignore a11y_autofocus -->
                    <Input
                      class="h-7 text-xs"
                      aria-label={addMode === 'scene' ? 'Scene title' : 'Character name'}
                      placeholder={addMode === 'scene' ? 'Scene title' : 'Character name'}
                      autofocus
                      bind:value={addName}
                      onkeydown={(e: KeyboardEvent) => e.key === 'Enter' && createEntity()}
                    />
                    <Button
                      size="sm"
                      class="h-7 px-2 text-xs"
                      disabled={!addName.trim() || addBusy}
                      onclick={createEntity}
                    >
                      {addBusy ? '…' : 'Add'}
                    </Button>
                  </div>
                {/if}
              </div>
            {/if}
          </div>

        </div>
        <div class="mt-1 flex flex-wrap items-center gap-1 border-t border-border pt-1">
          <Button variant="ghost" size="sm" disabled={!nodes.length} onclick={fitAll} title="Show the whole project"><Maximize2 class="size-3.5" />Fit all</Button>
          <Button variant="ghost" size="sm" disabled={!nodes.length} onclick={autoArrange} title="Organize items without changing their relationships"><LayoutGrid class="size-3.5" />Arrange</Button>
          {#if previousLayout}<Button variant="ghost" size="sm" aria-label="Undo arrange" onclick={undoArrange}><Undo2 class="size-3.5" />Undo</Button>{/if}
          <Button variant="ghost" size="sm" aria-expanded={legendOpen} onclick={()=>legendOpen=!legendOpen}>Guide</Button>
        </div>
        {#if panelNode}<div class="mt-1 flex items-center gap-2 border-t border-border pt-2"><p class="min-w-0 flex-1 truncate text-xs" title={String(panelNode.data?.label)}>{KIND_LABELS[String(panelNode.data?.kind)]} · {panelNode.data?.label}</p><Button variant="ghost" size="sm" onclick={focusConnections} aria-label="Focus selected item and connections"><Focus class="size-3.5" />Connections</Button></div>{/if}
        {#if graphError}<div role="alert" class="mt-2 space-y-2 border-t border-border pt-2 text-sm"><p>{graphError}</p>{#if nodes.length}<p class="text-xs text-muted-foreground">Showing the last loaded items.</p>{/if}<Button variant="outline" size="sm" onclick={refresh}>Try again</Button></div>{:else if loading}<p role="status" class="mt-2 text-xs text-muted-foreground">Loading canvas…</p>{/if}
      </div>
      {#if finderOpen}
        <section id="canvas-finder" aria-label="Find canvas items" class="mt-2 rounded-lg border border-border bg-popover p-3 text-popover-foreground">
          <div class="flex items-center gap-2"><Input bind:ref={finderInput} aria-label="Find canvas items" type="search" placeholder="Find a scene, character or asset…" bind:value={finderQuery}/><Button variant="ghost" size="icon-sm" aria-label="Close canvas finder" onclick={closeFinder}><X class="size-4" /></Button></div>
          <div class="my-2 flex items-center gap-2"><select aria-label="Canvas item type" bind:value={finderKind} class="h-9 min-w-0 flex-1 rounded-md border border-input bg-background px-2 text-xs"><option value="">All item types</option>{#each LEGEND_KINDS as kind}<option value={kind}>{KIND_LABELS[kind]}</option>{/each}</select>{#if finderQuery || finderKind}<Button variant="ghost" size="sm" aria-label="Clear search" onclick={()=>{finderQuery='';finderKind='';finderInput?.focus();}}>Clear</Button>{/if}</div>
          <p role="status" class="mb-1 text-xs text-muted-foreground">{finderItems.length} {finderItems.length===1?'item':'items'}</p>
          <div class="max-h-56 overflow-y-auto">{#each finderItems as node (node.id)}<button type="button" class="flex min-h-11 w-full items-center gap-2 rounded-md px-2 text-left text-sm hover:bg-accent" aria-label={`${node.data?.label} · ${KIND_LABELS[String(node.data?.kind)]}`} aria-pressed={panelNode?.id===node.id} onclick={()=>findItem(node.id)}><span class="min-w-0 flex-1 truncate">{node.data?.label}</span><span class="text-xs text-muted-foreground">{KIND_LABELS[String(node.data?.kind)]}</span></button>{:else}<p class="py-3 text-sm text-muted-foreground">No matching items.</p>{/each}</div>
        </section>
      {/if}
      {#if legendOpen}<section aria-label="Canvas guide" class="mt-2 space-y-3 rounded-lg border border-border bg-popover p-3 text-sm text-popover-foreground"><p>Select an item to edit it. Drag the background to move around; use Fit all to return.</p><p class="text-xs text-muted-foreground">Connect a character to a scene, or an asset to a shot, by dragging between the side handles. Right-click an item for more actions.</p><div class="flex flex-wrap gap-x-4 gap-y-2">{#each LEGEND_KINDS as kind}<span class="flex items-center gap-2 text-xs"><span class="size-2 rounded-full" style={`background:${kindColor(kind)}`}></span>{KIND_LABELS[kind]}</span>{/each}</div></section>{/if}
    </Panel>

    <Background />
    <Controls showFitView={false} />
    {#if nodes.length > 8}<MiniMap class="canvas-minimap" pannable zoomable nodeColor={(n) => kindColor(String(n.data?.kind ?? ''))} />{/if}
  </SvelteFlow>

  <!-- Docked detail panel: absolutely positioned inside this pane (top-right),
    NOT a viewport-fixed full-height slab, so the chat + minimap stay reachable. -->
  {#if !externalPanel}<NodePanel
    node={panelNode}
    onclose={() => selectNode(null)}
    onsaved={refresh}
  />{/if}
</div>

<svelte:window onpointerdown={addClickAway} onkeydown={onWindowKeydown} />

{#if menu}
  <CanvasMenu
    node={menu.node}
    x={menu.x}
    y={menu.y}
    initialView={menu.view ?? 'root'}
    {nodes}
    {edges}
    onclose={() => (menu = null)}
    onselect={selectNode}
    onmutate={refresh}
  />
{/if}

<style>
  .canvas-surface :global(.canvas-toolbar) { width: min(380px, calc(100% - 24px)); }
  .canvas-surface :global(.canvas-minimap) { width: 130px; height: 90px; }
  @media(max-width: 640px) { .canvas-surface :global(.canvas-minimap) { display:none; } }
</style>
