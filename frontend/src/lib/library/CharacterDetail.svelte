<script lang="ts">
  import { errorMessage } from '$lib/errors';
  import { page } from '$app/state';
  const returnScene = $derived(page.url.searchParams.get('scene_id'));
  import { onMount } from 'svelte';
  import { get, patch, post, upload } from '$lib/api';
  import { protectDraft } from '$lib/protect-draft';
  import { REFERENCE_IMAGE_HINT, uploadProblem } from '$lib/upload-limits';
  import CharacterReferences from '$lib/canvas/CharacterReferences.svelte';
  import { Button } from '$lib/components/ui/button';
  import { Input } from '$lib/components/ui/input';
  import { Textarea } from '$lib/components/ui/textarea';
  import { Label } from '$lib/components/ui/label';
  import { ArrowLeft, Save, Upload } from '@lucide/svelte';
  import { toast } from 'svelte-sonner';
  import type { Character } from '$lib/api-types';
  let { characterId }: { characterId: string } = $props();
  let character = $state<Character | null>(null),
    loading = $state(true),
    error = $state(''),
    busy = $state(''),
    baseline = $state(''),
    referencesVersion = $state(0);
  let bibleNotes = $state('');
  let form = $state({
    name: '',
    description: '',
    appearance: '',
    personality: '',
    visual_rules: '',
    voice_rules: '',
    sample_dialogue: ''
  });
  const dirty = $derived(!!character && JSON.stringify(form) !== baseline);
  protectDraft(() => dirty);
  function populate(c: Character) {
    character = c;
    form = {
      name: c.name ?? '',
      description: c.description ?? '',
      appearance: c.appearance ?? '',
      personality: c.personality ?? '',
      visual_rules: (c.visual_rules_json ?? []).join('\n'),
      voice_rules: (c.voice_rules_json ?? []).join('\n'),
      sample_dialogue: c.sample_dialogue ?? ''
    };
    baseline = JSON.stringify(form);
  }
  async function load() {
    loading = true;
    error = '';
    try {
      populate(await get<Character>(`/characters/${encodeURIComponent(characterId)}`));
    } catch (e) {
      error = errorMessage(e);
    } finally {
      loading = false;
    }
  }
  onMount(load);
  async function save(e: Event) {
    e.preventDefault();
    if (busy || !form.name.trim()) return;
    busy = 'save';
    error = '';
    try {
      const rules = (value: string) =>
        value
          .split('\n')
          .map(s => s.trim())
          .filter(Boolean);
      populate(
        await patch<Character>(`/characters/${encodeURIComponent(characterId)}`, {
          name: form.name.trim(),
          description: form.description,
          appearance: form.appearance,
          personality: form.personality,
          visual_rules_json: rules(form.visual_rules),
          voice_rules_json: rules(form.voice_rules),
          sample_dialogue: form.sample_dialogue.trim()
        })
      );
      toast.success('Character saved.');
    } catch (e) {
      error = errorMessage(e);
    } finally {
      busy = '';
    }
  }
  async function generateBible() {
    if (busy || dirty) return;
    const notes = bibleNotes.trim() || character?.description || '';
    busy = 'bible';
    error = '';
    try {
      await post(`/characters/${encodeURIComponent(characterId)}/bible`, { notes });
      populate(await get<Character>(`/characters/${encodeURIComponent(characterId)}`));
      toast.success('Character bible updated.');
    } catch (e) {
      error = errorMessage(e);
    } finally {
      busy = '';
    }
  }
  async function uploadPhotos(e: Event) {
    const input = e.target as HTMLInputElement;
    const files = Array.from(input.files ?? []);
    if (!files.length) return;
    const problem = uploadProblem(files, true);
    input.value = '';
    if (problem) {
      toast.error(problem);
      return;
    }
    busy = 'upload';
    error = '';
    let uploaded = 0;
    try {
      for (const file of files) {
        const body = new FormData();
        body.append('file', file);
        body.append('character_id', characterId);
        await upload('/assets/upload', body);
        uploaded++;
      }
      toast.success(`${uploaded} reference photo${uploaded === 1 ? '' : 's'} uploaded.`);
    } catch (e) {
      error = errorMessage(e);
    } finally {
      if (uploaded) referencesVersion++;
      busy = '';
    }
  }
</script>

