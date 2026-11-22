<script lang="ts">
  import DeleteProject from '$lib/projects/DeleteProject.svelte';
  import ProjectStyle from '$lib/projects/ProjectStyle.svelte';
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { goto, invalidateAll, replaceState } from '$app/navigation';
  import { get, post, patch, mediaUrl } from '$lib/api';
  import { protectDraft } from '$lib/protect-draft';
  import { Button } from '$lib/components/ui/button';
  import { Badge } from '$lib/components/ui/badge';
  import { Input } from '$lib/components/ui/input';
  import { Skeleton } from '$lib/components/ui/skeleton';
  import * as Dialog from '$lib/components/ui/dialog';
  import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
  import { toast } from 'svelte-sonner';
  import { Folder, FolderPlus, LayoutGrid, List, Ellipsis, Upload } from '@lucide/svelte';
  import type { Project } from '$lib/api-types';
  let projects: Project[] = $state([]),
    loaded = $state(false),
    error = $state(''),
    busy = $state('');
  let search = $state(''),
    sort = $state('updated'),
    view = $state<'folders' | 'table'>('folders');
  let createOpen = $state(false),
    name = $state(''),
    description = $state(''),
    createError = $state(''),
    createdProject = $state<Project | null>(null);
  let renameTarget = $state<Project | null>(null),
    renameValue = $state(''),
    deleteTarget = $state<Project | null>(null);
  let importInput: HTMLInputElement | null = $state(null);
  // Details dialog: counts and the project look (the look can only be edited on the active project).
  let detailId = $state(''),
    styleSaving = $state(false);
  const detail = $derived(projects.find(p => p.id === detailId));
  const filtered = $derived(
    projects
      .filter(p => `${p.name} ${p.description ?? ''}`.toLocaleLowerCase().includes(search.trim().toLocaleLowerCase()))
      .slice()
      .sort((a, b) =>
        sort === 'name'
          ? a.name.localeCompare(b.name)
          : String(b[sort === 'created' ? 'created_at' : 'updated_at'] ?? b.created_at ?? '').localeCompare(
              String(a[sort === 'created' ? 'created_at' : 'updated_at'] ?? a.created_at ?? '')
            )
      )
  );
  const fmtDate = (s: string) =>
    s && !Number.isNaN(Date.parse(s))
      ? new Date(s).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
      : '—';
  const plural = (n: number | null | undefined, word: string) => (n == null ? `— ${word}s` : `${n} ${word}${n === 1 ? '' : 's'}`);
  const countsLine = (p: Project) =>
    `${plural(p.counts?.scenes, 'scene')} · ${plural(p.counts?.characters, 'character')} · ${plural(p.counts?.assets, 'image')}`;
  protectDraft(
    () =>
      (createOpen && !!(name.trim() || description.trim()) && !createdProject) ||
      (!!renameTarget && renameValue !== renameTarget.name)
  );
  async function refresh() {
    error = '';
    try {
      const data = await get<Project[] | null>('/projects');
      if (!Array.isArray(data)) throw Error('Project list is unavailable');
      projects = data;
    } catch (e) {
      error = String(e);
    } finally {
      loaded = true;
    }
  }
  onMount(() => {
    try {
      view = localStorage.getItem('videoflow.projects.view') === 'table' ? 'table' : 'folders';
    } catch {}
    void refresh();
  });
  function setView(next: 'folders' | 'table') {
    view = next;
    try {
      localStorage.setItem('videoflow.projects.view', next);
    } catch {}
  }
  async function run(key: string, fn: () => Promise<unknown>) {
    if (busy) return;
    busy = key;
    try {
      await fn();
    } catch (e) {
      toast.error(String(e));
    } finally {
      busy = '';
    }
  }
  function changeCreateOpen(open: boolean) {
    if (busy) return;
    if (
      !open &&
      (name.trim() || description.trim()) &&
      !createdProject &&
      !window.confirm('Discard this new project draft?')
    )
      return;
    createOpen = open;
    if (!open) {
      if (createdProject) void refresh();
      name = '';
      description = '';
      createError = '';
      createdProject = null;
    }
  }
  async function createProject(e: SubmitEvent) {
    e.preventDefault();
    if (busy || !name.trim()) return;
    busy = 'create';
    createError = '';
    try {
      // Retain the created ID if activation fails; retry activation rather than creating twice.
      if (!createdProject)
        createdProject = await post<Project>('/projects', { name: name.trim(), description: description.trim() });
      await post(`/projects/${createdProject.id}/activate`);
      const id = createdProject.id;
      createOpen = false;
      name = '';
      description = '';
      createdProject = null;
      window.dispatchEvent(new Event('videoflow:projects-changed'));
      await invalidateAll();
      await refresh();
      detailId = id;
    } catch (e) {
      createError = String(e);
    } finally {
      busy = '';
    }
  }
  const activate = (p: Project) =>
    run(`open-${p.id}`, async () => {
      await post(`/projects/${p.id}/activate`);
      await invalidateAll();
      await goto('/');
    });
  const activateForStyle = (p: Project) =>
    run(`open-${p.id}`, async () => {
      await post(`/projects/${p.id}/activate`);
      window.dispatchEvent(new Event('videoflow:projects-changed'));
      await invalidateAll();
      await refresh();
    });
  $effect(() => {
    const target = (page.state as { activateProject?: string }).activateProject;
    if (!loaded || !target || busy) return;
    replaceState(page.url, { ...page.state, activateProject: undefined });
    const p = projects.find(p => p.id === target);
    if (p) void activate(p);
    else toast.error('That project is no longer available.');
  });
  function exportProject(p: Project) {
    void run(`export-${p.id}`, async () => {
      const data = await get(`/projects/${p.id}/export`);
      const url = URL.createObjectURL(new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' }));
      const a = document.createElement('a');
      a.href = url;
      a.download = `${p.name.replace(/[^a-z0-9_-]/gi, '_') || 'project'}.json`;
      document.body.appendChild(a);
      a.click();
      a.remove();
      setTimeout(() => URL.revokeObjectURL(url), 1000);
    });
  }
  function importProject(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    void run('import', async () => {
      let data;
      try {
        data = JSON.parse(await file.text());
      } catch {
        throw Error('Choose a VideoFlow project JSON export. ZIP files are not supported.');
      }
      await post('/projects/import', data);
      await refresh();
      toast.success('Project imported');
    }).finally(() => (input.value = ''));
  }
  function startRename(p: Project) {
    renameTarget = p;
    renameValue = p.name;
  }
  function closeRename() {
    if (busy) return;
    if (renameTarget && renameValue !== renameTarget.name && !window.confirm('Discard the name change?')) return;
    renameTarget = null;
  }
  async function saveRename(e: SubmitEvent) {
    e.preventDefault();
    if (!renameValue.trim()) return;
    await run('rename', async () => {
      await patch(`/projects/${renameTarget!.id}`, { name: renameValue.trim() });
      renameTarget = null;
      await refresh();
    });
  }
</script>
<svelte:head><title>Projects · VideoFlow</title></svelte:head>
{#snippet actions(p: Project)}
 <DropdownMenu.Root><DropdownMenu.Trigger
   aria-label={`Actions for ${p.name}`}
   disabled={!!busy}
   class="inline-flex size-11 shrink-0 items-center justify-center rounded-md hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring"><Ellipsis
     class="size-4"/></DropdownMenu.Trigger><DropdownMenu.Content align="end"><DropdownMenu.Item
     onclick={()=>detailId=p.id}>Look &amp; details</DropdownMenu.Item><DropdownMenu.Item
     onclick={()=>activate(p)}>Open in Studio</DropdownMenu.Item><DropdownMenu.Item
     onclick={()=>startRename(p)}>Rename</DropdownMenu.Item><DropdownMenu.Item
     onclick={()=>exportProject(p)}>Export project</DropdownMenu.Item><DropdownMenu.Separator/><DropdownMenu.Item
   class="text-destructive"
   onclick={()=>deleteTarget=p}>Delete project</DropdownMenu.Item></DropdownMenu.Content></DropdownMenu.Root>
{/snippet}
<div class="mx-auto w-full max-w-[1600px] p-4 sm:p-8">
 <header class="mb-8 flex flex-wrap items-center justify-between gap-4"><div><h1
   class="text-2xl font-semibold tracking-tight">Projects</h1><p
   class="mt-1 text-sm text-muted-foreground">{loaded?`${projects.length} project${projects.length===1?'':'s'}`:'Loading projects…'}</p></div><div
   class="flex flex-wrap gap-2"><input
   bind:this={importInput}
   type="file"
   accept=".json,application/json"
   class="hidden"
   onchange={importProject}/><Button
   variant="outline"
   disabled={!!busy}
   onclick={()=>importInput?.click()}><Upload class="size-4"/>{busy==='import'?'Importing…':'Import'}</Button><Button
   disabled={!!busy}
   onclick={()=>changeCreateOpen(true)}><FolderPlus class="size-4"/>New project</Button></div></header>
 <div class="mb-6 flex flex-wrap items-center gap-3"><input
   type="search"
   aria-label="Search projects"
   placeholder="Search projects…"
   bind:value={search}
   class="min-h-11 min-w-0 flex-1 basis-60 rounded-md border border-input bg-background px-3 text-sm"/><select
   aria-label="Sort projects"
   bind:value={sort}
   class="min-h-11 rounded-md border border-input bg-background px-3 text-sm"><option
     value="updated">Recently updated</option><option value="created">Newest first</option><option value="name">Name A–Z</option></select><div
   role="group"
   aria-label="Project view"
   class="flex rounded-md border border-border p-1"><Button
   size="icon"
   variant={view==='folders'?'secondary':'ghost'}
   aria-label="Folder view"
   aria-pressed={view==='folders'}
   onclick={()=>setView('folders')}><LayoutGrid class="size-4"/></Button><Button
   size="icon"
   variant={view==='table'?'secondary':'ghost'}
   aria-label="Table view"
   aria-pressed={view==='table'}
   onclick={()=>setView('table')}><List class="size-4"/></Button></div></div>
 {#if error}<div
   role="alert"
   class="mb-4 rounded-md border border-destructive p-4 text-sm"><p>{error}</p><Button variant="outline"
     class="mt-2" onclick={refresh}>Try again</Button></div>{/if}
 {#if !loaded}<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">{#each [1,2,3] as n}<Skeleton class="h-48 rounded-lg"/>{/each}</div>
 {:else if !projects.length&&!error}<div class="py-20 text-center"><Folder
   class="mx-auto mb-4 size-10 text-muted-foreground"/><h2
   class="text-lg font-semibold">Create your first project</h2><Button class="mt-4" onclick={()=>changeCreateOpen(true)}>New project</Button></div>
 {:else if !filtered.length&&!error}<div role="status" class="py-16 text-center"><p>No matching projects</p><Button
   variant="outline"
   class="mt-3"
   onclick={()=>search=''}>Clear search</Button></div>
 {:else if view==='folders'}<section
   aria-label="Project folders"
   class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">{#each filtered as p (p.id)}<article
     class="group relative flex min-w-0 flex-col overflow-hidden rounded-xl border border-border bg-card transition hover:border-foreground/30 {p.is_active?'ring-1 ring-primary/50':''}"><div
     class="relative aspect-[16/9] bg-muted">{#if p.cover}<img
   src={mediaUrl(p.cover)??''}
   alt=""
   class="absolute inset-0 size-full object-cover"
   loading="lazy"/>{:else}<div
     class="flex size-full items-center justify-center text-muted-foreground"><Folder
     class="size-9"/></div>{/if}<div
     class="absolute right-2 top-2 z-10 rounded-md bg-background/85 shadow-sm">{@render actions(p)}</div>{#if p.is_active}<Badge
     class="absolute left-2 top-2">Open now</Badge>{/if}</div><div class="flex flex-1 flex-col p-4"><button
       type="button"
       onclick={()=>detailId=p.id}
       class="break-words text-left text-base font-semibold underline-offset-4 after:absolute after:inset-0 hover:underline focus-visible:outline-2 focus-visible:outline-ring">{p.name}</button>{#if p.description}<p
     class="mt-2 line-clamp-2 text-sm text-muted-foreground">{p.description}</p>{/if}<p
     class="mt-3 text-xs text-muted-foreground">{countsLine(p)}</p><div
     class="mt-auto flex items-center justify-between gap-2 pt-3"><p
     class="text-xs text-muted-foreground">Updated {fmtDate(p.updated_at||p.created_at)}</p>{#if p.is_active}<Button
   size="sm"
   variant="outline"
   class="relative z-10"
   href="/">Go to Studio</Button>{:else}<Button
   size="sm"
   variant="outline"
   class="relative z-10"
   disabled={!!busy}
   onclick={()=>activate(p)}>Open</Button>{/if}</div></div></article>{/each}</section>
 {:else}
 <!-- svelte-ignore a11y_no_noninteractive_tabindex (Keyboard focus enables horizontal table scrolling on small screens.) -->
 <div
   role="region"
   aria-label="Projects table"
   tabindex="0"
   class="relative max-w-full overflow-x-auto rounded-lg border border-border"><table
     class="w-full min-w-[640px] text-left text-sm"><caption
     class="sr-only">Projects with scene, character and asset counts</caption><thead
     class="border-b border-border text-xs text-muted-foreground"><tr><th scope="col"
     class="px-4 py-3 font-medium">Project</th><th scope="col" class="px-4 py-3 font-medium">Scenes</th><th
     scope="col" class="px-4 py-3 font-medium">Characters</th><th scope="col"
     class="px-4 py-3 font-medium">Assets</th><th scope="col" class="px-4 py-3 font-medium">Updated</th><th
     scope="col" class="px-3 py-3 font-medium"><span
     class="sr-only">Actions</span></th></tr></thead><tbody>{#each filtered as p (p.id)}<tr
     class="border-b border-border last:border-0 hover:bg-accent/30"><th scope="row"
     class="max-w-80 px-4 py-4 font-normal"><div class="flex items-center gap-2"><button
   type="button"
   onclick={()=>detailId=p.id}
   class="break-words text-left font-medium underline-offset-4 hover:underline">{p.name}</button>{#if p.is_active}<Badge
     variant="secondary">Active</Badge>{/if}</div>{#if p.description}<p
     class="mt-1 line-clamp-1 text-xs text-muted-foreground">{p.description}</p>{/if}</th><td
     class="px-4 py-4 tabular-nums">{p.counts?.scenes??'—'}</td><td
     class="px-4 py-4 tabular-nums">{p.counts?.characters??'—'}</td><td
     class="px-4 py-4 tabular-nums">{p.counts?.assets??'—'}</td><td
     class="whitespace-nowrap px-4 py-4 text-muted-foreground">{fmtDate(p.updated_at||p.created_at)}</td><td
     class="px-3 py-2">{@render actions(p)}</td></tr>{/each}</tbody></table></div>{/if}
 <details class="mt-8 text-xs text-muted-foreground"><summary
   class="min-h-11 cursor-pointer py-3">About project files</summary><p
   class="max-w-prose">JSON includes story data and media references. Media files, render history, voices and AI sessions are not included. Keep your data volume for a full backup.</p></details>
</div>
<Dialog.Root
  open={createOpen}
  onOpenChange={changeCreateOpen}><Dialog.Content
  class="sm:max-w-lg"
  showCloseButton={!busy}
  onEscapeKeydown={e=>{e.preventDefault();changeCreateOpen(false)}}
  onInteractOutside={e=>{e.preventDefault();changeCreateOpen(false)}}><Dialog.Header><Dialog.Title>New project</Dialog.Title><Dialog.Description>Create a project, then choose its visual style. This switches your active workspace.</Dialog.Description></Dialog.Header><form
    onsubmit={createProject} class="space-y-4"><fieldset
  disabled={!!busy||!!createdProject}
  class="space-y-4"><label class="grid gap-2 text-sm">Project name<Input
  bind:value={name}
  required
  maxlength={200}
  placeholder="My new video"/></label><label class="grid gap-2 text-sm">Description (optional)<Input
    bind:value={description} maxlength={2000}/></label></fieldset>{#if createError}<p role="alert"
    class="text-sm text-destructive">{createdProject?'Project created. Opening it failed; retry to open the same project. ':''}{createError}</p>{/if}<Dialog.Footer><Button
      type="button"
      variant="outline"
      disabled={!!busy}
      onclick={()=>changeCreateOpen(false)}>Cancel</Button><Button type="submit"
    disabled={!!busy||!name.trim()}>{busy==='create'?'Opening…':createdProject?'Retry opening project':'Create project'}</Button></Dialog.Footer></form></Dialog.Content></Dialog.Root>
<Dialog.Root
  open={!!renameTarget}
  onOpenChange={open=>{if(!open)closeRename()}}><Dialog.Content
  showCloseButton={!busy}
  onEscapeKeydown={e=>{e.preventDefault();closeRename()}}
  onInteractOutside={e=>{e.preventDefault();closeRename()}}><Dialog.Header><Dialog.Title>Rename project</Dialog.Title><Dialog.Description>Update the name shown in your project collection.</Dialog.Description></Dialog.Header><form
    onsubmit={saveRename} class="space-y-4"><label class="grid gap-2 text-sm">Project name<Input
  bind:value={renameValue}
  required
  maxlength={200}
  disabled={!!busy}/></label><Dialog.Footer><Button
  type="button"
  variant="outline"
  disabled={!!busy}
  onclick={closeRename}>Cancel</Button><Button
  type="submit"
  disabled={!!busy||!renameValue.trim()}>Save name</Button></Dialog.Footer></form></Dialog.Content></Dialog.Root>
<Dialog.Root
  open={!!detail}
  onOpenChange={open=>{if(!open&&!styleSaving)detailId=''}}><Dialog.Content
  class="max-h-[90dvh] overflow-y-auto sm:max-w-3xl"
  showCloseButton={!styleSaving}>{#if detail}<Dialog.Header><Dialog.Title
    class="flex flex-wrap items-center gap-2 break-words">{detail.name}{#if detail.is_active}<Badge
    variant="secondary">Active</Badge>{/if}</Dialog.Title><Dialog.Description>{detail.description||'Counts and the look used for new images and videos.'}</Dialog.Description></Dialog.Header>
 <dl
   class="grid grid-cols-3 gap-4 border-y border-border py-4">{#each [
       ['Scenes', detail.counts?.scenes ?? '—'],
       ['Characters', detail.counts?.characters ?? '—'],
       ['Assets', detail.counts?.assets ?? '—']
     ] as [label,count]}<div><dt
   class="text-xs text-muted-foreground">{label}</dt><dd class="mt-1 text-2xl font-semibold tabular-nums">{count}</dd></div>{/each}</dl>
 {#if detail.is_active}{#key detail.id}<ProjectStyle
   projectId={detail.id}
   projectName={detail.name}
   disabled={!!busy}
   onSavingChange={saving=>styleSaving=saving}/>{/key}{:else}<section
     class="rounded-lg border border-border p-5"><h2 class="text-base font-semibold">Project look</h2><p
     class="mt-2 text-sm text-muted-foreground">Make this your active project to edit its look.</p><Button
   variant="outline"
   class="mt-4"
   disabled={!!busy}
   onclick={()=>activateForStyle(detail)}>Activate to edit look</Button></section>{/if}
 <Dialog.Footer><Button
   variant="outline"
   disabled={!!busy||styleSaving}
   onclick={()=>detailId=''}>Close</Button><Button
   disabled={!!busy||styleSaving}
   onclick={()=>activate(detail)}>Open in Studio</Button></Dialog.Footer>{/if}</Dialog.Content></Dialog.Root>
{#if deleteTarget}<DeleteProject project={deleteTarget} last={projects.length===1} onclose={()=>deleteTarget=null} ondeleted={refresh}/>{/if}
