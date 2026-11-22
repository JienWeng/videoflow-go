<script lang="ts">
  import { errorMessage } from '$lib/errors';
  import StyleReference from '$lib/brief/StyleReference.svelte';
  import { onMount } from 'svelte';
  import StyleCards from '$lib/brief/StyleCards.svelte';
  import { get, patch } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import { Input } from '$lib/components/ui/input';
  import type { Project, StyleCatalog, StyleFields, StyleGuide, StylePreset } from '$lib/api-types';

  let {
    projectId,
    projectName,
    disabled = false,
    onSavingChange
  }: {
    projectId: string;
    projectName: string;
    disabled?: boolean;
    onSavingChange?: (saving: boolean) => void;
  } = $props();
  type Style = StyleFields;
  const empty: Style = { name: '', style_prompt: '', palette: '', lighting: '', audience: '', tone: '' };
  let form: Style = $state({ ...empty });
  let presets: StylePreset[] = $state([]);
  let baseline = $state('');
  let selected = $state('');
  let loaded = $state(false);
  let saving = $state(false);
  let error = $state('');
  let message = $state('');
  let hasSavedStyle = $state(false);
  const key = $derived(`VideoFlow.projectStyleDraft.${projectId}`);
  const clean = (value: unknown): Style => {
    const source = (value && typeof value === 'object' ? value : {}) as Record<string, unknown>;
    return Object.fromEntries(Object.keys(empty).map(k => [k, typeof source[k] === 'string' ? source[k] : ''])) as Style;
  };
  let dirty = $derived(loaded && JSON.stringify(form) !== baseline);
  let preview = $derived(presets.find(p => p.id === selected));

  async function load() {
    loaded = false;
    error = '';
    try {
      const [style, catalog] = await Promise.all([get<StyleGuide>('/style'), get<StyleCatalog>('/style-presets')]);
      if (style.project_id !== projectId)
        throw new Error('The active project changed. Reload Projects to edit its style.');
      presets = catalog.presets ?? [];
      hasSavedStyle = !!catalog.has_project_style;
      form = clean(style);
      baseline = JSON.stringify(form);
      selected = presets.find(p => p.style_prompt === form.style_prompt)?.id ?? '';
      try {
        const draft = JSON.parse(sessionStorage.getItem(key) || 'null');
        if (draft?.form && typeof draft.baseline === 'string') {
          form = clean(draft.form);
          selected = draft.selected ?? '';
          if (draft.baseline !== baseline)
            message = 'Your draft was restored. The saved style changed since you started; review it before saving.';
          else message = 'Your unsaved draft was restored.';
          // Retain the original baseline so Save cannot silently overwrite another edit.
          baseline = draft.baseline;
        }
      } catch {
        /* Draft storage may be unavailable. Editing still works. */
      }
      loaded = true;
    } catch (e) {
      error = errorMessage(e, 'Could not load project style.');
    }
  }
  onMount(() => {
    void load();
  });
  $effect(() => {
    if (!loaded) return;
    try {
      if (dirty) sessionStorage.setItem(key, JSON.stringify({ form, baseline, selected }));
      else sessionStorage.removeItem(key);
    } catch {
      /* Storage is optional. */
    }
  });
  async function save() {
    saving = true;
    onSavingChange?.(true);
    error = '';
    message = '';
    try {
      const projects = await get<Project[]>('/projects');
      if (!projects.some(p => p.id === projectId && p.is_active))
        throw new Error('The active project changed. Reload Projects before saving. Your draft is kept.');
      const current = await get<StyleGuide>('/style');
      if (current.project_id !== projectId || JSON.stringify(clean(current)) !== baseline)
        throw new Error(
          'The saved style changed. Your draft is kept. Choose “Undo changes” to load the latest saved look.'
        );
      const result = await patch<StyleGuide>('/style', { ...form, project_id: projectId });
      form = clean(result);
      baseline = JSON.stringify(form);
      hasSavedStyle = true;
      message = 'Look saved. New images and stories in this project will use it.';
    } catch (e) {
      error = errorMessage(e, 'Could not save project style. Try again.');
    } finally {
      saving = false;
      onSavingChange?.(false);
    }
  }
  async function reset() {
    try {
      sessionStorage.removeItem(key);
    } catch {
      /* Storage is optional. */
    }
    message = '';
    await load();
  }
</script>