<svelte:head><title>{character?.name??'Character'} · VideoFlow</title></svelte:head>
<main class="mx-auto w-full max-w-7xl px-4 py-6 sm:px-8 sm:py-8">
  <a
    href={returnScene ? `/?view=story&scene_id=${encodeURIComponent(returnScene)}&focus=look` : "/library?tab=characters"}
    class="mb-6 inline-flex min-h-11 items-center gap-2 text-sm text-muted-foreground hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring"><ArrowLeft
      class="size-4"/>{returnScene ? 'Back to scene look' : 'Characters'}</a>
  {#if loading}<p role="status" class="py-16 text-muted-foreground">Loading character…</p>
  {:else if !character}<div role="alert" class="space-y-4 py-12"><h1
    class="text-2xl font-semibold">Could not load character</h1><p
    class="text-sm text-destructive">{error}</p><Button variant="outline" onclick={load}>Try again</Button></div>
  {:else}
    <header class="mb-8 flex flex-wrap items-start justify-between gap-4 border-b border-border pb-6"><div
      class="min-w-0"><h1 class="break-words text-3xl font-semibold">{character.name}</h1><p
      class="mt-2 max-w-2xl text-sm leading-relaxed text-muted-foreground">Define their appearance and personality, then build a consistent set of reference images.</p></div><Button
        variant="outline"
        href={`/voices?character_id=${encodeURIComponent(characterId)}`}>Voice</Button></header>
    {#if error}<p role="alert" class="mb-6 rounded-lg border border-destructive p-4 text-sm text-destructive">{error}</p>{/if}
    <div class="grid items-start gap-10 lg:grid-cols-[minmax(0,0.9fr)_minmax(0,1.2fr)]">
      <section aria-labelledby="character-details-heading" class="min-w-0">
        <div class="mb-5 flex flex-wrap items-center justify-between gap-3"><h2
          id="character-details-heading"
          class="text-xl font-semibold">Character details</h2></div>
        <details class="mb-5 rounded-lg border p-3"><summary
          class="cursor-pointer text-sm">Develop character with AI</summary><label class="mt-3 grid gap-2 text-sm">Direction for the character<Textarea
          rows={3}
          bind:value={bibleNotes}
          placeholder="Keep the same appearance; make their personality more curious…"/></label><Button
          class="mt-3"
          variant="outline"
          size="sm"
          disabled={!!busy||dirty}
          onclick={generateBible}>{busy==='bible'?'Developing…':'Generate character details'}</Button><p
            class="mt-2 text-xs text-muted-foreground">Uses the selected AI session. Save your current edits first.</p></details>
        <form onsubmit={save} class="space-y-5">
          <fieldset disabled={!!busy} class="space-y-5">
            <div class="space-y-2"><Label for="character-name">Name</Label><Input id="character-name" required bind:value={form.name}/></div>
            <div class="space-y-2"><Label for="character-description">Description</Label><Textarea
              id="character-description"
              rows={3}
              bind:value={form.description}/></div>
            <div class="space-y-2"><Label for="character-appearance">Appearance</Label><Textarea
              id="character-appearance"
              rows={4}
              bind:value={form.appearance}/></div>
            <div class="space-y-2"><Label for="character-personality">Personality</Label><Textarea
              id="character-personality"
              rows={3}
              bind:value={form.personality}/></div>
            <div class="space-y-2"><Label for="character-visual">Visual rules (one per line)</Label><Textarea
              id="character-visual"
              rows={4}
              bind:value={form.visual_rules}/></div>
            <div class="space-y-2"><Label for="character-voice">Voice rules (one per line)</Label><Textarea
              id="character-voice"
              rows={3}
              bind:value={form.voice_rules}/></div>
            <div class="space-y-2"><Label for="character-dialogue">Sample dialogue</Label><Textarea
              id="character-dialogue"
              rows={3}
              bind:value={form.sample_dialogue}/></div>
          </fieldset>
          <div class="flex flex-wrap items-center gap-3 border-t border-border pt-5"><Button
            type="submit"
            disabled={!!busy||!dirty||!form.name.trim()}><Save class="size-4"/>{busy==='save'?'Saving…':'Save changes'}</Button><Button
            type="button"
            variant="outline"
            disabled={!!busy||!dirty}
            onclick={() => {
              if (character && window.confirm('Discard your character edits?')) populate(character);
            }}>Discard changes</Button>{#if dirty}<span role="status" class="text-xs text-muted-foreground">Unsaved changes</span>{/if}</div>
        </form>
      </section>
      <div class="min-w-0 space-y-8">
        {#key referencesVersion}<CharacterReferences {characterId} disabled={dirty||!!busy} onchanged={()=>{}}/>{/key}
        <section
          id="reference-photos"
          aria-labelledby="reference-upload-heading"
          class="space-y-3 border-t border-border pt-6"><h2
          id="reference-upload-heading"
          class="text-lg font-semibold">Add reference photos</h2><p
            class="text-sm leading-relaxed text-muted-foreground">Upload existing photos or character sheets to guide future images.</p><label
            class="inline-flex min-h-11 cursor-pointer items-center gap-2 rounded-md border border-input px-4 text-sm hover:bg-accent focus-within:outline-2 focus-within:outline-ring"><Upload
            class="size-4"/>{busy==='upload'?'Uploading…':'Upload reference photos'}<input
          type="file"
          class="sr-only"
          multiple
          accept=".jpg,.jpeg,.png,.gif,.webp"
          disabled={!!busy||dirty}
          aria-describedby="reference-upload-hint"
          onchange={uploadPhotos}/></label><p
          id="reference-upload-hint"
          class="text-xs text-muted-foreground">{REFERENCE_IMAGE_HINT}</p>{#if dirty}<p
            class="text-xs text-muted-foreground">Save or discard changes before uploading photos.</p>{/if}</section>
      </div>
    </div>
  {/if}
</main>
