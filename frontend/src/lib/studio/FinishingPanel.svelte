<script lang="ts">
  import { onMount } from 'svelte';
  import { get, post, put } from '$lib/api';
  import { layouts, readLayout, useLayout } from './layouts.mjs';
  import type { Project } from '$lib/api-types';
  import type { CompositionManifest, CompositionSource, Cue } from '$lib/composition/types';
  type Layout = (typeof layouts)[number];
  /** Project brand (internal/api/studio_brand.go). */
  type Brand = { name: string; primary: string; text: string; font: string; logo_asset_id: string; revision: string };
  /** POST /compositions/localize, plus the draft it was made from. */
  type Translation = { captions: Cue[]; language: string; execution?: string; audio_changed?: boolean; base: string };
  let {
    draft,
    assets,
    disabled = false,
    onchange,
    onbusy,
    onexport
  }: {
    draft: CompositionManifest;
    assets: CompositionSource[];
    disabled?: boolean;
    onchange: (draft: CompositionManifest) => void;
    onbusy: (busy: boolean) => void;
    onexport: () => Promise<void>;
  } = $props();
  let error = $state(''),
    notice = $state(''),
    language = $state(''),
    translation = $state<Translation | null>(null),
    brand = $state<Brand>({
      name: '',
      primary: '#111827',
      text: '#FFFFFF',
      font: 'sans-serif',
      logo_asset_id: '',
      revision: ''
    }),
    custom = $state<Layout[]>([]),
    layoutIndex = $state(0),
    storage = '';
  const clone = <T,>(value: T): T => JSON.parse(JSON.stringify(value));
  onMount(() => {
    void (async () => {
      try {
        const project = await get<Project>('/projects/active');
        storage = 'videoflow.layouts.' + project.id;
        brand = await get<Brand>('/studio/brand');
        try {
          custom = JSON.parse(localStorage.getItem(storage) || '[]').map((p: unknown) =>
            readLayout({ format: 'videoflow-layout', version: 1, layout: p })
          );
        } catch {
          custom = [];
        }
      } catch (e) {
        error = String(e);
      }
    })();
  });
  function change(next: CompositionManifest) {
    onchange(next);
    translation = null;
  }
  async function translate() {
    onbusy(true);
    error = '';
    const base = JSON.stringify(draft);
    try {
      const result = await post<Omit<Translation, 'base'>>('/compositions/localize', { captions: draft.captions, language });
      translation = { ...result, base };
      notice = 'Translation is ready to edit. Spoken audio has not changed.';
    } catch (e) {
      error = String(e);
    } finally {
      onbusy(false);
    }
  }
  function applyTranslation() {
    if (!translation || JSON.stringify(draft) !== translation.base) {
      error = 'Captions changed after translation. Translate the updated version.';
      return;
    }
    const variants = [...(draft.caption_variants ?? [])];
    if (!variants.length) variants.push({ language: 'Original', captions: clone(draft.captions) });
    const translated = { language: translation.language, captions: translation.captions, bindings: [] };
    const next = {
      ...draft,
      captions: translated.captions,
      caption_bindings: [],
      caption_variants: [...variants.filter(v => v.language !== translated.language), translated]
    };
    change(next);
  }
  async function saveBrand() {
    onbusy(true);
    error = '';
    try {
      brand = await put<Brand>('/studio/brand', brand);
      notice = 'Project brand saved.';
    } catch (e) {
      error = String(e);
    } finally {
      onbusy(false);
    }
  }
  function applyBrand() {
    let next = {
      ...draft,
      texts: (draft.texts ?? []).map(t => ({
        ...t,
        color: brand.text,
        background: brand.primary,
        font: brand.font
      }))
    };
    if (brand.logo_asset_id) {
      const logo = {
        id: 'brand-logo',
        asset_id: brand.logo_asset_id,
        start: 0,
        trim: 0,
        duration: draft.duration,
        speed: 1,
        has_audio: false,
        overlay: { x: 80, y: 5, width: 15, height: 15 }
      };
      next.clips = [...draft.clips.filter(c => c.id !== 'brand-logo'), logo];
    }
    change(next);
    notice = 'Brand applied to title layers and logo. Source media stays unchanged.';
  }
  async function importLayout(file?: File) {
    if (!file) return;
    try {
      if (file.size > 65536) throw Error('Layout file must be smaller than 64 KB');
      const layout = readLayout(JSON.parse(await file.text()));
      custom = [...custom, layout];
      localStorage.setItem(storage, JSON.stringify(custom));
      notice = 'Layout imported. Choose it below to apply.';
    } catch (e) {
      error = String(e);
    }
  }
  function exportLayout() {
    const selected = [...layouts, ...custom][layoutIndex];
    const current = {
      ...selected,
      width: draft.width,
      height: draft.height,
      title: draft.texts?.[0]?.text ?? selected.title,
      position: draft.texts?.[0]?.position ?? selected.position
    };
    const url = URL.createObjectURL(
      new Blob([JSON.stringify({ format: 'videoflow-layout', version: 1, layout: current }, null, 2)], {
        type: 'application/json'
      })
    );
    const a = document.createElement('a');
    a.href = url;
    a.download = 'videoflow-layout.json';
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
</script>
<section
  aria-label="Finish video"
  class="space-y-4 rounded-lg border border-border p-4"><h2
    class="text-base font-medium">More caption tools</h2><p class="text-sm text-muted-foreground">Other languages and title layouts.</p>
 <details><summary class="min-h-11 cursor-pointer py-3 text-sm font-medium">Caption languages</summary><div
   class="space-y-3"><label class="grid gap-1 text-sm">Target language<input
   maxlength="80"
   bind:value={language}
   placeholder="Spanish, Malay, Mandarin…"/></label><button
   disabled={disabled||!language.trim()||!draft.captions.length}
   onclick={translate}>Translate with local session</button><p
     class="text-xs text-muted-foreground">Preserves subtitle timings. This translates captions, not the spoken audio or lip movements.</p>
 {#if translation}<div class="space-y-2"><h3
   class="text-sm font-medium">Edit translation</h3>{#each translation.captions as cue(cue.id)}<label
   class="grid gap-1 text-xs">{cue.start.toFixed(2)}–{cue.end.toFixed(2)}s<textarea bind:value={cue.text} rows="2"></textarea></label>{/each}<button
   disabled={disabled} onclick={applyTranslation}>Use translated captions</button></div>{/if}
 {#if draft.caption_variants?.length}<label class="grid gap-1 text-sm">Saved language version<select
   onchange={e=>{const variant=draft.caption_variants![Number(e.currentTarget.value)];change({...draft,captions:clone(variant.captions)})}}><option
   value="">Choose a saved caption version…</option>{#each draft.caption_variants as variant,i}<option
   value={i}>{variant.language}</option>{/each}</select></label>{/if}</div></details>
 <details><summary class="min-h-11 cursor-pointer py-3 text-sm font-medium">Layouts & brand</summary><div
   class="space-y-3"><label class="grid gap-1 text-sm">Reusable layout<select
   bind:value={layoutIndex}>{#each [...layouts,...custom] as layout,i}<option
   value={i}>{layout.name}</option>{/each}</select></label><div class="flex flex-wrap gap-2"><button
   disabled={disabled}
   onclick={()=>change(useLayout(draft,[...layouts,...custom][layoutIndex],brand))}>Add layout title</button><button
     onclick={exportLayout}>Export layout JSON</button></div><label class="grid gap-1 text-sm">Import community layout<input
   type="file"
   accept="application/json,.json"
   onchange={e => {
     void importLayout(e.currentTarget.files?.[0]);
     e.currentTarget.value = '';
   }}/></label>
 <h3 class="text-sm font-medium">Project brand</h3><label class="grid gap-1 text-sm">Brand name<input
   bind:value={brand.name} maxlength="120"/></label><div class="grid grid-cols-2 gap-3"><label
   class="grid gap-1 text-sm">Background color<input type="color" bind:value={brand.primary}/></label><label
   class="grid gap-1 text-sm">Text color<input type="color" bind:value={brand.text}/></label></div><label
   class="grid gap-1 text-sm">Font family<select bind:value={brand.font}><option
   value="sans-serif">Sans serif</option><option value="serif">Serif</option><option
   value="monospace">Monospace</option></select></label><label class="grid gap-1 text-sm">Logo image<select
   bind:value={brand.logo_asset_id}><option
   value="">No logo</option>{#each assets.filter(a=>a.media_kind==='image') as asset}<option
   value={asset.id}>{asset.name}</option>{/each}</select></label><div class="flex flex-wrap gap-2"><button
   disabled={disabled||!brand.revision}
   onclick={saveBrand}>Save project brand</button><button disabled={disabled} onclick={applyBrand}>Apply brand to this edit</button></div></div></details>
 {#if draft.texts?.length}<details open><summary
   class="min-h-11 cursor-pointer py-3 text-sm font-medium">Title & end cards</summary>{#each draft.texts as layer,i(layer.id)}<div
   class="space-y-2 border-t border-border py-3"><label class="grid gap-1 text-sm">Title text<textarea
   value={layer.text}
   oninput={e =>
     change({
       ...draft,
       texts: (draft.texts ?? []).map((t, j) => (j === i ? { ...t, text: e.currentTarget.value } : t))
     })}></textarea></label><div class="grid grid-cols-2 gap-3">{#each (['start','end'] as const) as key}<label
       class="grid gap-1 text-sm">{key==='start'?'Starts':'Ends'} (s)<input
   type="number"
   min="0"
   max={draft.duration}
   step=".1"
   value={layer[key]}
   oninput={e =>
     change({
       ...draft,
       texts: (draft.texts ?? []).map((t, j) => (j === i ? { ...t, [key]: Number(e.currentTarget.value) } : t))
     })}/></label>{/each}</div><button
       onclick={()=>change({...draft,texts:(draft.texts ?? []).filter((_,j)=>j!==i)})}>Remove title</button></div>{/each}</details>{/if}
 {#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}{#if notice}<p role="status" class="text-sm">{notice}</p>{/if}
</section>
<style>
  button {
    min-height: 44px;
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px 12px;
    background: var(--secondary);
    font-size: 13px;
  }
  button:disabled {
    opacity: 0.5;
  }
  input,
  select,
  textarea {
    min-height: 44px;
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px;
    background: var(--background);
    color: var(--foreground);
    min-width: 0;
    width: 100%;
  }
  button:focus-visible,
  input:focus-visible,
  select:focus-visible,
  textarea:focus-visible {
    outline: 2px solid var(--ring);
    outline-offset: 2px;
  }
</style>
