<script lang="ts">
  import { errorMessage } from '../errors';
  import { onMount, onDestroy, untrack } from 'svelte';
  import { get, patch, upload } from '$lib/api';
  import { runBackgroundOp } from '$lib/ops';
  import { Button } from '$lib/components/ui/button';
  import type { Asset, ProjectStylePreset, StyleCatalog, StyleFields, StyleGuide } from '$lib/api-types';
  type Style = StyleFields;
  /** result_json of the style reference op (internal/api/style_reference.go). */
  type ReferenceAnalysis = { style?: unknown; source: string; source_kind: string; sample_count: number; saved?: boolean };
  let {
    disabled = false,
    initialURL = '',
    initialAssetID = '',
    onsaved
  }: {
    disabled?: boolean;
    initialURL?: string;
    initialAssetID?: string;
    onsaved?: (preset: ProjectStylePreset, style: Style) => void;
  } = $props();
  let assets = $state<Asset[]>([]),
    assetId = $state(untrack(() => initialAssetID)),
    url = $state(untrack(() => initialURL)),
    focus = $state(''),
    busy = $state(false),
    error = $state(''),
    notice = $state(''),
    draft = $state<Style | null>(null),
    source = $state(''),
    kind = $state(''),
    sampleCount = $state(0),
    baseline = '',
    project: string | null | undefined = '';
  const controller = new AbortController();
  onDestroy(() => controller.abort());
  const fields = ['name', 'style_prompt', 'palette', 'lighting', 'tone', 'audience'] as const;
  const clean = (style: unknown): Style => {
    const source = (style && typeof style === 'object' ? style : {}) as Record<string, unknown>;
    return Object.fromEntries(fields.map(k => [k, typeof source[k] === 'string' ? source[k] : ''])) as Style;
  };
  const inputClass = 'min-h-11 w-full rounded-md border border-input bg-background px-3 py-2 text-sm';
  async function load() {
    try {
      assets = (await get<Asset[]>('/assets')).filter(a => /\.(png|jpe?g|gif|webp|mp4|mov|webm)$/i.test(a.file_path ?? ''));
    } catch (e) {
      error = errorMessage(e);
    }
  }
  onMount(() => {
    void load();
  });
  async function attach(file?: File) {
    if (!file) return;
    if (file.size > 50 * 1024 * 1024) {
      error = 'Choose a file smaller than 50 MB';
      return;
    }
    busy = true;
    error = '';
    try {
      const data = new FormData();
      data.set('file', file);
      data.set('type', 'other');
      const asset = await upload<Asset>('/assets/upload', data);
      assets = [asset, ...assets];
      assetId = asset.id;
      url = '';
      draft = null;
      notice = 'Reference uploaded to your Library. Analyze it to extract visual direction.';
    } catch (e) {
      error = errorMessage(e);
    } finally {
      busy = false;
    }
  }
  async function analyze() {
    busy = true;
    error = '';
    notice = '';
    draft = null;
    try {
      const current = await get<StyleGuide>('/style');
      project = current.project_id;
      baseline = JSON.stringify(clean(current));
      await runBackgroundOp<ReferenceAnalysis>(
        '/style/reference',
        url.trim() ? { url: url.trim(), instruction: focus } : { asset_id: assetId, instruction: focus },
        {
          label: 'Reference analysis',
          signal: controller.signal,
          onDone: op => {
            const result = op.result_json;
            if (!result?.style) {
              error = 'Analysis returned no editable style';
              busy = false;
              return;
            }
            draft = clean(result.style);
            source = result.source;
            kind = result.source_kind;
            sampleCount = result.sample_count;
            busy = false;
          },
          onFail: op => {
            error = op.error || 'Reference analysis failed';
            busy = false;
          }
        }
      );
    } catch (e) {
      error = errorMessage(e);
      busy = false;
    }
  }
  async function save() {
    if (!draft?.style_prompt.trim()) return;
    busy = true;
    error = '';
    try {
      const current = await get<StyleGuide>('/style');
      if (current.project_id !== project || JSON.stringify(clean(current)) !== baseline)
        throw Error('Project style changed while analyzing. Analyze again before replacing it.');
      await patch('/style', { ...current, ...draft, project_id: project });
      const catalog = await get<StyleCatalog>('/style-presets');
      if (!catalog.project_style) throw Error('Style saved. Refresh styles to select the project default.');
      baseline = JSON.stringify(draft);
      notice = 'Reference style saved as your project default. Existing stories keep their saved look.';
      onsaved?.(catalog.project_style, draft);
      window.dispatchEvent(new Event('videoflow:style-updated'));
    } catch (e) {
      error = errorMessage(e);
    } finally {
      busy = false;
    }
  }
