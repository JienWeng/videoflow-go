<script lang="ts">
  import { page } from '$app/state';
  import { onMount } from 'svelte';
  import { goto, invalidateAll, replaceState } from '$app/navigation';
  import { get, post, patch, del } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import { Badge } from '$lib/components/ui/badge';
  import { Card, CardContent } from '$lib/components/ui/card';
  import { Input } from '$lib/components/ui/input';
  import { Skeleton } from '$lib/components/ui/skeleton';
  import * as Dialog from '$lib/components/ui/dialog';
  import { toast } from 'svelte-sonner';
  import { FolderPlus, Pencil, Trash2, Check, X, Download, Upload } from '@lucide/svelte';

  let projects: any[] = $state([]);
  let loaded = $state(false);
  let busy = $state('');

  let name = $state('');
  let description = $state('');

  let renamingId = $state('');
  let renameValue = $state('');

  let deleteTarget: any = $state(null);
  let importInput: HTMLInputElement | null = $state(null);

  async function refresh() {
    projects = await get('/projects');
  }
  onMount(() =>
    refresh()
      .catch((e) => toast.error(e.message))
      .finally(() => (loaded = true))
  );

  async function run(key: string, fn: () => Promise<unknown>) {
    busy = key;
    try {
      await fn();
    } catch (e: any) {
      toast.error(e.message);
    } finally {
      busy = '';
    }
  }

  // The active project transparently scopes every backend endpoint. After
  // creating or switching we soft-navigate to the Studio with invalidateAll so
  // every page refetches its data under the new project scope (no full reload).
  const createProject = (e: Event) => {
    e.preventDefault();
    run('create', async () => {
      const project = await post('/projects', { name: name.trim(), description });
      await post(`/projects/${project.id}/activate`);
      name = '';
      description = '';
      await invalidateAll();
      await goto('/');
    });
  };

  const activate = (p: any) =>
    run(`open-${p.id}`, async () => {
      await post(`/projects/${p.id}/activate`);
      await invalidateAll();
      await goto('/');
    });

  $effect(() => {
    const target = (page.state as {activateProject?: string}).activateProject;
    if (!loaded || !target || busy) return;
    // Consume before submitting: browser back or a failed request must not replay it.
    replaceState(page.url, {...page.state, activateProject: undefined});
    const project = projects.find(p => p.id === target);
    if (project) void activate(project);
    else toast.error('That project is no longer available. Choose a project to continue.');
  });

  function exportProject(p: any) {
    run(`export-${p.id}`, async () => {
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

  const importProject = (e: Event) => {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    run('import', async () => {
      let data: unknown;
      try { data = JSON.parse(await file.text()); }
      catch { throw new Error('Choose a VideoFlow project JSON export. ZIP files are not supported.'); }
      await post('/projects/import', data);
      input.value = '';
      await refresh();
      toast.success('Project imported');
    }).finally(() => {
      if (input) input.value = '';
    });
  };

  function startRename(p: any) {
    renamingId = p.id;
    renameValue = p.name;
  }

  const saveRename = (p: any) =>
    run(`rename-${p.id}`, async () => {
      await patch(`/projects/${p.id}`, { name: renameValue });
      renamingId = '';
      await refresh();
    });

  const deleteProject = (p: any) =>
    run(`delete-${p.id}`, async () => {
      await del(`/projects/${p.id}`);
      deleteTarget = null;
      await refresh();
    });

  function countsLine(c: any): string {
    const parts = [
      `${c?.scenes ?? 0} scenes`,
      `${c?.characters ?? 0} characters`,
      `${c?.assets ?? 0} assets`
    ];
    return parts.join(' · ');
  }

  const fmtDate = (s: string) =>
    new Date(s).toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' });
</script>

<svelte:head><title>Projects · VideoFlow</title></svelte:head>

<div class="p-6">
  <div class="mb-4 flex items-start justify-between gap-3">
    <div>
      <h1 class="text-lg font-semibold">Projects</h1>
    </div>
    <div>
      <input
        bind:this={importInput}
        type="file"
        accept=".json,application/json"
        class="hidden"
        onchange={importProject}
      />
      <Button variant="outline" size="sm" disabled={busy === 'import'} onclick={() => importInput?.click()}>
        <Upload class="size-4 mr-1" />
        {busy === 'import' ? 'Importing…' : 'Import'}
      </Button>
    </div>
  </div>

  <form class="mb-6 rounded-lg border border-border bg-card p-4" onsubmit={createProject}>
    <div class="flex flex-wrap gap-4 items-end">
      <div class="flex-1 min-w-[160px]">
        <label class="block text-xs text-muted-foreground mb-1" for="proj-name">Name</label>
        <Input id="proj-name" bind:value={name} required placeholder="My new video" />
      </div>
      <div class="flex-1 min-w-[160px]">
        <label class="block text-xs text-muted-foreground mb-1" for="proj-desc">Description (optional)</label>
        <Input id="proj-desc" bind:value={description} />
      </div>
      <div>
        <Button type="submit" size="sm" disabled={!!busy || !name.trim()}>
          <FolderPlus class="size-4 mr-1" />Create project
        </Button>
      </div>
    </div>
    <p class="mt-2 text-xs text-muted-foreground">Creating switches you to the new project.</p>
  </form>
  <details class="mb-4 text-xs text-muted-foreground"><summary class="cursor-pointer py-2">About project files</summary><p class="mt-1 max-w-prose">JSON includes story data and media references. Media files, render history, voices and AI sessions are not included. Keep your data volume for a full backup.</p></details>

  {#if !loaded}
    <div class="grid grid-cols-[repeat(auto-fill,minmax(280px,1fr))] gap-4">
      {#each Array(2) as _, i (i)}
        <div class="rounded-lg border border-border p-4 space-y-2">
          <Skeleton class="h-4 w-32" />
          <Skeleton class="h-3 w-48" />
          <Skeleton class="h-3 w-40" />
        </div>
      {/each}
    </div>
  {:else}
    <div class="grid grid-cols-[repeat(auto-fill,minmax(280px,1fr))] gap-4">
      {#each projects as p (p.id)}
        <Card>
          <CardContent class="p-4">
            <div class="flex items-start justify-between gap-2 mb-1">
              {#if renamingId === p.id}
                <form
                  class="flex items-center gap-1 flex-1"
                  onsubmit={(e) => {
                    e.preventDefault();
                    saveRename(p);
                  }}
                >
                  <Input class="h-7 text-sm" bind:value={renameValue} required />
                  <Button type="submit" size="icon" variant="ghost" class="size-7 shrink-0"
                    disabled={!!busy || !renameValue} title="Save name">
                    <Check class="size-3.5" />
                  </Button>
                  <Button type="button" size="icon" variant="ghost" class="size-7 shrink-0"
                    onclick={() => (renamingId = '')} title="Cancel">
                    <X class="size-3.5" />
                  </Button>
                </form>
              {:else}
                <div class="flex items-center gap-1.5 min-w-0">
                  <span class="font-semibold text-sm truncate">{p.name}</span>
                  <button
                    class="text-muted-foreground hover:text-foreground shrink-0"
                    title="Rename project"
                    onclick={() => startRename(p)}
                  >
                    <Pencil class="size-3" />
                  </button>
                </div>
                {#if p.is_active}
                  <Badge>Active</Badge>
                {/if}
              {/if}
            </div>
            {#if p.description}
              <p class="text-sm text-muted-foreground mb-1">{p.description}</p>
            {/if}
            {#if p.counts}<p class="text-xs text-muted-foreground mb-0.5">{countsLine(p.counts)}</p>{/if}
            <p class="text-xs text-muted-foreground mb-3">Created {fmtDate(p.created_at)}</p>
            <div class="flex flex-wrap gap-1.5">
              {#if !p.is_active}
                <Button variant="secondary" size="sm" disabled={!!busy} onclick={() => activate(p)}>
                  {busy === `open-${p.id}` ? 'Opening…' : 'Open'}
                </Button>
              {/if}
              <Button variant="ghost" size="sm" disabled={!!busy} onclick={() => exportProject(p)}>
                <Download class="size-3.5 mr-1" />Export
              </Button>
              {#if !p.is_active}
                <Button
                  variant="ghost"
                  size="sm"
                  class="text-destructive hover:text-destructive"
                  disabled={!!busy}
                  onclick={() => (deleteTarget = p)}
                >
                  <Trash2 class="size-3.5 mr-1" />Delete
                </Button>
              {/if}
            </div>
          </CardContent>
        </Card>
      {/each}
    </div>
  {/if}

  <Dialog.Root open={deleteTarget !== null} onOpenChange={(open) => !open && (deleteTarget = null)}>
    <Dialog.Content>
      <Dialog.Header>
        <Dialog.Title>Delete "{deleteTarget?.name}"?</Dialog.Title>
        <Dialog.Description>
          Deletes ALL its scenes, characters, assets and renders. Files on disk are kept.
        </Dialog.Description>
      </Dialog.Header>
      <Dialog.Footer>
        <Button variant="outline" onclick={() => (deleteTarget = null)}>Cancel</Button>
        <Button variant="destructive" disabled={!!busy} onclick={() => deleteProject(deleteTarget)}>
          <Trash2 class="size-4 mr-1" />Delete project
        </Button>
      </Dialog.Footer>
    </Dialog.Content>
  </Dialog.Root>
</div>
