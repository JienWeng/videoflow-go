<script lang="ts">
  import '../app.css';
  import CreateStoryDialog from '$lib/create/CreateStoryDialog.svelte';
  import {storyCreation} from '$lib/create/story-creation.svelte';
  import BrandLogo from '$lib/shell/BrandLogo.svelte';
  import { afterNavigate, goto } from '$app/navigation';
  import { Button } from '$lib/components/ui/button';
  import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
  import * as Dialog from '$lib/components/ui/dialog';
  import { Menu, Search, PanelLeftClose, PanelLeftOpen, Ellipsis, Compass, Receipt } from '@lucide/svelte';
  import GuidePanel from '$lib/shell/GuidePanel.svelte';
  import Tour from '$lib/shell/Tour.svelte';
  import { guide } from '$lib/shell/guide.svelte';
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
  let sidebarCollapsed=$state(false);
  function toggleSidebar(){sidebarCollapsed=!sidebarCollapsed;try{localStorage.setItem('videoflow.sidebar.collapsed',sidebarCollapsed?'1':'0');}catch{/* Keep the current layout when storage is unavailable. */}}

  onMount(() => {
    try{sidebarCollapsed=localStorage.getItem('videoflow.sidebar.collapsed')==='1';}catch{/* Expanded is the safe default. */}
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
    let latest=0;
    const refresh=()=>{const request=++latest;get('/projects/active').then(p=>{if(request===latest)activeProject=p}).catch(()=>{})};
    refresh();
    window.addEventListener('videoflow:projects-changed',refresh);
    return ()=>{latest++;window.removeEventListener('videoflow:projects-changed',refresh)};
  });

  let navigationOpen = $state(false);
  afterNavigate(() => { navigationOpen = false;storyCreation.open=false; });
  const navGroups = [{label:'Workspace',items:[{href:'/',label:'Studio',icon:Clapperboard},{href:'/library',label:'Library',icon:FolderOpen},{href:'/render',label:'Videos',icon:Film},{href:'/costs',label:'Costs',icon:Receipt}]}];
  // The editor is contextual (reached from a render output), so it highlights Render.
  function isActive(href: string, pathname: string): boolean {
    if (href === '/') return pathname === '/' || pathname === '/director';
    if(href==='/library')return ['/library','/characters','/assets','/voices'].some(path=>pathname===path);
    if (href === '/render') return pathname.startsWith('/render') || pathname === '/compositions';
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
  {#snippet navigation(compact=false)}
    <a href="/projects" aria-label={activeProject?.name??'Choose project'} title={compact?(activeProject?.name??'Choose project'):undefined} class="mb-5 flex min-h-11 items-center gap-2 rounded-lg border border-border text-sm hover:bg-accent {compact?'justify-center px-0':'px-3'}" aria-current={page.url.pathname==='/projects'?'page':undefined}>
      <FolderOpen class="size-4 shrink-0" /><span class={compact?'sr-only':'truncate'}>{activeProject?.name ?? 'Choose project'}</span>
    </a>
    <nav aria-label="Primary" class="space-y-6">
      {#each navGroups as group}
        <div><p class={compact?'sr-only':'mb-2 px-3 text-xs font-medium text-muted-foreground'}>{group.label}</p>
          {#each group.items as item}
            <a href={item.href} data-tour={item.href==='/render'?'nav-videos':undefined} aria-label={item.label} title={compact?item.label:undefined} aria-current={isActive(item.href,page.url.pathname)?'page':undefined} class="flex min-h-11 items-center gap-3 rounded-lg text-sm {compact?'justify-center px-0':'px-3'} {isActive(item.href,page.url.pathname)?'bg-accent font-semibold':'text-muted-foreground hover:bg-accent hover:text-foreground'}"><item.icon class="size-4 shrink-0" /><span class={compact?'sr-only':''}>{item.label}</span></a>
          {/each}
        </div>
      {/each}
    </nav>
    <a href="/settings" aria-label="Settings" title={compact?'Settings':undefined} aria-current={isActive('/settings',page.url.pathname)?'page':undefined} class="mt-6 flex min-h-11 items-center gap-3 rounded-lg text-sm hover:bg-accent {compact?'justify-center px-0':'px-3'} {isActive('/settings',page.url.pathname)?'bg-accent font-semibold':'text-muted-foreground'}"><SlidersHorizontal class="size-4 shrink-0" /><span class={compact?'sr-only':''}>Settings</span></a>
  {/snippet}
  <header class="flex h-14 shrink-0 items-center gap-1 border-b border-border px-2 md:hidden">
    <Dialog.Root bind:open={navigationOpen}>
      <Dialog.Trigger><span class="inline-flex min-h-11 min-w-11 items-center justify-center rounded-lg hover:bg-accent" aria-label="Open navigation"><Menu class="size-5" /><span class="sr-only">Open navigation</span></span></Dialog.Trigger>
      <Dialog.Content class="max-h-[90dvh] overflow-y-auto"><Dialog.Header><Dialog.Title>Navigation</Dialog.Title><Dialog.Description>Switch workspace or open your project tools.</Dialog.Description></Dialog.Header>{@render navigation()}</Dialog.Content>
    </Dialog.Root>
    <a href="/" aria-label="VideoFlow" class="inline-flex min-h-11 min-w-11 items-center justify-center"><BrandLogo class="size-8" /></a>
    <div class="ml-auto flex items-center gap-0.5">
      <Button variant="ghost" size="icon" class="size-11" aria-label="Guide" onclick={() => guide.openPanel()}><Compass class="size-5" /></Button><Button variant="ghost" size="icon" class="size-11" aria-label="Search" onclick={() => palette.toggle()}><Search class="size-5" /></Button>
      <div class="w-11"><ActivityTray compact /></div>
    </div>
  </header>
  <aside aria-label="Sidebar" class="hidden shrink-0 flex-col border-r border-border bg-sidebar transition-[width,padding] duration-150 motion-reduce:transition-none md:flex {sidebarCollapsed?'w-16 p-2':'w-56 p-4'}">
    <div class="mb-5 flex min-h-11 items-center gap-1 {sidebarCollapsed?'flex-col':''}">
      <a href="/" aria-label="VideoFlow" title="VideoFlow" class="flex min-h-11 min-w-0 items-center {sidebarCollapsed?'w-full justify-center':'flex-1 px-1'}"><BrandLogo class={sidebarCollapsed?'size-8':'size-11'} /></a>
      <Button variant="ghost" size="icon" class="size-11 shrink-0" aria-label={sidebarCollapsed?'Expand sidebar':'Collapse sidebar'} title={sidebarCollapsed?'Expand sidebar':'Collapse sidebar'} aria-expanded={!sidebarCollapsed} aria-controls="desktop-sidebar-navigation" onclick={toggleSidebar}>{#if sidebarCollapsed}<PanelLeftOpen class="size-4" />{:else}<PanelLeftClose class="size-4" />{/if}</Button>
    </div>
    <div id="desktop-sidebar-navigation" class="min-h-0 flex-1 overflow-y-auto">{@render navigation(sidebarCollapsed)}</div>
    <div class="mt-3 border-t border-border pt-3"><Button variant="ghost" data-tour="guide" aria-label="Guide" title={sidebarCollapsed?'Guide: checklist, tour and tips':undefined} class="mb-1 min-h-11 w-full text-muted-foreground {sidebarCollapsed?'justify-center px-0':'justify-start'}" onclick={()=>guide.openPanel()}><Compass class="size-4 shrink-0" /><span class={sidebarCollapsed?'sr-only':''}>Guide</span></Button><Button variant="ghost" aria-label="Search" title={sidebarCollapsed?'Search (Ctrl/⌘ K)':undefined} class="mb-2 min-h-11 w-full text-muted-foreground {sidebarCollapsed?'justify-center px-0':'justify-start'}" onclick={()=>palette.toggle()}><Search class="size-4 shrink-0" /><span class={sidebarCollapsed?'sr-only':''}>Search</span>{#if !sidebarCollapsed}<kbd class="ml-auto text-xs">⌘ K</kbd>{/if}</Button><div data-tour="nav-activity"><ActivityTray compact={sidebarCollapsed} /></div></div>
  </aside>
  <main class="min-h-0 min-w-0 flex-1 overflow-auto">
    {@render children?.()}
  </main>
</div>

<svelte:window onkeydown={onKeydown} />

<GuidePanel />
<Tour />
<!-- Single app-wide palette instance. Studio-only callbacks are read from the
  store (set by the Studio page on mount); off Studio they are undefined and
  the palette falls back to navigation. -->
<CommandPalette
  bind:open={palette.open}
  onfocus={(id) => palette.handlers.onfocus?.(id)}
  onnewstory={() => storyCreation.show()}
/>

<CreateStoryDialog />
<Toaster richColors />