</script>
<section aria-label="Style reference" class="space-y-3 rounded-lg border border-border p-3">
 <div><h3 class="text-sm font-medium">Start from a reference</h3><p
   class="mt-1 text-xs text-muted-foreground">Upload an image or video, choose Library media, or paste a public link. Extract visual direction, then edit it before saving.</p></div>
 <fieldset disabled={disabled||busy} class="min-w-0 space-y-3">
 <label class="grid gap-1 text-sm">Image or video file<input
   class="w-full min-w-0 text-xs file:mr-2 file:min-h-11 file:rounded-md file:border file:border-border file:bg-background file:px-3"
   type="file"
   accept="image/png,image/jpeg,image/gif,image/webp,video/mp4,video/quicktime,video/webm"
   onchange={e=>{void attach(e.currentTarget.files?.[0]);e.currentTarget.value=''}}/></label>
 <label class="grid gap-1 text-sm">Library reference<select
   class={inputClass}
   value={assetId}
   onchange={e=>{assetId=e.currentTarget.value;url='';draft=null}}><option
     value="">Choose existing image or video…</option>{#each assets as asset(asset.id)}<option value={asset.id}>{asset.name}</option>{/each}</select></label>
 <label class="grid gap-1 text-sm">Reference link<input
   class={inputClass}
   type="url"
   placeholder="https://…"
   value={url}
   oninput={e=>{url=e.currentTarget.value;assetId='';draft=null}}/></label>
 <p class="text-xs text-muted-foreground">Public HTTPS image/video links work directly. Web pages use their preview image. For YouTube, social posts or private pages without a usable preview, upload a clip or screenshot.</p>
 <label class="grid gap-1 text-sm">What should we learn? <span class="text-xs text-muted-foreground">Optional</span><input
   class={inputClass}
   maxlength="2000"
   bind:value={focus}
   placeholder="Color palette, picture-book texture, lighting…"/></label>
 <Button type="button" variant="outline" disabled={!assetId&&!url.trim()} onclick={analyze}>Analyze reference</Button>
 <p class="text-xs text-muted-foreground">Uses the configured vision API and may consume credits. Analysis does not generate media, train a model or change the saved project style.</p>
 {#if draft}
 <div class="space-y-3 border-t border-border pt-3"><h4
   class="text-sm font-medium">Extracted visual direction</h4><p
   class="break-words text-xs text-muted-foreground">{source}{kind === 'sampled_video'
       ? ` · ${sampleCount} sampled frames`
       : kind === 'web_preview'
         ? ' · web preview image'
         : ' · image reference'}</p>
 {#if kind==='sampled_video'}<p
   class="text-xs text-muted-foreground">Samples describe the look. Audio, exact pacing and the complete video were not analyzed.</p>{/if}
 {#each fields as field}<label
   class="grid gap-1 text-sm">{{
       name: 'Style name',
       style_prompt: 'Visual direction',
       palette: 'Palette',
       lighting: 'Lighting',
       tone: 'Suggested tone',
       audience: 'Suggested audience'
     }[field]}{#if field==='style_prompt'}<textarea
     class={inputClass}
     rows="4"
     maxlength="12000"
     bind:value={draft[field]}></textarea>{:else}<input
   class={inputClass}
   maxlength="500"
   bind:value={draft[field]}/>{/if}</label>{/each}
 <Button
   type="button"
   disabled={!draft.style_prompt.trim()}
   onclick={save}>Save as project style</Button><p
     class="text-xs text-muted-foreground">Replaces the project default for future work. Your saved stories and character identities stay unchanged.</p></div>
 {/if}
 </fieldset>
 {#if busy}<p role="status" class="text-sm text-muted-foreground motion-safe:animate-pulse">{draft?'Saving style…':'Uploading or analyzing reference…'}</p>{/if}
 {#if notice}<p role="status" class="text-sm">{notice}</p>{/if}{#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
</section>
