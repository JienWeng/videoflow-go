<script lang="ts">
  import { errorMessage } from '../errors';
  import BriefChoice from './BriefChoice.svelte';
  import StyleReference from './StyleReference.svelte';
  import { onMount } from 'svelte';
  import { get } from '$lib/api';
  import type { ProjectStylePreset, StyleCatalog, StyleFields } from '$lib/api-types';
  import { Button } from '$lib/components/ui/button';
  import StyleCards from './StyleCards.svelte';
  import { normalizeBrief, missingBriefFields } from './brief.mjs';
  type Brief = ReturnType<typeof normalizeBrief>;
  let {
    value = $bindable(normalizeBrief()),
    confirmed = $bindable(false),
    disabled = false,
    onchange,
    onstyleapplied
  }: {
    value?: Brief;
    confirmed?: boolean;
    disabled?: boolean;
    onchange?: (brief: Brief) => void;
    onstyleapplied?: (preset: ProjectStylePreset, style: StyleFields) => void;
  } = $props();
  let catalog = $state<StyleCatalog | null>(null),
    loading = $state(false),
    error = $state(''),
    step = $state(0),
    attempted = $state(false);
  const labels = ['Look & mood', 'Delivery', 'Length & scenes', 'Confirm'];
  const options = $derived(
    catalog
      ? [
          ...(catalog.project_style
            ? [
                {
                  ...catalog.project_style,
                  name: `Project style${catalog.project_style.name ? ' · ' + catalog.project_style.name : ''}`
                }
              ]
            : []),
          ...(catalog.presets ?? [])
        ]
      : []
  );
  const selected = $derived(options.find(p => p.id === value.style_preset_id));
  const styleValid = $derived(!!selected?.fingerprint && selected.fingerprint === value.style_fingerprint);
  const missing = $derived(missingBriefFields(value));
  const stepReady = $derived(
    step === 0
      ? styleValid && !!value.tone.trim() && !!value.audience.trim()
      : step === 1
        ? !!value.platform.trim() &&
          !!value.aspect_ratio &&
          !!value.resolution &&
          !!value.language.trim() &&
          !!value.conversation_mode
        : step === 2
          ? Number.isInteger(value.scene_count) &&
            Number.isInteger(value.target_duration) &&
            value.scene_count >= 1 &&
            value.scene_count <= 20 &&
            value.target_duration >= value.scene_count &&
            value.target_duration <= 600
          : missing.length === 0 && styleValid
  );
  async function load() {
    error = '';
    loading = true;
    confirmed = false;
    try {
      const result = await get<StyleCatalog | null>('/style-presets');
      if (!Array.isArray(result?.presets)) throw Error('Style catalog is unavailable');
      catalog = result;
      if (value.style_preset_id && !styleValid) confirmed = false;
    } catch (e) {
      error = errorMessage(e);
    } finally {
      loading = false;
    }
  }
  onMount(() => {
    void load();
    if (!missing.length) step = 3;
  });
  function change(patch: Partial<Brief>) {
    value = { ...value, ...patch };
    confirmed = false;
    attempted = false;
    onchange?.(JSON.parse(JSON.stringify(value)));
  }
  function recommend() {
    const p =
      options.find(p => p.id === 'project-style') ?? options.find(p => p.id === 'live-action-documentary') ?? options[0];
    if (!p) return;
    change({
      style_preset_id: p.id,
      style_fingerprint: p.fingerprint,
      tone: value.tone || 'Warm and hopeful',
      audience: value.audience || 'Families',
      platform: value.platform || 'YouTube Shorts',
      aspect_ratio: value.aspect_ratio || '9:16',
      resolution: value.resolution || '720p',
      language: value.language || 'English',
      conversation_mode: value.conversation_mode || 'dialogue',
      target_duration: value.target_duration || 8,
      scene_count: value.scene_count || 1
    });
    step = 3;
  }
  const inputClass = 'min-h-11 w-full rounded-md border border-input bg-background px-3 py-2 text-sm';
