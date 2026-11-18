<script lang="ts">
 import StyleReference from '$lib/brief/StyleReference.svelte';
  import { onMount } from 'svelte';
 import StyleCards from '$lib/brief/StyleCards.svelte';
  import { get, patch } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import { Input } from '$lib/components/ui/input';

  let { projectId, projectName, disabled = false, onSavingChange }: { projectId: string; projectName: string; disabled?: boolean; onSavingChange?: (saving: boolean) => void } = $props();
  type Style = { name: string; style_prompt: string; palette: string; lighting: string; audience: string; tone: string };
  type Preset = { id: string; name: string; description: string; style_prompt: string; guidance: Record<string, string> };
  const empty: Style = { name: '', style_prompt: '', palette: '', lighting: '', audience: '', tone: '' };
  let form: Style = $state({ ...empty });
  let presets: Preset[] = $state([]);
  let baseline = $state('');
  let selected = $state('');
  let loaded = $state(false);
  let saving = $state(false);
  let error = $state('');
  let message = $state('');
  let hasSavedStyle = $state(false);
  const key = $derived(`VideoFlow.projectStyleDraft.${projectId}`);
  const clean = (value: any): Style => Object.fromEntries(Object.keys(empty).map(k => [k, typeof value?.[k] === 'string' ? value[k] : ''])) as Style;
  let dirty = $derived(loaded && JSON.stringify(form) !== baseline);
  let preview = $derived(presets.find(p => p.id === selected));

  async function load() {
    loaded = false;
    error = '';
    try {
      const [style, catalog] = await Promise.all([get('/style'), get('/style-presets')]);
      if (style.project_id !== projectId) throw new Error('The active project changed. Reload Projects to edit its style.');
      presets = catalog.presets ?? [];
      hasSavedStyle = !!catalog.has_project_style;
      form = clean(style);
      baseline = JSON.stringify(form);
      selected=presets.find(p=>p.style_prompt===form.style_prompt)?.id??'';
      try {
        const draft = JSON.parse(sessionStorage.getItem(key) || 'null');
        if (draft?.form && typeof draft.baseline === 'string') {
          form = clean(draft.form);
          selected = draft.selected ?? '';
          if (draft.baseline !== baseline) message = 'Your draft was restored. The saved style changed since you started; review it before saving.';
          else message = 'Your unsaved draft was restored.';
          // Retain the original baseline so Save cannot silently overwrite another edit.
          baseline = draft.baseline;
        }
      } catch { /* Draft storage may be unavailable. Editing still works. */ }
      loaded = true;
    } catch (e) { error = e instanceof Error ? e.message : 'Could not load project style.'; }
  }
  onMount(() => { void load(); });
  $effect(() => {
    if (!loaded) return;
    try {
      if (dirty) sessionStorage.setItem(key, JSON.stringify({ form, baseline, selected }));
      else sessionStorage.removeItem(key);
    } catch { /* Storage is optional. */ }
  });
  async function save() {
    saving = true;
    onSavingChange?.(true);
    error = '';
    message = '';
    try {
      const projects = await get('/projects');
      if (!projects.some((p: any) => p.id === projectId && p.is_active)) throw new Error('The active project changed. Reload Projects before saving. Your draft is kept.');
      const current = await get('/style');
      if (current.project_id !== projectId || JSON.stringify(clean(current)) !== baseline) throw new Error('The saved style changed. Your draft is kept. Use “Reload saved style” to review the current version before editing again.');
      const result = await patch('/style', {...form,project_id:projectId});
      form = clean(result);
      baseline = JSON.stringify(form);
      hasSavedStyle = true;
      message = 'Project style saved. New stories can use this project style.';
    } catch (e) { error = e instanceof Error ? e.message : 'Could not save project style. Try again.'; }
    finally { saving = false; onSavingChange?.(false); }
  }
  async function reset() {
    try { sessionStorage.removeItem(key); } catch { /* Storage is optional. */ }
    message = '';
    await load();
  }
</script>

