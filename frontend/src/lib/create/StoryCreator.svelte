<script lang="ts">
  let { embedded = false }:{embedded?:boolean}=$props();
  import { onDestroy, onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { protectDraft } from '$lib/protect-draft';
  import { Clapperboard, Sparkles } from '@lucide/svelte';
  import { Button } from '$lib/components/ui/button';
  import GenerationProgress from '$lib/create/GenerationProgress.svelte';
  import { get, post, mediaUrl } from '$lib/api';

  let idea = $state('');
  let submitted=$state(false);
  const draftDirty=()=>!submitted&&!!(idea.trim()||instruction.trim()||selectedCharacters.length||style!==defaultStyle||aspectRatio!=='9:16'||sceneCount!==1||language!=='English'||duration||!createCharacters);
  export function canClose(){return !draftDirty()||window.confirm('Discard this story draft?');}
  protectDraft(draftDirty,'Discard this story draft and leave?');
  let style = $state('2d-picture-book');
 let defaultStyle=$state('2d-picture-book');let styleTouched=false;
 let styleOptions:any[]=$state([]);let presetError=$state('');
 async function loadStyles(){try{const result=await get('/style-presets');if(!Array.isArray(result?.presets))return;styleOptions=result.presets;if(result.has_project_style&&!styleTouched&&!submitted){defaultStyle='project-style';style='project-style'}presetError=''}catch(e){presetError='Could not load extra styles. Reopen this form to retry.'}}

  let aspectRatio = $state('9:16');
  let duration = $state('');
  let sceneCount=$state(1);
  let language = $state('English');
  const conversationMode = 'dialogue';
  let characters:any[]=$state([]);
  let selectedCharacters:string[]=$state([]);
  let createCharacters=$state(true);
  let instruction = $state('');
  let busy = $state(false);
  let opId = $state('');
  let opStatus = $state('');
  let opError = $state('');
  let stages = $state<string[]>([]);
  let automaticReferences=$state(false),continuingStory=$state(false);
  const characterReferenceIds=()=>lastResult?.cast_character_ids?.length?lastResult.cast_character_ids.flatMap((id:string)=>characters.find(c=>c.id===id)?.reference_asset_ids_json??[]):lastResult?.character_reference_ids??[];
  const missingCharacterReferences=()=>lastResult?.cast_character_ids?.some((id:string)=>!characters.find(c=>c.id===id)?.reference_asset_ids_json?.length);
  const referenceIds=()=>[...new Set<string>([...characterReferenceIds(),...(lastResult?.story_asset_ids??[])])];
  const charactersApproved=()=>!missingCharacterReferences()&&referenceIds().every((id:string)=>{const a=reviewAssets.find(a=>a.id===id);const review=a?.metadata_json?.consistency_review;return !!a&&(!review||review==='approved')});
  async function excludeAsset(id:string){if(continuingStory)return;continuingStory=true;opError='';try{lastResult=await post(`/videos/generate/${opId}/assets/exclude`,{asset_id:id});await refreshReviewAssets()}catch(e){opError=String(e)}finally{continuingStory=false}}
  async function continueStory(){if(continuingStory||busy)return;continuingStory=true;opError='';try{if(!await refreshReviewAssets())return;if(!charactersApproved())throw Error('Approve each character, prop and location reference before continuing.');await post(`/videos/generate/${opId}/continue`,{});busy=true;opStatus='running';stopPolling();poller=setInterval(()=>void checkOperation(),2500);await checkOperation()}catch(e){opError=String(e)}finally{continuingStory=false}}
  let preflight: any = $state(null);
  let preflightError = $state('');
  let activeProject: any = $state(null);
  let lastResult: any = $state(null);
 let reviewAssets:any[]=$state([]);let reviewLoading=$state(false);let renderingScene=$state('');
 async function refreshReviewAssets(){reviewLoading=true;try{const [assets,cast]=await Promise.all([get('/assets'),get('/characters')]);reviewAssets=assets;characters=cast;return true}catch(e){opError=String(e);return false}finally{reviewLoading=false}}
 function sceneReferencesApproved(id:string){const refs=reviewAssets.filter(a=>a.metadata_json?.scene_id===id&&a.metadata_json?.consistency_review);return refs.length>0&&refs.every(a=>a.metadata_json.consistency_review==='approved')}
 async function renderReviewedScene(id:string){renderingScene=id;try{await refreshReviewAssets();if(!sceneReferencesApproved(id))throw Error('Approve the generated references before rendering this scene.');await post(`/scenes/${id}/render`, {use_scene_cast:true});await goto('/render')}catch(e){opError=String(e)}finally{renderingScene=''}}

  let poller: ReturnType<typeof setInterval> | null = null;
  let renderPoller: ReturnType<typeof setInterval> | null = null;

  function stopPolling() {
    if (poller) clearInterval(poller);
    poller = null;
  }

  function stopRenderPolling() {
    if (renderPoller) clearInterval(renderPoller);
    renderPoller = null;
  }

  async function checkRenderJobs() {
    if(['awaiting_reference_review','awaiting_character_review'].includes(lastResult?.status)){busy=false;opStatus=lastResult.status;stopRenderPolling();await refreshReviewAssets();return}
    const ids: string[] = lastResult?.render_job_ids ?? [];
    if (!ids.length) {
      busy = false;
      opStatus = 'completed';
      stopRenderPolling();
      return;
    }
    try {
      const jobs = await Promise.all(ids.map((id) => get(`/render-jobs/${id}`)));
      opError = '';
      const states = jobs.map((item) => item.job.status);
      if (states.some((state) => state === 'failed' || state === 'reconciliation_required')) {
        opStatus = 'failed';
        opError = jobs.find((item) => item.job.status === 'failed' || item.job.status === 'reconciliation_required')?.job.error || 'A render job failed.';
        busy = false;
        stopRenderPolling();
      } else if (states.every((state) => state === 'succeeded')) {
        opStatus = 'completed';
        busy = false;
        stopRenderPolling();
      } else {
        opStatus = 'rendering';
      }
    } catch (e: any) {
      opError = `Connection interrupted. Checking render status again… (${e.message})`;
    }
  }

  async function checkOperation() {
    if (!opId) return;
    try {
      const op = await get(`/ops/${opId}`);
      opError = '';
      opStatus = op.status;
      stages = op.result_json?.stages ?? stages;
      automaticReferences=op.result_json?.character_references??automaticReferences;
      if (op.status === 'succeeded') {
        stopPolling();
        lastResult = op.result_json;
        if(lastResult?.created_character_ids?.length)void get('/characters').then(result=>characters=result).catch(()=>{});
        opStatus = 'rendering';
        await checkRenderJobs();
        if (opStatus === 'rendering') renderPoller = setInterval(() => void checkRenderJobs(), 4000);
      } else if ((op.status === 'failed' || op.status === 'reconciliation_required')) {
        stopPolling();
        lastResult=op.result_json;
        if(lastResult?.created_character_ids?.length)void get('/characters').then(result=>characters=result).catch(()=>{});
        opError = op.error || 'The generation failed.';
        busy = false;
      }
    } catch (e: any) {
      // Keep the last known state while the interval retries after reconnect.
      opError = `Connection interrupted. The operation is still running; reconnecting… (${e.message})`;
    }
  }

  let preflightSequence=0;
  async function loadPreflight(ratio: string, styleChoice: string) {
    const sequence=++preflightSequence;
    preflightError = '';
    try {
      const params = new URLSearchParams({ aspect_ratio: ratio });
      params.set('style', styleChoice); if(styleOptions.length)params.set('style_preset_id',styleChoice);
      params.set('create_characters',String(createCharacters));params.set('scene_count',String(sceneCount));params.set('target_duration',duration||'0');params.set('character_ids',selectedCharacters.join(','));
      const result=await get(`/videos/preflight?${params}`);if(sequence!==preflightSequence)return null;preflight=result;
      return preflight;
    } catch (e: any) {
      if(sequence!==preflightSequence)return null;
      preflightError = e.message;
      preflight = null;
      return null;
    }
  }

  async function restoreOperation() {
    const requested = new URL(window.location.href).searchParams.get('op_id');
    try {
      activeProject = await get('/projects/active');
      const op = requested
        ? await get(`/ops/${encodeURIComponent(requested)}`)
        : (await get(`/ops?project_id=${encodeURIComponent(activeProject.id)}&kind=video_generation&limit=1`))?.[0];
      if (!op) return;
      if (op.kind !== 'video_generation' || (op.project_id && op.project_id !== activeProject.id)) throw new Error('That generation does not belong to this project.');
      opId = op.id;
      opStatus = op.status;
      stages = op.result_json?.stages ?? [];
      automaticReferences=op.result_json?.character_references??false;
      lastResult = op.result_json;
      if (op.status === 'running') {
        busy = true;
        poller = setInterval(() => void checkOperation(), 2500);
        await checkOperation();
      } else if (op.status === 'succeeded') {
        opStatus = 'rendering';
        busy = true;
        await checkRenderJobs();
        if (opStatus === 'rendering') renderPoller = setInterval(() => void checkRenderJobs(), 4000);
      } else if ((op.status === 'failed' || op.status === 'reconciliation_required')) {
        opError = op.error || 'The generation failed.';
      }
    } catch (e: any) { if(requested)opError=`Could not restore this generation: ${e.message}`; }
  }

  onMount(() => { void loadStyles(); void restoreOperation(); get('/characters').then(result=>characters=result).catch(()=>{}); });

  $effect(() => {
    const ratio = aspectRatio;
    const styleChoice = style;
    void loadPreflight(ratio, styleChoice);
  });

  async function createVideo(e: SubmitEvent) {
    e.preventDefault();
    if (!idea.trim() || busy) return;
    busy = true;
    opError = '';
    stages = [];
    lastResult = null;
    await loadPreflight(aspectRatio, style);
    if (preflightError || preflight?.ready === false) {
      busy = false;
      return;
    }
    try {
      const response = await post('/videos/generate?background=true', {
        idea: idea.trim(),
        scene_count:sceneCount,
        style,
        ...(styleOptions.length?{style_preset_id:style}:{}),
        aspect_ratio: aspectRatio,
        language,
        conversation_mode: conversationMode,
        character_ids:selectedCharacters,
        create_characters:createCharacters,
        ...(duration ? { target_duration: Number(duration) } : {}),
        ...(instruction.trim() ? { instruction: instruction.trim() } : {})
      });
      submitted=true;
      opId = response.op_id;
      opStatus = response.status;
      automaticReferences=response.character_references??false;
      activeProject = activeProject ?? await get('/projects/active');
      poller = setInterval(() => void checkOperation(), 2500);
      await checkOperation();
    } catch (e: any) {
      busy = false;
      opError = e.message;
    }
  }

  $effect(() => {
    if (['failed','reconciliation_required'].includes(opStatus)) busy = false;
  });

  onDestroy(() => { stopPolling(); stopRenderPolling(); });
</script>

<svelte:head>{#if !embedded}<title>Guided creator · VideoFlow</title>{/if}</svelte:head>

{#snippet resultSummary()}
      {#if lastResult && opStatus !== 'running'}
      <div class="mt-6 space-y-4">
        {#if lastResult.created_character_ids?.length}<div class="space-y-1 text-sm"><p>{lastResult.created_character_ids.length} new character {lastResult.created_character_ids.length===1?'profile saved':'profiles saved'}. <a class="underline underline-offset-4" href="/library?tab=characters">Review characters</a></p><p class="text-muted-foreground">For spoken dialogue, choose approved voices in <a class="underline underline-offset-4" href="/voices">Voice &amp; dialogue</a> before rendering.</p></div>{/if}
        <div class="rounded-lg border border-border p-4 text-sm">
          {#if lastResult.status==='awaiting_character_review'}
            <h3 class="font-semibold">Review story references</h3><p class="mt-2 text-muted-foreground">Characters, props and locations are saved in Library. Existing suitable assets are reused; missing images are generated. Approve each reference before continuing with scene images.</p>
            {#if missingCharacterReferences()}<p role="alert" class="mt-3 text-sm">Some story characters need reference images. <a class="underline underline-offset-4" href="/library?tab=characters">Open Characters</a> to add or generate them, then refresh here.</p>{/if}<ul class="mt-4 space-y-3">{#each referenceIds() as id}{@const asset=reviewAssets.find(a=>a.id===id)}{@const character=characters.find(c=>c.id===asset?.character_id)}{@const decision=lastResult.story_assets?.find((item:any)=>item.asset_id===id)}<li class="flex flex-wrap items-center justify-between gap-3"><a class="inline-flex min-h-11 items-center gap-3 underline underline-offset-4" href={`/library?tab=media&asset_id=${encodeURIComponent(id)}`}>{#if asset?.file_path}<img class="size-12 rounded-md object-cover" src={mediaUrl(asset.file_path)??''} alt=""/>{/if}Review {character?.name??asset?.name??'reference image'}</a>{#if decision}<span class="text-xs text-muted-foreground" title={decision.reason}>{decision.reuse?'Reused from Library':'Generated'}{#if decision.reason}<span class="block">{decision.reason}</span>{/if}</span><Button type="button" variant="ghost" size="sm" disabled={continuingStory} onclick={()=>excludeAsset(id)}>Remove from story</Button>{/if}<span class="text-xs text-muted-foreground">{asset&&(!asset.metadata_json?.consistency_review||asset.metadata_json.consistency_review==='approved')?'Approved':'Needs review'}</span></li>{/each}</ul>
            <div class="mt-4 flex flex-wrap gap-2"><Button type="button" variant="outline" disabled={reviewLoading||continuingStory} onclick={refreshReviewAssets}>{reviewLoading?'Checking…':'Refresh approvals'}</Button><Button type="button" disabled={!charactersApproved()||reviewLoading||continuingStory} onclick={continueStory}>{continuingStory?'Continuing…':'Continue storyboarding'}</Button></div><p class="mt-2 text-xs leading-relaxed text-muted-foreground">Continuing generates scene images and may incur provider charges. Video rendering stays separate.</p>
          {:else if lastResult.status==='awaiting_reference_review'}
 <p class="font-medium">Your keyframes are ready for review.</p><p class="mt-1 text-muted-foreground">Compare the generated images with your character and asset references, then approve each reference. Video generation starts only when you choose Render scene.</p><div class="mt-3 space-y-3">{#each lastResult.scene_ids??[] as sceneID,i}<div class="flex flex-wrap items-center gap-2"><span>Scene {i+1}</span><a class="underline" href={`/library?tab=media&scene_id=${encodeURIComponent(sceneID)}`}>Review references</a><Button type="button" size="sm" disabled={reviewLoading||!!renderingScene||!sceneReferencesApproved(sceneID)} onclick={()=>renderReviewedScene(sceneID)}>Render scene {i+1}</Button><span class="text-muted-foreground">{sceneReferencesApproved(sceneID)?'References approved':'Waiting for reference approval'}</span></div>{/each}<Button type="button" variant="outline" size="sm" disabled={reviewLoading} onclick={refreshReviewAssets}>Refresh approvals</Button></div>
 {:else}
 <p class="font-medium">{['failed','reconciliation_required'].includes(opStatus) ? 'Existing work is preserved.' : opStatus === 'completed' ? 'Video complete.' : 'Render jobs submitted.'}</p>
          <p class="mt-1 text-muted-foreground">Open existing scenes to inspect completed work. Creating again starts a new pipeline and may repeat paid stages.</p>
          {/if}
          {#if !['awaiting_reference_review','awaiting_character_review'].includes(lastResult.status)}<div class="mt-3 flex flex-wrap gap-2">
            <Button type="button" variant="secondary" onclick={() => goto('/scenes')}>Open existing scenes</Button>
            {#if lastResult.render_job_ids?.length}<Button type="button" variant="outline" onclick={() => goto('/render')}>Open render jobs</Button>{/if}

          </div>{/if}
        </div>
      </div>{/if}
{/snippet}

<div class="mx-auto flex min-h-full w-full max-w-3xl flex-col {embedded?'py-2':'px-6 py-10'}">
  {#if busy || opId}
    <div class="w-full">
      <GenerationProgress automaticAssets={lastResult?.automatic_assets??stages.includes("assets")} characterReferences={automaticReferences} referenceCount={lastResult?.status==='awaiting_character_review'?referenceIds().length:lastResult?.character_reference_ids?.length??null} reviewHref={lastResult?.status==='awaiting_character_review'?(referenceIds()[0]?`/library?tab=media&asset_id=${encodeURIComponent(referenceIds()[0])}`:'/library?tab=characters'):lastResult?.scene_ids?.[0]?`/library?tab=media&scene_id=${encodeURIComponent(lastResult.scene_ids[0])}`:undefined} status={opStatus} {stages} error={opError} onopen={lastResult?undefined:() => goto('/?view=story')} />
      {#if !busy}{@render resultSummary()}<Button variant="ghost" class="mt-4" onclick={()=>{opId='';opStatus='';lastResult=null;stages=[];submitted=false;automaticReferences=false;}}>{['failed','reconciliation_required'].includes(opStatus)?'Edit brief & try again':'Create another story'}</Button>{/if}
    </div>
  {:else}
    <div class="mb-6">
      <div class="mb-3 flex size-10 items-center justify-center rounded-xl bg-primary/10 text-primary">
        <Clapperboard class="size-5" />
      </div>
      {#if !embedded}<h1 class="text-2xl font-semibold tracking-tight">Create story</h1>{/if}
      <p class="mt-2 max-w-xl text-muted-foreground">
        Provider calls may incur charges.
      </p>
    </div>

    <form class="space-y-5" onsubmit={createVideo}>
      <div class="rounded-xl border border-border bg-card p-5 shadow-sm">
        <label class="mb-2 block text-sm font-medium" for="idea">What video do you want to make?</label>
        <textarea
          id="idea"
          bind:value={idea}
          required
          placeholder="A warm 2D story about two siblings learning why families celebrate the Mid-Autumn Festival…"
          class="min-h-40 w-full resize-y rounded-lg border border-input bg-background px-3 py-2.5 text-sm outline-none focus:ring-2 focus:ring-ring"
        ></textarea>
      </div>

      <label class="grid gap-1 text-sm"><span class="font-medium">Number of scene clips</span><input type="number" min="1" max="20" bind:value={sceneCount} class="rounded-lg border bg-background px-3 py-2"/><span class="text-xs text-muted-foreground">Each scene generates its own image and video. Start with one clip to test your setup.</span></label>
      <fieldset class="space-y-2 rounded-lg border p-3"><legend class="text-sm font-medium">Characters in this story</legend>
       <label class="flex items-start gap-2 text-sm"><input class="mt-1" type="checkbox" bind:checked={createCharacters}/><span>Create missing characters from the story</span></label>
       <p class="text-xs leading-relaxed text-muted-foreground">{#if preflight?.character_references===false}New character profiles are saved when needed. Add reference images in Library to guide their appearance. Voice setup is separate.{:else}New character profiles are saved when needed. Reference sheets are generated automatically for story characters without images. Essential props and locations are matched against Library; missing images are generated. These image calls may incur provider charges. Review them before continuing to scene images. Voice setup is separate.{/if}</p>
       {#if characters.length}
       {#each characters as character(character.id)}<label class="flex items-center gap-2 text-sm"><input type="checkbox" bind:group={selectedCharacters} value={character.id}/>{character.name}</label>{/each}
       <p class="text-xs text-muted-foreground">Select existing characters to reuse them. Their saved identities stay unchanged.</p>
       {/if}
      </fieldset>
      {#if styleOptions.find(p=>p.id===style)}{@const selected=styleOptions.find(p=>p.id===style)}<details class="text-sm"><summary class="cursor-pointer text-muted-foreground">About {selected.name}</summary><p class="mt-2">{selected.description}</p><dl class="mt-2 space-y-2">{#each Object.entries(selected.guidance??{}) as [stage,text]}<div><dt class="font-medium capitalize">{stage}</dt><dd class="text-muted-foreground">{String(text)}</dd></div>{/each}</dl><a href="/settings#presets" class="mt-2 inline-block underline">Manage preset packs</a></details>{/if}
      <div class="grid gap-3 sm:grid-cols-2">
        <label class="grid gap-1.5 text-sm">
          <span class="font-medium">Visual style</span>
          <select bind:value={style} onchange={()=>styleTouched=true} class="rounded-lg border border-input bg-background px-3 py-2 text-sm">
            {#if preflight?.has_project_style||defaultStyle==='project-style'}<option value="project-style">Project style</option>{/if}
            {#if styleOptions.length}{#each styleOptions as option(option.id)}<option value={option.id}>{option.name}{option.id.startsWith('community:')?' · Community':''}</option>{/each}
            {:else}<option value="2d-picture-book">2D picture book</option><option value="cinematic">Cinematic</option>{/if}
          </select>
          {#if presetError}<span role="status" class="text-xs text-muted-foreground">{presetError}</span>{/if}

        </label>
        <label class="grid gap-1.5 text-sm">
          <span class="font-medium">Format</span>
          <select bind:value={aspectRatio} class="rounded-lg border border-input bg-background px-3 py-2 text-sm">
            <option value="9:16">Vertical · 9:16</option>
            <option value="16:9">Landscape · 16:9</option>
            <option value="1:1">Square · 1:1</option>
            <option value="3:4">Portrait · 3:4</option>
            <option value="4:3">Landscape · 4:3</option>
            <option value="21:9">Ultrawide · 21:9</option>
          </select>
        </label>
        <label class="grid gap-1.5 text-sm">
          <span class="font-medium">Language</span>
          <select bind:value={language} class="rounded-lg border border-input bg-background px-3 py-2 text-sm">
            <option>English</option>
            <option>Chinese</option>
            <option>Malay</option>
          </select>
        </label>
        <div class="grid gap-1.5 text-sm">
          <span class="font-medium">Dialogue</span>
          <span class="text-muted-foreground">Conversational dialogue is applied to generated scenes.</span>
        </div>
      </div>

      <div class="rounded-lg border border-border bg-card p-4 text-sm">
        <h2 class="font-medium">Before you create</h2>
        <p class="mt-2 text-muted-foreground">
          Request brief: {language} dialogue, {aspectRatio} format, {preflight?.effective_style ?? style} visual style.
          {#if instruction.trim()} Direction: {instruction.trim()}{:else} No extra direction added.{/if}
        </p>
        {#if preflightError}
          <p class="mt-2 text-destructive">Could not check provider setup: {preflightError}</p>
        {:else if !preflight}
          <p class="mt-2 text-muted-foreground">Checking configured models and supported video settings…</p>
        {:else}
          <p class="mt-2 text-muted-foreground">{#if preflight?.character_references===false}This request creates a story, shots and storyboard images. Review references before rendering each scene clip.{:else}This request creates a story, shots and any missing character, prop and location reference images. Review story references, then continue to storyboard images. Review and approve scene references before rendering each clip.{/if}</p>
          {#if preflight.native_images}<p class="mt-2 text-muted-foreground">Images use Codex first. The configured image API is used only when native generation is unavailable or the request is unsupported. Native errors pause for review.</p>{/if}
          <details class="mt-3">
            <summary class="cursor-pointer font-medium">Models &amp; clip limits</summary>
            <p class="mt-2 break-words text-muted-foreground">Images: {#if preflight.native_images}Codex built-in · API fallback: {/if}{preflight.effective_routes.image_provider} / {preflight.effective_routes.image_model}. Video: {preflight.effective_routes.video_provider} / {preflight.effective_routes.video_model}.</p>
            <p class="mt-1 text-muted-foreground">{#if preflight.video_durations?.length}Supported clip durations: {preflight.video_durations.join(', ')} seconds.{:else}Clip durations are unavailable until the video provider is configured.{/if}</p>
          </details>
          {#each preflight.warnings ?? [] as warning}
            <p class="mt-2 text-amber-700 dark:text-amber-300">Unverified route: {warning.reason}</p>
          {/each}
          {#if preflight.missing.length}
            <ul class="mt-2 list-disc space-y-1 pl-5 text-destructive">
              {#each preflight.missing as item}
                <li>{item.reason} <a class="underline" href={item.setting}>{item.setting?.startsWith('/director') ? 'Open AI sessions' : 'Open settings'}</a></li>
              {/each}
            </ul>
          {:else}
            <p class="mt-2 text-emerald-700 dark:text-emerald-300">Configured routes and selected format are ready. This check does not contact providers.</p>
          {/if}
        {/if}
      </div>

      <details class="rounded-lg border border-border px-4 py-3 text-sm">
        <summary class="cursor-pointer font-medium">More options</summary>
        <div class="mt-3 grid gap-3 sm:grid-cols-2">
          <label class="grid gap-1.5">
            <span class="text-muted-foreground">Target duration in seconds</span>
            <input type="number" min={sceneCount} max="300" bind:value={duration} placeholder="Automatic" class="rounded-lg border border-input bg-background px-3 py-2" />
          </label>
          <label class="grid gap-1.5 sm:col-span-2">
            <span class="text-muted-foreground">Direction for the AI</span>
            <input bind:value={instruction} placeholder="Warm, clear dialogue with short lines…" class="rounded-lg border border-input bg-background px-3 py-2" />
          </label>
        </div>
      </details>

      {#if opError}
        <div role="alert" class="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">{#if opStatus==='failed'}<p class="mb-1 font-medium">Last attempt failed</p>{/if}<p>{opError}</p></div>
      {/if}

      {@render resultSummary()}

      <Button type="submit" size="lg" class="w-full sm:w-auto" disabled={!idea.trim() || !preflight || preflight.missing.length > 0 || !!preflightError}>
        <Sparkles class="mr-2 size-4" />Create video
      </Button>
    </form>
  {/if}
</div>