<section class="mb-6 rounded-lg border border-border bg-card p-4 sm:p-5" aria-labelledby="project-style-title">
  <div class="mb-4 flex flex-wrap items-start justify-between gap-3">
    <div class="min-w-0 space-y-1">
      <h2 id="project-style-title" class="text-base font-semibold">Project look</h2>
      <p class="max-w-prose text-sm text-muted-foreground">How new images and videos in <span
        class="font-medium text-foreground">{projectName}</span> should look. Pick a style, then save. Images you already made don't change.</p>
    </div>
    {#if loaded}<span
      class="shrink-0 rounded-full px-2.5 py-1 text-xs font-medium {dirty?'bg-amber-500/15 text-amber-800 dark:text-amber-300':'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300'}"
      role="status">{dirty?'Not saved yet':hasSavedStyle?`Using: ${form.name||'Custom look'}`:'Default look'}</span>{/if}
  </div>
  {#if error}<p role="alert" class="mb-3 text-sm text-destructive">{error}</p>{/if}
  {#if !loaded}
    {#if error}<Button variant="outline" onclick={load}>Try again</Button>
    {:else}<p role="status" class="text-sm text-muted-foreground">Loading project look…</p>{/if}
  {:else}
    <form onsubmit={(event) => { event.preventDefault(); void save(); }} class="space-y-5">
      <fieldset disabled={disabled || saving} class="min-w-0 space-y-5">
        <StyleCards
          {presets}
          {selected}
          disabled={disabled||saving}
          onselect={preset => {
            selected = preset.id;
            form = { ...form, name: preset.name, style_prompt: preset.style_prompt ?? '', palette: '', lighting: '' };
            message = '';
          }}/>
        {#if !selected && form.style_prompt}<p
          class="text-sm text-muted-foreground">Your current look is custom: <span
          class="font-medium text-foreground">{form.name||'Custom look'}</span>. Pick a style above to replace it, or fine-tune it below.</p>{/if}
        {#if preview}
          <details class="rounded-md bg-muted/30 px-3 text-sm">
            <summary class="min-h-10 cursor-pointer py-2.5 font-medium">What “{preview.name}” tells the AI</summary>
            <dl
              class="grid gap-3 pb-3 sm:grid-cols-2">{#each Object.entries(preview.guidance ?? {}) as [stage, guidance]}<div><dt
              class="font-medium capitalize">{stage}</dt><dd
              class="max-w-prose whitespace-pre-wrap break-words text-muted-foreground">{guidance}</dd></div>{/each}</dl>
          </details>
        {/if}
        <details class="rounded-md border border-border px-3">
          <summary class="min-h-11 cursor-pointer py-3 text-sm font-medium">Fine-tune the look <span
            class="font-normal text-muted-foreground">(optional)</span></summary>
          <div class="space-y-4 pb-4">
            <p class="text-xs text-muted-foreground">Describe it in your own words. Picking a style above fills these in for you; colors and lighting reset to that style's defaults.</p>
            <label class="grid gap-1.5 text-sm font-medium">Name<Input
              bind:value={form.name}
              required
              maxlength={120}
              placeholder="e.g. Warm storybook" /></label>
            <label class="grid gap-1.5 text-sm font-medium">Describe the look
              <textarea
                bind:value={form.style_prompt}
                required
                rows="3"
                placeholder="e.g. Soft watercolor illustration, gentle outlines, cozy and friendly"
                class="w-full resize-y rounded-md border border-input bg-background px-3 py-2 text-sm font-normal focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"></textarea>
            </label>
            <div class="grid gap-4 sm:grid-cols-2">
              <label class="grid gap-1.5 text-sm font-medium">Colors<Input bind:value={form.palette} placeholder="e.g. pastel greens and warm amber" /></label>
              <label class="grid gap-1.5 text-sm font-medium">Lighting<Input bind:value={form.lighting} placeholder="e.g. soft golden-hour light" /></label>
              <label class="grid gap-1.5 text-sm font-medium">Who it's for<Input bind:value={form.audience} placeholder="e.g. kids 6–10" /></label>
              <label class="grid gap-1.5 text-sm font-medium">Mood<Input bind:value={form.tone} placeholder="e.g. playful and curious" /></label>
            </div>
          </div>
        </details>
        <details class="rounded-md border border-border px-3">
          <summary
            class="min-h-11 cursor-pointer py-3 text-sm font-medium">Match a reference image, video or link <span
            class="font-normal text-muted-foreground">(optional)</span></summary>
          <div class="pb-4">{#if dirty}<p
            class="mb-2 text-sm text-muted-foreground">Save or undo your current changes first; a reference replaces the whole look.</p>{/if}<StyleReference
            disabled={disabled||saving||dirty}
            onsaved={async()=>{try{sessionStorage.removeItem(key)}catch{}await load()}}/></div>
        </details>
        <div class="flex flex-wrap items-center gap-3 border-t border-border pt-4">
          <Button
            type="submit"
            disabled={saving || (!dirty && hasSavedStyle) || !form.name.trim() || !form.style_prompt.trim()}>{saving ? 'Saving…' : dirty || !hasSavedStyle ? 'Save look' : 'Saved'}</Button>
          {#if dirty}<Button type="button" variant="ghost" onclick={reset}>Undo changes</Button>{/if}
          <span
            class="text-xs text-muted-foreground">{dirty
                ? 'Your choice is kept in this tab until you save.'
                : hasSavedStyle
                  ? 'New stories in this project use this look.'
                  : 'Save to use this look for new stories.'}</span>
        </div>
      </fieldset>
      {#if message}<p role="status" class="text-sm">{message}</p>{/if}
    </form>
  {/if}
</section>