<section class="mb-6 rounded-lg border border-border bg-card p-4 sm:p-5" aria-labelledby="project-style-title">
  <div class="mb-4 space-y-1">
    <h2 id="project-style-title" class="text-base font-semibold">Visual style · {projectName}</h2>
    <p class="max-w-prose text-sm text-muted-foreground">For the active project only. Save its look here, then choose Project style when creating a story. Existing images and stories stay as they are.</p>
  </div>
  {#if error}<p role="alert" class="mb-3 text-sm text-destructive">{error}</p>{/if}
  {#if !loaded}
    {#if error}<Button variant="outline" onclick={load}>Retry loading style</Button>
    {:else}<p role="status" class="text-sm text-muted-foreground">Loading project style…</p>{/if}
  {:else}
    <details class="mb-4 rounded-md border border-border p-3"><summary class="min-h-11 cursor-pointer py-3 text-sm font-medium">Create a look from an image, video or link</summary>{#if dirty}<p class="text-sm text-muted-foreground">Save or discard your current style edits before replacing the project style.</p>{/if}<StyleReference disabled={disabled||saving||dirty} onsaved={async()=>{try{sessionStorage.removeItem(key)}catch{}await load()}}/></details>
    <form onsubmit={(event) => { event.preventDefault(); void save(); }} class="space-y-4">
      <fieldset disabled={disabled || saving} class="min-w-0 space-y-4">
        <p class="text-sm"><span class="text-muted-foreground">Current look:</span> {form.name||'Custom project style'}{dirty?' · Unsaved changes':''}</p>
        <StyleCards {presets} {selected} disabled={disabled||saving} onselect={preset=>{selected=preset.id;form={...form,name:preset.name,style_prompt:preset.style_prompt??'',palette:'',lighting:''};message=''}}/>
        {#if preview}
          <div class="space-y-2 text-sm">
            <p class="max-w-prose text-muted-foreground">{preview.description}</p>
            <p class="text-xs text-muted-foreground">Applying a preset starts a fresh palette and lighting. Add your own under Customize this look.</p>
            <details><summary class="cursor-pointer py-1 font-medium">Preview preset guidance</summary>
              <dl class="mt-2 grid gap-3 sm:grid-cols-2">{#each Object.entries(preview.guidance ?? {}) as [stage, guidance]}<div><dt class="font-medium capitalize">{stage}</dt><dd class="max-w-prose whitespace-pre-wrap break-words text-muted-foreground">{guidance}</dd></div>{/each}</dl>
              <p class="mt-3 text-xs text-muted-foreground">Stage guidance applies when choosing this preset in Create story. Project style saves the visual prompt and fields below.</p>
            </details>
          </div>
        {/if}
        <details><summary class="cursor-pointer py-2 text-sm font-medium">Customize this look</summary><div class="mt-3 space-y-4"><label class="grid gap-1.5 text-sm font-medium">Style name<Input bind:value={form.name} required maxlength={120} /></label>
        <label class="grid gap-1.5 text-sm font-medium">Visual prompt
          <textarea bind:value={form.style_prompt} required rows="3" class="w-full resize-y rounded-md border border-input bg-background px-3 py-2 text-sm font-normal focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"></textarea>
        </label>
        <div class="grid gap-4 sm:grid-cols-2">
          <label class="grid gap-1.5 text-sm font-medium">Palette<Input bind:value={form.palette} /></label>
          <label class="grid gap-1.5 text-sm font-medium">Lighting<Input bind:value={form.lighting} /></label>
          <label class="grid gap-1.5 text-sm font-medium">Audience<Input bind:value={form.audience} /></label>
          <label class="grid gap-1.5 text-sm font-medium">Tone<Input bind:value={form.tone} /></label>
        </div>
        </div></details>
        <div class="flex flex-wrap items-center gap-3">
          <Button type="submit" disabled={saving || (!dirty && hasSavedStyle) || !form.name.trim() || !form.style_prompt.trim()}>{saving ? 'Saving…' : 'Save project style'}</Button>
          <Button type="button" variant="outline" onclick={reset}>Reload saved style</Button>
          <span class="text-xs text-muted-foreground">{dirty ? 'Unsaved draft · kept in this tab' : hasSavedStyle ? 'Saved project style' : 'Default look · save to use as project style'}</span>
        </div>
      </fieldset>
      {#if message}<p role="status" class="text-sm">{message}</p>{/if}
    </form>
  {/if}
</section>