</script>
<section aria-label="Creative brief" class="space-y-4 rounded-lg border border-border p-3 sm:p-4">
 <div class="flex flex-wrap items-center justify-between gap-2"><h3 class="text-base font-medium">Plan your video</h3><Button
   type="button"
   size="sm"
   variant="outline"
   disabled={disabled||loading||!catalog}
   onclick={recommend}>Use suggested settings</Button></div>
 <nav
   aria-label="Creative brief steps"
   class="grid grid-cols-2 gap-1">{#each labels as label,index}<button
   type="button"
   class={`min-h-11 rounded-md px-2 text-left text-xs ${step===index?'bg-accent font-medium':'text-muted-foreground hover:bg-accent/50'}`}
   aria-current={step===index?'step':undefined}
   disabled={disabled}
   onclick={()=>{step=index;attempted=false}}>{index+1}. {label}</button>{/each}</nav>
 <fieldset disabled={disabled} class="min-w-0 space-y-3">
 {#if step===0}
 <details class="rounded-md border border-border p-2"><summary
   class="min-h-11 cursor-pointer py-3 text-sm font-medium">Use an image, video or link</summary><StyleReference
   {disabled}
   onsaved={async (p, style) => {
     await load();
     change({
       style_preset_id: p.id,
       style_fingerprint: p.fingerprint,
       tone: value.tone || style.tone,
       audience: value.audience || style.audience
     });
     onstyleapplied?.(p, style);
   }}/></details>
 <Button type="button" size="sm" variant="ghost" disabled={disabled||loading} onclick={load}>{loading?'Loading styles…':'Refresh styles'}</Button>
 {#if catalog}<StyleCards
   presets={options}
   selected={value.style_preset_id}
   disabled={disabled||loading}
   onselect={p=>change({style_preset_id:p.id,style_fingerprint:p.fingerprint??''})}/>{:else if !error}<p
     role="status" class="text-sm text-muted-foreground">Loading visual styles…</p>{/if}
 <p class="text-xs text-muted-foreground">This look applies to this story. It does not overwrite your project default.</p>
 <label class="grid gap-1 text-sm">Storytelling method<select
   class={inputClass}
   value={value.storytelling_id||'custom'}
   onchange={e=>change({storytelling_id:e.currentTarget.value})}><option
     value="custom">Follow my story</option>{#each catalog?.storytelling_presets??[] as method}<option
     value={method.id}>{method.name}</option>{/each}</select></label>
 {#if value.storytelling_id&&value.storytelling_id!=='custom'}<p
   class="text-xs text-muted-foreground">{catalog?.storytelling_presets?.find(p => p.id === value.storytelling_id)?.description ??
     'Saved storytelling method'}</p>{/if}
 <p class="text-xs text-muted-foreground">Storytelling shapes the pacing and direction. Your visual style stays separate.</p>
 <BriefChoice
   label="Tone"
   value={value.tone}
   options={['Documentary', 'Dramatic', 'Warm and hopeful', 'Playful', 'Suspenseful']}
   maxlength={200}
   onchange={v=>change({tone:v})}/>
 <BriefChoice
   label="Audience"
   value={value.audience}
   options={['General audience', 'Families', 'Young children', 'Teens', 'Adults']}
   maxlength={200}
   onchange={v=>change({audience:v})}/>
 {:else if step===1}
 <BriefChoice
   label="Destination platform"
   value={value.platform}
   options={['YouTube', 'YouTube Shorts', 'TikTok', 'Instagram Reels', 'LinkedIn', 'Website']}
   maxlength={120}
   onchange={v=>change({platform:v})}/>
 <div class="grid grid-cols-2 gap-3"><label class="grid gap-1 text-sm">Format<select
   class={inputClass}
   value={value.aspect_ratio}
   onchange={e=>change({aspect_ratio:e.currentTarget.value})}><option value="">Choose format…</option><option
     value="9:16">Portrait · 9:16</option><option value="16:9">Landscape · 16:9</option><option
     value="1:1">Square · 1:1</option><option value="3:4">Portrait · 3:4</option><option
     value="4:3">Landscape · 4:3</option><option value="21:9">Ultrawide · 21:9</option></select></label><label class="grid gap-1 text-sm">Video size<select
   class={inputClass}
   value={value.resolution}
   onchange={e=>change({resolution:e.currentTarget.value})}><option value="">Choose size…</option><option
     value="720p">720p</option><option value="480p">480p</option><option value="auto">Model default</option></select></label></div>
 <p class="text-xs text-muted-foreground">Shorts, Reels and TikTok usually use portrait. Size support depends on the video model.</p>
 <BriefChoice
   label="Spoken language"
   value={value.language}
   options={['English', 'Malay', 'Mandarin', 'Tamil', 'Spanish', 'Japanese', 'French']}
   maxlength={80}
   onchange={v=>change({language:v})}/>
 <label class="grid gap-1 text-sm">Speech<select
   class={inputClass}
   value={value.conversation_mode}
   onchange={e=>change({conversation_mode:e.currentTarget.value})}><option
     value="dialogue">Character dialogue · characters speak</option><option
     value="narration">Narration · a voice tells the story</option><option
     value="silent">No speech · silent video</option></select>{#if value.conversation_mode==='silent'}<span
   role="note"
   class="text-xs text-amber-600 dark:text-amber-400">The video will have no voices. Choose dialogue or narration to hear speech.</span>{/if}</label>
 {#if value.conversation_mode!=='silent'}<label class="grid gap-1 text-sm">Music (optional)<input
   class={inputClass}
   placeholder="For example: slow sparse piano with low strings"
   value={value.music??''}
   oninput={e=>change({music:e.currentTarget.value})}/><span
     class="text-xs text-muted-foreground">Made by the video model together with the voice. Leave empty for no music; you can also add your own track later in Edit video.</span></label>{/if}
 {:else if step===2}
 <label class="grid gap-1 text-sm">Total video duration (seconds)<input
   class={inputClass}
   type="number"
   min="1"
   max="600"
   step="1"
   value={value.target_duration||''}
   oninput={e=>change({target_duration:Number(e.currentTarget.value)})}/></label>
 <label class="grid gap-1 text-sm">Number of scenes<input
   class={inputClass}
   type="number"
   min="1"
   max="20"
   step="1"
   value={value.scene_count||''}
   oninput={e=>change({scene_count:Number(e.currentTarget.value)})}/></label>
 <p class="text-xs text-muted-foreground">This is the duration of the complete story. A scene can contain several shots. Scenes do not automatically mean separate paid video calls; the render plan groups shots later.</p>
 {:else}
 <h4 class="text-sm font-medium">Confirm your creative brief</h4><dl
   class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3 gap-y-2 text-sm"
   aria-label="Creative brief summary">{#each [
       ['Story', value.idea],
       ['Direction', value.instruction || 'None'],
       ['Visual style', selected?.name || 'Not selected'],
       [
         'Storytelling',
         catalog?.storytelling_presets?.find(p => p.id === value.storytelling_id)?.name ||
           (value.storytelling_id && value.storytelling_id !== 'custom' ? value.storytelling_id : 'Follow my story')
       ],
       ['Tone', value.tone],
       ['Audience', value.audience],
       ['Platform', value.platform],
       ['Format', value.aspect_ratio],
       ['Size', value.resolution],
       ['Language', value.language],
       ['Speech', value.conversation_mode],
       ['Music', value.conversation_mode === 'silent' ? 'None' : value.music || 'None'],
       ['Total duration', value.target_duration ? `${value.target_duration} seconds` : ''],
       ['Scenes', value.scene_count ? String(value.scene_count) : '']
     ] as [label,text]}<dt
     class="text-muted-foreground">{label}</dt><dd class="min-w-0 break-words">{text||'Not chosen'}</dd>{/each}</dl>
 {#if missing.length||!styleValid}<p role="status"
   class="text-sm text-muted-foreground">Complete the missing choices using the steps above. {missing.join(', ')}{!styleValid?' · Choose the current visual style':''}</p>{/if}
 <label class="flex min-h-11 items-start gap-2 text-sm"><input
   class="mt-1"
   type="checkbox"
   checked={confirmed}
   disabled={loading||missing.length>0||!styleValid}
   onchange={e=>confirmed=e.currentTarget.checked}/>These are the settings I want to use</label>
 <p class="text-xs text-muted-foreground">Confirmation does not start generation. The Create action uses this brief to plan the story and generate references; account quota or provider credits may apply. Video rendering stays separate.</p>
 {/if}
 </fieldset>
 {#if step<3}<div class="flex flex-wrap gap-2"><Button
   type="button"
   size="sm"
   disabled={disabled}
   onclick={()=>{if(stepReady){step++;attempted=false}else attempted=true}}>Continue</Button>{#if step>0}<Button
   type="button"
   size="sm"
   variant="ghost"
   disabled={disabled}
   onclick={()=>step--}>Back</Button>{/if}</div>{:else}<Button
   type="button"
   size="sm"
   variant="outline"
   disabled={disabled}
   onclick={()=>step=0}>Edit the look</Button>{/if}
 {#if attempted&&!stepReady}<p role="status" class="text-sm text-destructive">Complete the choices in this step before continuing.</p>{/if}
 {#if error}<p role="alert" class="text-sm text-destructive">{error}</p><Button
   type="button"
   size="sm"
   variant="outline"
   onclick={load}>Reload styles</Button>{/if}
</section>
