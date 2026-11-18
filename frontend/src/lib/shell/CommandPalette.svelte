<script lang="ts">
  /**
   * Cmd/Ctrl+K command palette — the keyboard-first way around the Studio.
   *
   * Fuzzy-searches the active project's graph (scenes, characters, assets,
   * renders) plus a fixed set of top actions (new story, navigate to any
   * surface, open Settings, switch project). Built on the shadcn Dialog + a
   * plain filtered list — no new dependency, no shadcn Command component.
   *
   * Navigation is the one sanctioned place we drive the router on the user's
   * behalf (they explicitly picked a destination). Picking a scene/character/
   * asset/render routes to its surface; picking "open a scene" on the canvas
   * focuses the node via the onfocus callback instead of navigating.
   */
  import { goto } from '$app/navigation';
  import { get } from '$lib/api';
  import * as Dialog from '$lib/components/ui/dialog';
  import {
    Search,
    Clapperboard,
    Users,
    Image,
    Film,
    ListVideo,
    SlidersHorizontal,
    FolderOpen,
    Sparkles,
    Crosshair,
    CornerDownLeft
  } from '@lucide/svelte';

  let {
    open = $bindable(false),
    onfocus,
    onnewstory
  }: {
    open?: boolean;
    /** Focus a graph node already on the canvas (Studio only). */
    onfocus?: (id: string) => void;
    /** Seed the chat composer for a fresh story. */
    onnewstory?: () => void;
  } = $props();

  type Item = {
    id: string;
    label: string;
    hint?: string;
    group: string;
    icon: any;
    /** Either navigate to a route, focus a canvas node, or run a callback. */
    href?: string;
    focus?: string;
    run?: () => void;
  };

  let query = $state('');
  let active = $state(0);
  let inputEl = $state<HTMLInputElement | null>(null);

  // Project-scoped data, lazily (re)loaded each time the palette opens so the
  // results never go stale after a generation/render.
  let graph = $state<any>(null);
  let projects = $state<any[]>([]);
  let loaded = $state(false);

  async function load() {
    loaded = false;
    const [g, p] = await Promise.all([
      get('/graph').catch(() => null),
      get('/projects').catch(() => [])
    ]);
    graph = g;
    projects = p ?? [];
    loaded = true;
  }

  // Re-fetch + reset the query whenever the palette is opened. The dialog
  // mounts its content in a portal a frame later, so focus after a rAF (the
  // `autofocus` attribute is the belt; this is the braces for bits-ui's own
  // focus management moving it elsewhere).
  $effect(() => {
    if (open) {
      query = '';
      active = 0;
      load();
      requestAnimationFrame(() => inputEl?.focus());
    }
  });

  const lastPath = (p: string | null | undefined) =>
    p ? p.split('/').pop() : undefined;

  /** The full action+entity catalogue, rebuilt as data arrives. */
  const items = $derived.by<Item[]>(() => {
    const out: Item[] = [];

    // --- Top actions -----------------------------------------------------
    out.push({
      id: 'act-new-story',
      label: 'Create story',
      hint: 'story, characters and storyboard',
      group: 'Actions',
      icon: Sparkles,
      run: () => onnewstory?.()
    });
    out.push({
      id: 'nav-studio',
      label: 'Go to Studio',
      group: 'Go to',
      icon: Clapperboard,
      href: '/?view=studio'
    });
    out.push({
      id: 'nav-scenes',
      label: 'Go to Story',
      group: 'Go to',
      icon: ListVideo,
      href: '/?view=story'
    });
    out.push({
      id: 'nav-render',
      label: 'Go to Videos',
      group: 'Go to',
      icon: Film,
      href: '/render'
    });
    out.push({
      id: 'nav-characters',
      label: 'Go to Characters',
      group: 'Go to',
      icon: Users,
      href: '/library?tab=characters'
    });
    out.push({
      id: 'nav-assets',
      label: 'Go to Media',
      group: 'Go to',
      icon: Image,
      href: '/library?tab=media'
    });
    out.push({
      id: 'nav-settings',
      label: 'Open Settings',
      group: 'Go to',
      icon: SlidersHorizontal,
      href: '/settings'
    });

    out.push({id:'nav-sessions',label:'Manage AI sessions',hint:'assistant setup and saved conversations',group:'Go to',icon:SlidersHorizontal,href:'/director'});
    out.push({id:'nav-voices',label:'Voice & dialogue',hint:'character voices, speech and script captions',group:'Go to',icon:Users,href:'/voices'});
    out.push({id:'nav-composition',label:'Edit video',hint:'timeline, captions, music and export',group:'Go to',icon:Film,href:'/?view=edit'});

    // --- Entities from the graph ----------------------------------------
    const nodes: any[] = graph?.nodes ?? [];
    for (const n of nodes) {
      if (n.type === 'scene') {
        out.push({
          id: `scene-${n.id}`,
          label: n.label || 'Untitled scene',
          hint: 'scene',
          group: 'Scenes',
          icon: ListVideo,
          // On the canvas we focus the node; elsewhere /scenes opens it.
          focus: n.id,
          href: '/?view=story'
        });
      } else if (n.type === 'character') {
        out.push({
          id: `char-${n.id}`,
          label: n.label || 'Character',
          hint: 'character',
          group: 'Characters',
          icon: Users,
          focus: n.id,
          href: '/library?tab=characters'
        });
      } else if (n.type === 'asset') {
        out.push({
          id: `asset-${n.id}`,
          label: n.label || 'Asset',
          hint: String(n.data?.asset_type ?? 'asset'),
          group: 'Assets',
          icon: Image,
          focus: n.id,
          href: '/library?tab=media'
        });
      } else if (n.type === 'output') {
        out.push({
          id: `out-${n.id}`,
          label: lastPath(n.data?.video_path) || n.label || 'Render',
          hint: 'render output',
          group: 'Renders',
          icon: Film,
          href: `/editor/${n.id}`
        });
      }
    }

    // --- Switch project --------------------------------------------------
    for (const p of projects) {
      if (p.is_active) continue;
      out.push({
        id: `proj-${p.id}`,
        label: `Switch to ${p.name}`,
        hint: 'project',
        group: 'Projects',
        icon: FolderOpen,
        run: () => switchProject(p.id)
      });
    }

    return out;
  });

  /** Subsequence ("vf" matches "VideoFlow") fuzzy test, case-insensitive. */
  function fuzzy(needle: string, haystack: string): boolean {
    const n = needle.toLowerCase();
    const h = haystack.toLowerCase();
    if (!n) return true;
    let i = 0;
    for (const ch of h) {
      if (ch === n[i]) i++;
      if (i === n.length) return true;
    }
    return false;
  }

  const filtered = $derived.by(() => {
    const q = query.trim();
    if (!q) return items;
    // Best matches first: a word in the title beats a word in the hint beats loose letters.
    const needle = q.toLowerCase();
    const score = (it: any): number => {
      const label = String(it.label).toLowerCase(), hint = String(it.hint ?? '').toLowerCase();
      if (label.startsWith(needle)) return 5;
      if (label.split(/[\s·\-–—/]+/).some((w) => w.startsWith(needle))) return 4;
      if (label.includes(needle)) return 3;
      if (hint.split(/[\s,·\-–—/]+/).some((w) => w.startsWith(needle))) return 2;
      if (needle.length <= 3 && fuzzy(q, label)) return 1; // initials like “vf”
      return 0;
    };
    const ranked = items
      .map((it, index) => ({ it, index, s: score(it) }))
      .filter((x) => x.s > 0)
      .sort((a, b) => b.s - a.s || a.index - b.index)
      .map((x) => x.it);
    // Sections are shown together, best-matching section first, so keyboard order matches the screen.
    const groupOrder: string[] = [];
    for (const it of ranked) if (!groupOrder.includes(it.group)) groupOrder.push(it.group);
    return groupOrder.flatMap((g) => ranked.filter((it) => it.group === g));
  });

  // Keep the active index in range as the filtered list shrinks.
  $effect(() => {
    if (active >= filtered.length) active = Math.max(0, filtered.length - 1);
  });

  async function switchProject(id: string) {
    open = false;
    // Let the current screen's navigation guard run before changing backend scope.
    // The Projects page consumes this navigation intent once, after departure succeeds.
    await goto('/projects', {state: {activateProject: id}});
  }

  function choose(it: Item | undefined) {
    if (!it) return;
    open = false;
    if (it.run) {
      it.run();
      return;
    }
    // On the Studio canvas, focusing beats navigating for graph entities.
    const onStudio =
      typeof window !== 'undefined' && window.location.pathname === '/';
    if (it.focus && onStudio && onfocus) {
      onfocus(it.focus);
      return;
    }
    if (it.href) goto(it.href);
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      active = Math.max(0,Math.min(active + 1, filtered.length - 1));
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      active = Math.max(active - 1, 0);
    } else if (e.key === 'Enter') {
      e.preventDefault();
      choose(filtered[active]);
    }
    requestAnimationFrame(() => document.getElementById(`command-result-${active}`)?.scrollIntoView({block:'nearest'}));
  }

  // Group the visible items in stable catalogue order for section headers.
  const grouped = $derived.by(() => {
    const order: string[] = [];
    const map = new Map<string, { item: Item; index: number }[]>();
    filtered.forEach((item, index) => {
      if (!map.has(item.group)) {
        map.set(item.group, []);
        order.push(item.group);
      }
      map.get(item.group)!.push({ item, index });
    });
    return order.map((g) => ({ group: g, rows: map.get(g)! }));
  });
