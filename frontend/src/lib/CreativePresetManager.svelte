<script lang="ts">
  import { onMount } from 'svelte';
  import { API_BASE, get } from '$lib/api';
  type Preset = { id: string; name: string; description: string; style_prompt: string; guidance: Record<string, string> };
  type Pack = { id: string; name: string; version: string; author?: string; license?: string; presets: Preset[] };
  let catalog: { presets: Preset[]; packs: { pack: Pack; hash: string }[]; built_in_count: number } | null = $state(null);
  let preview: { pack: Pack; existing_hash: string; unchanged: boolean } | null = $state(null);
  let error = $state(''),
    message = $state(''),
    busy = $state(false),
    fileName = $state('');
  async function load() {
    try {
      catalog = await get('/style-presets');
    } catch (e) {
      error = String(e);
    }
  }
  onMount(() => {
    void load();
  });
  async function request(path: string, method: string, body?: unknown, hash = '') {
    const r = await fetch(API_BASE + path, {
      method,
      headers: { 'Content-Type': 'application/json', ...(hash ? { 'If-Match': hash } : {}) },
      ...(body ? { body: JSON.stringify(body) } : {})
    });
    if (!r.ok) throw new Error(await r.text());
    return r.json();
  }
  async function inspect(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    preview = null;
    error = '';
    message = '';
    fileName = file?.name ?? '';
    if (!file) return;
    if (file.size > 524288) {
      error = 'Choose a JSON pack smaller than 512 KiB.';
      input.value = '';
      return;
    }
    busy = true;
    try {
      const raw = JSON.parse(await file.text());
      preview = await request('/preset-packs/preview', 'POST', raw);
    } catch (e) {
      error = String(e);
    } finally {
      busy = false;
      input.value = '';
    }
  }
  async function install() {
    if (!preview) return;
    busy = true;
    error = '';
    try {
      await request('/preset-packs/import', 'POST', preview.pack, preview.existing_hash);
      message = `Installed ${preview.pack.name}.`;
      preview = null;
      await load();
    } catch (e) {
      error = String(e);
    } finally {
      busy = false;
    }
  }
  async function remove(pack: Pack, hash: string) {
    if (!confirm(`Remove ${pack.name}? Existing stories keep their saved preset.`)) return;
    busy = true;
    error = '';
    try {
      await request('/preset-packs/' + pack.id, 'DELETE', undefined, hash);
      message = `Removed ${pack.name}.`;
      await load();
    } catch (e) {
      error = String(e);
    } finally {
      busy = false;
    }
  }
</script>
<section class="space-y-6" aria-labelledby="preset-title">
 <div class="space-y-2"><h2 id="preset-title" class="text-lg font-semibold">Creative presets</h2><p
   class="max-w-prose text-sm text-muted-foreground">Choose a look when creating a story. Each preset guides the story, shots, images and motion. Existing stories keep a saved copy.</p></div>
 {#if error}<p role="alert" class="text-sm text-destructive">{error}</p><button
   class="underline"
   onclick={()=>{error='';void load()}}>Reload presets</button>{/if}
 <p role="status" class="text-sm">{busy?'Working…':message}</p>
 {#if catalog}
 <div class="divide-y rounded-xl border">
 {#each catalog.presets.slice(0,catalog.built_in_count) as preset(preset.id)}
 <details class="p-4"><summary class="cursor-pointer font-medium">{preset.name}</summary><p
   class="mt-2 text-sm text-muted-foreground">{preset.description}</p><p
   class="mt-3 text-sm">{preset.style_prompt}</p><dl
   class="mt-3 space-y-3">{#each Object.entries(preset.guidance) as [stage,text]}<div><dt
   class="text-sm font-medium capitalize">{stage}</dt><dd
   class="max-w-prose whitespace-pre-wrap text-sm text-muted-foreground">{text}</dd></div>{/each}</dl></details>
 {/each}</div>
 <section class="space-y-3" aria-labelledby="community-title"><h3 id="community-title" class="font-semibold">Community packs</h3>
 <p class="max-w-prose text-sm text-muted-foreground">Share data-only .videoflow-presets.json files. Packs contain creative guidance, never executable plugins. Author and license details are supplied by the pack’s creator.</p>
 <div class="flex flex-wrap items-center gap-3 text-sm"><a
   href={API_BASE+'/preset-packs/starter/export'}
   class="underline underline-offset-4"
   download>Download starter pack</a><a
   href="/presets/README.md"
   class="underline underline-offset-4">Format guide</a><a
   href="/presets/preset-pack.schema.json"
   class="underline underline-offset-4"
   download>JSON schema</a></div>
 <label class="grid max-w-lg gap-2 text-sm font-medium">Preview a preset pack<input
   disabled={busy}
   type="file"
   accept=".json,application/json"
   onchange={inspect}
   class="rounded-lg border p-3 font-normal file:mr-3" /></label>
 {#if preview}<div class="space-y-3 rounded-xl border p-4"><h4
   class="font-semibold">{preview.pack.name} · {preview.pack.version}</h4><p
   class="break-words text-sm text-muted-foreground">{fileName} · {preview.pack.author||'Author unspecified'} · {preview.pack.license||'License unspecified'}</p>
 {#each preview.pack.presets as p(p.id)}<details><summary
   class="cursor-pointer text-sm font-medium">{p.name}</summary><p
   class="mt-2 whitespace-pre-wrap break-words text-sm">{p.style_prompt}</p>{#each Object.entries(p.guidance) as [stage,text]}<p
   class="mt-2 whitespace-pre-wrap break-words text-sm"><strong class="capitalize">{stage}:</strong> {text}</p>{/each}</details>{/each}
 {#if preview.existing_hash&&!preview.unchanged}<p class="text-sm">This replaces the installed pack. Saved stories keep their previous guidance.</p>{/if}
 <div class="flex gap-2"><button
   disabled={busy||preview.unchanged}
   onclick={install}
   class="min-h-11 rounded-lg bg-primary px-4 text-sm font-medium text-primary-foreground disabled:opacity-50">{preview.unchanged?'Already installed':preview.existing_hash?'Replace pack':'Install pack'}</button><button
     disabled={busy}
     onclick={()=>preview=null}
     class="min-h-11 rounded-lg border px-4 text-sm">Cancel</button></div></div>{/if}
 {#if !catalog.packs.length}<p class="text-sm text-muted-foreground">No community packs installed.</p>{/if}
 <ul class="divide-y">{#each catalog.packs as {pack,hash}(pack.id)}<li
   class="flex flex-wrap items-center justify-between gap-3 py-4"><div><p
   class="font-medium">{pack.name} · {pack.version}</p><p
   class="text-sm text-muted-foreground">{pack.presets.length} presets · {pack.author||'Author unspecified'}</p></div><div
   class="flex gap-4 text-sm"><a
   href={API_BASE+'/preset-packs/'+pack.id+'/export'}
   class="inline-flex min-h-11 items-center underline"
   download>Export</a><button
   disabled={busy}
   onclick={()=>remove(pack,hash)}
   class="min-h-11 underline">Remove</button></div></li>{/each}</ul>
 </section>
 {:else if !error}<p role="status">Loading presets…</p>{/if}
</section>
