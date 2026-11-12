<script lang="ts">
  import '../app.css';
  import { afterNavigate } from '$app/navigation';
  import { Button } from '$lib/components/ui/button';
  import * as Dialog from '$lib/components/ui/dialog';
  import { Menu, Search } from '@lucide/svelte';
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import {
    Clapperboard,
    Image,
    Users,
    ListVideo,
    Film,
    FolderOpen,
    SlidersHorizontal
  } from '@lucide/svelte';
  import { Toaster } from '$lib/components/ui/sonner';
  import ActivityTray from '$lib/components/ActivityTray.svelte';
  import CommandPalette from '$lib/shell/CommandPalette.svelte';
  import { palette } from '$lib/shell/palette.svelte';
  import { initActivity } from '$lib/activity.svelte';
  import { get } from '$lib/api';
  import { watchTheme } from '$lib/shell/theme';

  let { children } = $props();

  // The active project scopes every backend endpoint; switching does a full
  // page reload (see /projects), so fetching once on mount is always fresh.
  let activeProject: any = $state(null);

  onMount(() => {
    initActivity();
    // ModeWatcher-style: keep <html> in sync with the saved theme preference
    // (the Settings appearance toggle writes localStorage['videoflow.theme']).
    // app.html applied the initial value pre-paint; this reacts to changes.
    const stopTheme = watchTheme();
    return stopTheme;
  });

  $effect(() => {
    // SvelteKit soft navigation reuses the root layout after project activation.
    const path = page.url.pathname;
    void path;
    get('/projects/active').then((p) => (activeProject = p)).catch(() => {});
  });

  let navigationOpen = $state(false);
  afterNavigate(() => { navigationOpen = false; });
  const navGroups = [
    { label: 'Workspace', items: [
      { href: '/', label: 'Studio', icon: Clapperboard },
      { href: '/render', label: 'Videos', icon: Film }
    ]},
    { label: 'Library', items: [
      { href: '/characters', label: 'Characters', icon: Users },
      { href: '/assets', label: 'Assets', icon: Image },
      { href: '/scenes', label: 'Scenes & shots', icon: ListVideo }
    ]}
  ];
  const tools = [
    {href:'/create', label:'Guided creator'}, {href:'/voices',label:'Voices & captions'},
    {href:'/compositions',label:'Compositions'}, {href:'/director',label:'AI sessions'}
  ];
  // The editor is contextual (reached from a render output), so it highlights Render.
  function isActive(href: string, pathname: string): boolean {
    if (href === '/') return pathname === '/';
    if (href === '/render') return pathname.startsWith('/render') || pathname.startsWith('/editor');
    return pathname.startsWith(href);
  }

  // App-wide Cmd/Ctrl+K. Hoisted here (not the Studio page) so the palette is
  // reachable from every route — onboarding promises "press Ctrl/⌘ K anytime".
  function onKeydown(e: KeyboardEvent) {
    if ((e.metaKey || e.ctrlKey) && (e.key === 'k' || e.key === 'K')) {
      e.preventDefault();
      palette.toggle();
    }
  }
</script>

<div class="flex h-dvh min-w-0 flex-col bg-background text-foreground md:flex-row">
  {#snippet navigation()}
    <a href="/projects" class="mb-5 flex min-h-11 items-center gap-2 rounded-lg border border-border px-3 text-sm hover:bg-accent" aria-current={page.url.pathname==='/projects'?'page':undefined}>
      <FolderOpen class="size-4 shrink-0" /><span class="truncate">{activeProject?.name ?? 'Choose project'}</span>
    </a>
    <nav aria-label="Primary" class="space-y-6">
      {#each navGroups as group}
        <div><p class="mb-2 px-3 text-xs font-medium text-muted-foreground">{group.label}</p>
          {#each group.items as item}
            <a href={item.href} aria-current={isActive(item.href,page.url.pathname)?'page':undefined} class="flex min-h-11 items-center gap-3 rounded-lg px-3 text-sm {isActive(item.href,page.url.pathname)?'bg-accent font-semibold':'text-muted-foreground hover:bg-accent hover:text-foreground'}"><item.icon class="size-4" />{item.label}</a>
          {/each}
        </div>
      {/each}
      <details open={tools.some(t=>page.url.pathname.startsWith(t.href))} class="group">
        <summary class="min-h-11 cursor-pointer rounded-lg px-3 py-3 text-sm text-muted-foreground hover:bg-accent">More tools</summary>
        <div class="mt-1 border-l border-border pl-3 ml-3">{#each tools as tool}<a href={tool.href} aria-current={isActive(tool.href,page.url.pathname)?'page':undefined} class="flex min-h-11 items-center rounded-lg px-3 text-sm hover:bg-accent {isActive(tool.href,page.url.pathname)?'font-semibold':'text-muted-foreground'}">{tool.label}</a>{/each}</div>
      </details>
    </nav>
    <a href="/settings" aria-current={isActive('/settings',page.url.pathname)?'page':undefined} class="mt-6 flex min-h-11 items-center gap-3 rounded-lg px-3 text-sm hover:bg-accent {isActive('/settings',page.url.pathname)?'bg-accent font-semibold':'text-muted-foreground'}"><SlidersHorizontal class="size-4" />Settings</a>
  {/snippet}
  <header class="flex h-14 shrink-0 items-center justify-between border-b border-border px-3 md:hidden">
    <a href="/" class="font-semibold">VideoFlow</a>
    <Dialog.Root bind:open={navigationOpen}>
      <Dialog.Trigger><span class="inline-flex min-h-11 min-w-11 items-center justify-center rounded-lg border border-border" aria-label="Open navigation"><Menu class="size-5" /><span class="sr-only">Open navigation</span></span></Dialog.Trigger>
      <Dialog.Content class="max-h-[90dvh] overflow-y-auto"><Dialog.Header><Dialog.Title>Navigation</Dialog.Title><Dialog.Description>Switch workspace or open your project tools.</Dialog.Description></Dialog.Header>{@render navigation()}</Dialog.Content>
    </Dialog.Root>
    <div class="flex items-center gap-1">
      <Button variant="ghost" size="icon" aria-label="Search" onclick={() => palette.toggle()}><Search class="size-4" /></Button>
      <ActivityTray />
    </div>
  </header>
  <aside class="hidden w-56 shrink-0 flex-col border-r border-border bg-sidebar p-4 md:flex">
    <a href="/" class="px-3 pt-2 pb-6 text-lg font-semibold tracking-tight">VideoFlow</a>
    <div class="min-h-0 flex-1 overflow-y-auto">{@render navigation()}</div>
    <div class="mt-3 border-t border-border pt-3"><Button variant="ghost" class="mb-2 w-full justify-start text-muted-foreground" onclick={()=>palette.toggle()}><Search class="size-4" />Search <kbd class="ml-auto text-xs">⌘ K</kbd></Button><ActivityTray /></div>
  </aside>
  <main class="min-h-0 min-w-0 flex-1 overflow-auto">
    {@render children?.()}
  </main>
</div>

<svelte:window onkeydown={onKeydown} />

<!-- Single app-wide palette instance. Studio-only callbacks are read from the
  store (set by the Studio page on mount); off Studio they are undefined and
  the palette falls back to navigation. -->
<CommandPalette
  bind:open={palette.open}
  onfocus={(id) => palette.handlers.onfocus?.(id)}
  onnewstory={() => palette.handlers.onnewstory?.()}
/>

<Toaster richColors />