</script>

<Dialog.Root bind:open>
  <Dialog.Content
    class="max-w-lg gap-0 overflow-hidden p-0 sm:max-w-lg"
    showCloseButton={false}
  >
    <Dialog.Title class="sr-only">Command palette</Dialog.Title>
    <Dialog.Description class="sr-only">
      Search scenes, characters, assets and renders, or jump to any surface.
    </Dialog.Description>

    <div class="flex items-center gap-2 border-b border-border px-3">
      <Search class="size-4 shrink-0 text-muted-foreground" />
      <!-- svelte-ignore a11y_autofocus -->
      <input
        bind:this={inputEl}
        bind:value={query}
        onkeydown={onKeydown}
        autofocus
        placeholder="Search or jump to… / 搜索"
        class="h-11 w-full border-0 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
        aria-label="Search commands"
        role="combobox"
        aria-autocomplete="list"
        aria-expanded="true"
        aria-controls="command-results"
        aria-activedescendant={filtered[active] ? `command-result-${active}` : undefined}
      />
    </div>

    <div id="command-results" role="listbox" aria-label="Commands and project items" class="max-h-80 overflow-y-auto py-1.5">
      {#if !loaded && !graph}
        <p class="px-3 py-6 text-center text-sm text-muted-foreground">Loading…</p>
      {:else if filtered.length === 0}
        <p class="px-3 py-6 text-center text-sm text-muted-foreground">
          No matches for "{query}"
        </p>
      {:else}
        {#each grouped as section (section.group)}
          <div class="px-3 pb-1 pt-2 text-[10px] font-medium uppercase tracking-wider text-muted-foreground/70">
            {section.group}
          </div>
          {#each section.rows as row (row.item.id)}
            {@const Icon = row.item.icon}
            <button
              type="button"
              id={`command-result-${row.index}`}
              role="option"
              tabindex="-1"
              aria-selected={row.index === active}
              class="flex w-full items-center gap-2.5 px-3 py-1.5 text-left text-sm
                     {row.index === active ? 'bg-accent' : 'hover:bg-accent/60'}"
              onmousemove={() => (active = row.index)}
              onclick={() => choose(row.item)}
            >
              <Icon class="size-4 shrink-0 text-muted-foreground" />
              <span class="truncate">{row.item.label}</span>
              {#if row.item.hint}
                <span class="ml-auto shrink-0 text-xs text-muted-foreground">{row.item.hint}</span>
              {/if}
              {#if row.item.focus}
                <Crosshair class="size-3 shrink-0 text-muted-foreground/60" />
              {/if}
            </button>
          {/each}
        {/each}
      {/if}
    </div>

    <div class="flex items-center gap-3 border-t border-border px-3 py-1.5 text-[11px] text-muted-foreground">
      <span class="flex items-center gap-1"><CornerDownLeft class="size-3" /> open</span>
      <span>↑↓ navigate</span>
      <span class="ml-auto">esc to close</span>
    </div>
  </Dialog.Content>
</Dialog.Root>
