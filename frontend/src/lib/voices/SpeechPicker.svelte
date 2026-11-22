<script lang="ts">
  import { errorMessage } from '../errors';
  import { get } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  type Model = { id: string; name: string; default: boolean };
  type Voice = { id: string; name: string; description: string; preview_url?: string };
  /** GET /voice/catalog/{provider}/models (speech_catalog.go). */
  type ModelCatalog = { models: Model[]; checked_at: string; source?: string; stale: boolean; warning?: string };
  /** GET /voice/catalog/{provider}/voices */
  type VoicePage = { voices: Voice[]; next_page?: string; suggested?: Voice | null; reason: string; checked_at?: string };
  let {
    provider,
    character,
    model = $bindable('auto'),
    voice = $bindable(''),
    onchange
  }: { provider: string; character: string; model?: string; voice?: string; onchange: () => void } = $props();
  let models = $state<Model[]>([]),
    voices = $state<Voice[]>([]),
    checked = $state(''),
    warning = $state(''),
    error = $state(''),
    query = $state(''),
    language = $state('en'),
    next = $state(''),
    reason = $state(''),
    loading = $state(false),
    modelLoading = $state(false);
  let sequence = 0,
    voiceSequence = 0;
  async function loadModels(p: string, refresh = false) {
    const seq = ++sequence;
    modelLoading = true;
    error = '';
    try {
      const c = await get<ModelCatalog>(`/voice/catalog/${p}/models${refresh ? '?refresh=true' : ''}`);
      if (seq !== sequence) return;
      models = c.models;
      checked = c.checked_at;
      warning = c.stale ? 'Using an older catalog. ' + (c.warning ?? '') : '';
    } catch (e) {
      if (seq === sequence) {
        models = [];
        error = errorMessage(e);
      }
    } finally {
      if (seq === sequence) modelLoading = false;
    }
  }
  $effect(() => {
    const p = provider;
    voices = [];
    next = '';
    reason = '';
    checked = '';
    warning = '';
    loading = false;
    voiceSequence++;
    if (p === 'fish' || p === 'elevenlabs') void loadModels(p);
    else {
      models = [];
      sequence++;
      modelLoading = false;
    }
  });
  async function browse(match = false, more = false) {
    const seq = ++voiceSequence,
      p = provider;
    loading = true;
    error = '';
    try {
      const result = await get<VoicePage>(
        `/voice/catalog/${p}/voices?character_id=${encodeURIComponent(character)}&search=${encodeURIComponent(query)}&language=${encodeURIComponent(language)}${more ? '&page=' + encodeURIComponent(next) : ''}`
      );
      if (seq !== voiceSequence || p !== provider) return;
      voices = more
        ? [...voices, ...result.voices.filter(v => !voices.some(old => old.id === v.id))]
        : result.voices;
      next = result.next_page ?? '';
      if (match) {
        if (result.suggested) {
          voice = result.suggested.id;
          onchange();
          reason = result.reason;
        } else reason = result.reason ?? 'No matching voices found.';
      }
    } catch (e) {
      if (seq === voiceSequence) error = errorMessage(e);
    } finally {
      if (seq === voiceSequence) loading = false;
    }
  }
</script>
<div class="space-y-3">
 <label>Speech model<select
   aria-label="Speech model"
   bind:value={model}
   onchange={onchange}><option
     value="auto">Automatic · provider default or first compatible model</option>{#if model&&model!=='auto'&&!models.some(m=>m.id===model)}<option
     value={model}>Saved · {model}</option>{/if}{#each models as m}<option
     value={m.id}>{m.name||m.id}{m.name&&m.name!==m.id?' · '+m.id:''}{m.default?' · Provider default':''}</option>{/each}</select></label>
 <div class="flex flex-wrap items-center gap-2"><Button
   variant="outline"
   size="sm"
   disabled={modelLoading}
   onclick={()=>loadModels(provider,true)}>{modelLoading?'Checking…':'Refresh models'}</Button><span
     class="text-xs text-muted-foreground">{checked?'Checked '+new Date(checked).toLocaleString():'Official catalog not loaded'}. Automatically checked after 24 hours when opened.</span></div>
 {#if warning}<p role="status" class="text-sm text-muted-foreground">{warning}</p>{/if}
 <details><summary class="cursor-pointer text-sm font-medium">Choose or match a voice</summary><div class="mt-3 space-y-3"><label>Voice search<input
   aria-label="Voice search"
   bind:value={query}
   placeholder="Warm, gentle, narrator…"/></label><label>Voice language<input
   aria-label="Voice language"
   bind:value={language}
   placeholder="en"/></label><div class="flex flex-wrap gap-2"><Button
   variant="outline"
   disabled={loading}
   onclick={()=>browse()}>Browse voices</Button><Button disabled={loading}
     onclick={()=>browse(true)}>{loading?'Finding voices…':'Auto-match voice'}</Button></div><p
     class="text-xs text-muted-foreground">Matches character descriptions and voice rules against returned voice metadata within your filters. Saved voices stay unchanged until you choose and save.</p>{#if reason}<p
     role="status" class="text-sm">{reason}</p>{/if}
 <ul
   class="max-h-64 space-y-3 overflow-y-auto"
   aria-label="Available voices">{#each voices as v}<li class="border-b border-border pb-3"><div
     class="flex items-center justify-between gap-2"><span class="font-medium">{v.name}</span><Button
   variant="outline"
   size="sm"
   aria-pressed={voice===v.id}
   onclick={()=>{voice=v.id;reason='Selected '+v.name;onchange()}}>{voice===v.id?'Selected':'Use voice'}</Button></div><p
     class="text-xs text-muted-foreground">{v.description}</p>{#if v.preview_url?.startsWith('https://')}<audio
   class="mt-2 w-full"
   controls
   preload="none"
   src={v.preview_url}
   aria-label={`Preview ${v.name}`}></audio>{/if}</li>{/each}</ul>{#if next}<Button
   variant="outline"
   disabled={loading}
   onclick={()=>browse(false,true)}>More voices</Button>{/if}</div></details>
 {#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
</div>
<style>
  label {
    display: flex;
    flex-direction: column;
    gap: 6px;
    font-size: 12px;
    color: var(--foreground);
  }
  input,
  select {
    width: 100%;
    min-height: 44px;
    border: 1px solid var(--input);
    background: var(--background);
    color: var(--foreground);
    border-radius: 6px;
    padding: 8px 10px;
    font: inherit;
  }
  input:focus-visible,
  select:focus-visible,
  summary:focus-visible {
    outline: 2px solid var(--ring);
    outline-offset: 3px;
  }
  summary {
    min-height: 44px;
    padding: 10px 0;
  }
</style>
