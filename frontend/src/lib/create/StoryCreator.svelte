<script lang="ts">
  import { errorMessage } from '$lib/errors';
  import CreativeBriefEditor from '$lib/brief/CreativeBriefEditor.svelte';
  import { normalizeBrief, confirmedBriefRequest, missingBriefFields } from '$lib/brief/brief.mjs';
  let creativeBrief = $state(normalizeBrief()),
    briefConfirmed = $state(false);
  function syncBrief(brief: ReturnType<typeof normalizeBrief>) {
    creativeBrief = brief;
    briefConfirmed = false;
    styleTouched = true;
    if (brief.style_preset_id) style = brief.style_preset_id;
    if (brief.aspect_ratio) aspectRatio = brief.aspect_ratio;
    if (brief.scene_count) sceneCount = brief.scene_count;
    if (brief.language) language = brief.language;
    duration = brief.target_duration ? String(brief.target_duration) : '';
    instruction = brief.instruction;
  }

  let { embedded = false }: { embedded?: boolean } = $props();
  import { onDestroy, onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { protectDraft } from '$lib/protect-draft';
  import { ArrowRight, Clapperboard, Sparkles } from '@lucide/svelte';
  import { Button } from '$lib/components/ui/button';
  import GenerationProgress from '$lib/create/GenerationProgress.svelte';
  import { get, post, mediaUrl } from '$lib/api';
  import type { Op } from '$lib/ops';
  import type {
    Asset,
    Character,
    OpStarted,
    Project,
    RenderJobDetail,
    StoryGenerationResult,
    StyleCatalog,
    StylePreset,
    VideoPreflight
  } from '$lib/api-types';

  let idea = $state('');
  let submitted = $state(false);
  const draftDirty = () =>
    !submitted &&
    !!(
      idea.trim() ||
      instruction.trim() ||
      selectedCharacters.length ||
      style !== defaultStyle ||
      aspectRatio !== '9:16' ||
      sceneCount !== 1 ||
      language !== 'English' ||
      duration ||
      !createCharacters
    );
  export function canClose() {
    return !draftDirty() || window.confirm('Discard this story draft?');
  }
  protectDraft(draftDirty, 'Discard this story draft and leave?');
  let style = $state('2d-picture-book');
  let defaultStyle = $state('2d-picture-book');
  let styleTouched = false;
  let styleOptions: StylePreset[] = $state([]);
  let presetError = $state('');
  async function loadStyles() {
    try {
      const result = await get<StyleCatalog | null>('/style-presets');
      if (!Array.isArray(result?.presets)) return;
      styleOptions = result.presets;
      if (result.has_project_style && !styleTouched && !submitted) {
        defaultStyle = 'project-style';
        style = 'project-style';
      }
      presetError = '';
    } catch (e) {
      presetError = 'Could not load extra styles. Reopen this form to retry.';
    }
  }

  let aspectRatio = $state('9:16');
  let duration = $state('');
  let sceneCount = $state(1);
  let language = $state('English');
  const conversationMode = 'dialogue';
  let characters: Character[] = $state([]);
  let selectedCharacters: string[] = $state([]);
  let createCharacters = $state(true);
  let instruction = $state('');
  let busy = $state(false);
  let opId = $state('');
  let opStatus = $state('');
  let opError = $state('');
  let stages = $state<string[]>([]);
  let automaticReferences = $state(false),
    continuingStory = $state(false);
  const characterReferenceIds = () =>
    lastResult?.cast_character_ids?.length
      ? lastResult.cast_character_ids.flatMap(
          id => characters.find(c => c.id === id)?.reference_asset_ids_json ?? []
        )
      : (lastResult?.character_reference_ids ?? []);
  const missingCharacterReferences = () =>
    lastResult?.cast_character_ids?.some(
      id => !characters.find(c => c.id === id)?.reference_asset_ids_json?.length
    );
  const referenceIds = () => [...new Set<string>([...characterReferenceIds(), ...(lastResult?.story_asset_ids ?? [])])];
  const charactersApproved = () =>
    !missingCharacterReferences() &&
    referenceIds().every(id => {
      const a = reviewAssets.find(a => a.id === id);
      const review = a?.metadata_json?.consistency_review;
      return !!a && review !== 'rejected';
    });
  async function excludeAsset(id: string) {
    if (continuingStory) return;
    continuingStory = true;
    opError = '';
    try {
      lastResult = await post<StoryGenerationResult>(`/videos/generate/${opId}/assets/exclude`, { asset_id: id });
      await refreshReviewAssets();
    } catch (e) {
      opError = errorMessage(e);
    } finally {
      continuingStory = false;
    }
  }
  async function continueStory() {
    if (continuingStory || busy) return;
    continuingStory = true;
    opError = '';
    try {
      if (!(await refreshReviewAssets())) return;
      if (!charactersApproved()) throw Error('Replace missing or excluded references before continuing.');
      await post(`/videos/generate/${opId}/continue`, {});
      busy = true;
      opStatus = 'running';
      stopPolling();
      poller = setInterval(() => void checkOperation(), 2500);
      await checkOperation();
    } catch (e) {
      opError = errorMessage(e);
    } finally {
      continuingStory = false;
    }
  }
  let preflight = $state<VideoPreflight | null>(null);
  let preflightError = $state('');
  let activeProject = $state<Project | null>(null);
  let lastResult = $state<StoryGenerationResult | null | undefined>(null);
  let reviewAssets: Asset[] = $state([]);
  let reviewLoading = $state(false);
  async function refreshReviewAssets() {
    reviewLoading = true;
    try {
      const [assets, cast] = await Promise.all([get<Asset[]>('/assets'), get<Character[]>('/characters')]);
      reviewAssets = assets;
      characters = cast;
      return true;
    } catch (e) {
      opError = errorMessage(e);
      return false;
    } finally {
      reviewLoading = false;
    }
  }

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

  let readingJobs = false;
  let readingOperation = false;

  async function checkRenderJobs() {
    if (
      lastResult &&
      ['awaiting_reference_review', 'awaiting_character_review', 'ready_to_render'].includes(lastResult.status)
    ) {
      busy = false;
      opStatus = lastResult.status;
      stopRenderPolling();
      await refreshReviewAssets();
      return;
    }
    const ids = lastResult?.render_job_ids ?? [];
    if (!ids.length) {
      busy = false;
      opStatus = 'completed';
      stopRenderPolling();
      return;
    }
    if (readingJobs) return;
    readingJobs = true;
    try {
      const jobs = await Promise.all(ids.map(id => get<RenderJobDetail>(`/render-jobs/${id}`)));
      opError = '';
      const states = jobs.map(item => item.job.status);
      if (states.some(state => state === 'failed' || state === 'reconciliation_required')) {
        opStatus = 'failed';
        opError =
          jobs.find(item => item.job.status === 'failed' || item.job.status === 'reconciliation_required')?.job.error ||
          'A render job failed.';
        busy = false;
        stopRenderPolling();
      } else if (states.every(state => state === 'succeeded')) {
        opStatus = 'completed';
        busy = false;
        stopRenderPolling();
      } else {
        opStatus = 'rendering';
      }
    } catch (e) {
      opError = 'Reconnecting to check render status…';
    } finally {
      readingJobs = false;
    }
  }

  async function checkOperation() {
    if (!opId || readingOperation) return;
    readingOperation = true;
    try {
      const op = await get<Op<StoryGenerationResult>>(`/ops/${opId}`);
      opError = '';
      opStatus = op.status;
      stages = op.result_json?.stages ?? stages;
      automaticReferences = op.result_json?.character_references ?? automaticReferences;
      if (op.status === 'succeeded') {
        stopPolling();
        lastResult = op.result_json;
        if (lastResult?.created_character_ids?.length)
          void get<Character[]>('/characters')
            .then(result => (characters = result))
            .catch(() => {});
        opStatus = 'rendering';
        await checkRenderJobs();
        if (opStatus === 'rendering') renderPoller = setInterval(() => void checkRenderJobs(), 4000);
      } else if (op.status === 'failed' || op.status === 'reconciliation_required') {
        stopPolling();
        lastResult = op.result_json;
        if (lastResult?.created_character_ids?.length)
          void get<Character[]>('/characters')
            .then(result => (characters = result))
            .catch(() => {});
        opError = op.error || 'The generation failed.';
        busy = false;
      }
    } catch (e) {
      // Keep the last known state while the interval retries after reconnect.
      opError = 'Reconnecting to check story progress…';
    } finally {
      readingOperation = false;
    }
  }

  // The dialog needs an AI session to plan the story. Start one here instead of sending
  // people away (leaving would discard their draft).
  let startingSession = $state(false),
    sessionError = $state('');
  async function startSession() {
    startingSession = true;
    sessionError = '';
    try {
      const ensured = await post<{ session: unknown; switched_from?: string } | null>('/director/sessions/ensure', {});
      if (!ensured?.session) {
        sessionError = 'Connect Claude Code or Codex first (Settings → AI setup).';
        return;
      }
      await loadPreflight(aspectRatio, style);
    } catch (e) {
      sessionError = errorMessage(e, String(e));
    } finally {
      startingSession = false;
    }
  }
  let preflightSequence = 0;
  async function loadPreflight(ratio: string, styleChoice: string) {
    const sequence = ++preflightSequence;
    preflightError = '';
    try {
      const params = new URLSearchParams({ aspect_ratio: ratio });
      params.set('style', styleChoice);
      if (styleOptions.length) params.set('style_preset_id', styleChoice);
      params.set('create_characters', String(createCharacters));
      params.set('scene_count', String(sceneCount));
      params.set('target_duration', duration || '0');
      params.set('character_ids', selectedCharacters.join(','));
      const result = await get<VideoPreflight>(`/videos/preflight?${params}`);
      if (sequence !== preflightSequence) return null;
      preflight = result;
      return preflight;
    } catch (e) {
      if (sequence !== preflightSequence) return null;
      preflightError = errorMessage(e);
      preflight = null;
      return null;
    }
  }

  // An old failed attempt is a footnote, not the whole dialog: people opening Create story
  // usually want a new story. They can still open the failed attempt from the note.
  let earlierFailure = $state<{ id: string; error: string } | null>(null);
  async function restoreOperation(explicit = '') {
    const requested = explicit || new URL(window.location.href).searchParams.get('op_id');
    try {
      const project = await get<Project>('/projects/active');
      activeProject = project;
      const op = requested
        ? await get<Op<StoryGenerationResult>>(`/ops/${encodeURIComponent(requested)}`)
        : (
            await get<Op<StoryGenerationResult>[] | null>(
              `/ops?project_id=${encodeURIComponent(project.id)}&kind=video_generation&limit=1`
            )
          )?.[0];
      if (!op) return;
      if (op.kind !== 'video_generation' || (op.project_id && op.project_id !== project.id))
        throw new Error('That generation does not belong to this project.');
      if (!requested && (op.status === 'failed' || op.status === 'reconciliation_required')) {
        earlierFailure = { id: op.id, error: op.error || '' };
        return;
      }
      earlierFailure = null;
      opId = op.id;
      opStatus = op.status;
      stages = op.result_json?.stages ?? [];
      automaticReferences = op.result_json?.character_references ?? false;
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
      } else if (op.status === 'failed' || op.status === 'reconciliation_required') {
        opError = op.error || 'The generation failed.';
      }
    } catch (e) {
      if (requested) opError = `Could not restore this generation: ${errorMessage(e)}`;
    }
  }

  onMount(() => {
    void loadStyles();
    void restoreOperation();
    get<Character[]>('/characters')
      .then(result => (characters = result))
      .catch(() => {});
  });

  $effect(() => {
    const ratio = aspectRatio;
    const styleChoice = style;
    void loadPreflight(ratio, styleChoice);
  });

  async function createVideo(e: SubmitEvent) {
    e.preventDefault();
    if (!idea.trim() || busy) return;
    if (!briefConfirmed || missingBriefFields({ ...creativeBrief, idea: idea.trim() }).length) {
      opError = 'Complete and confirm the creative brief before creating';
      return;
    }
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
      const response = await post<OpStarted & { status: string; character_references?: boolean }>(
        '/videos/generate?background=true',
        {
        ...confirmedBriefRequest({ ...creativeBrief, idea: idea.trim() }, briefConfirmed),
        review_before_render: true,
        character_ids: selectedCharacters,
        create_characters: createCharacters
      });
      submitted = true;
      opId = response.op_id;
      opStatus = response.status;
      automaticReferences = response.character_references ?? false;
      activeProject = activeProject ?? (await get<Project>('/projects/active'));
      poller = setInterval(() => void checkOperation(), 2500);
      await checkOperation();
    } catch (e) {
      busy = false;
      opError = errorMessage(e);
    }
  }

  $effect(() => {
    if (['failed', 'reconciliation_required'].includes(opStatus)) busy = false;
  });

  onDestroy(() => {
    stopPolling();
    stopRenderPolling();
  });
</script>

<svelte:head>{#if !embedded}<title>Guided creator · VideoFlow</title>{/if}</svelte:head>

{#snippet resultSummary()}
      {#if lastResult && opStatus !== 'running'}
      <div class="mt-6 space-y-4">
        {#if lastResult.created_character_ids?.length}<div
          class="space-y-1 text-sm"><p>{lastResult.created_character_ids.length} new character {lastResult.created_character_ids.length===1?'profile saved':'profiles saved'}. <a
            class="underline underline-offset-4"
            href="/library?tab=characters">View characters</a></p><p
            class="text-muted-foreground">To have narration spoken in a character's own voice, choose it in <a
            class="underline underline-offset-4" href="/voices">Voices</a>.</p></div>{/if}
        <div class="rounded-lg border border-border p-4 text-sm">
          {#if lastResult.status==='awaiting_character_review'}
            <h3 class="font-semibold">Story references</h3><p
              class="mt-2 text-muted-foreground">Characters, props and locations are saved in Library. Existing suitable assets are reused; missing images are generated. Images are ready to use without approval.</p>
            {#if missingCharacterReferences()}<p role="alert" class="mt-3 text-sm">Some story characters need reference images. <a
              class="underline underline-offset-4"
              href="/library?tab=characters">Open Characters</a> to add or generate them, then refresh here.</p>{/if}<ul
                class="mt-4 space-y-3">{#each referenceIds() as id}{@const asset=reviewAssets.find(a=>a.id===id)}{@const character=characters.find(c=>c.id===asset?.character_id)}{@const decision=lastResult.story_assets?.find(item=>item.asset_id===id)}<li
                class="flex flex-wrap items-center justify-between gap-3"><a
              class="inline-flex min-h-11 items-center gap-3 underline underline-offset-4"
              href={`/library?tab=media&asset_id=${encodeURIComponent(id)}`}>{#if asset?.file_path}<img
              class="size-12 rounded-md object-cover"
              src={mediaUrl(asset.file_path)??''}
              alt=""/>{/if}View {character?.name??asset?.name??'reference image'}</a>{#if decision}<span
              class="text-xs text-muted-foreground"
              title={decision.reason}>{decision.reuse?'Reused from Library':'Generated'}{#if decision.reason}<span
                class="block">{decision.reason}</span>{/if}</span><Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={continuingStory}
              onclick={()=>excludeAsset(id)}>Remove from story</Button>{/if}<span
                class="text-xs text-muted-foreground">{asset&&asset.metadata_json?.consistency_review!=='rejected'?'Ready':'Excluded'}</span></li>{/each}</ul>
            <div class="mt-4 flex flex-wrap gap-2"><Button
              type="button"
              variant="outline"
              disabled={reviewLoading||continuingStory}
              onclick={refreshReviewAssets}>{reviewLoading?'Checking…':'Refresh images'}</Button><Button
              type="button"
              disabled={!charactersApproved()||reviewLoading||continuingStory}
              onclick={continueStory}>{continuingStory?'Continuing…':'Continue storyboarding'}</Button></div><p
                class="mt-2 text-xs leading-relaxed text-muted-foreground">Continuing makes a storyboard picture for each scene that needs one, which may incur provider charges. Video rendering stays separate.</p>
          {:else if ['awaiting_reference_review','ready_to_render'].includes(lastResult.status)}
 {@const firstScene=lastResult.scene_ids?.[0]}
 <p class="font-medium">{lastResult.storyboards_made===0?'Your story is ready for video.':'Your story and images are ready.'}</p><p
   class="mt-1 text-muted-foreground">{lastResult.storyboards_made === 0
       ? "No storyboard pictures were needed: each scene's clips use its connected pictures. "
       : ''}Review each scene in Story, adjust its images, then make its clip there. Video calls may incur charges.</p>
 <div class="mt-3 flex flex-wrap items-center gap-2"><Button
   href={firstScene?`/?view=story&scene_id=${encodeURIComponent(firstScene)}`:'/?view=story'}>Review scenes and make clips<ArrowRight
   class="size-4" /></Button>{#if firstScene}<Button
   variant="outline"
   href={`/library?tab=media&scene_id=${encodeURIComponent(firstScene)}`}>View images</Button>{/if}{#if lastResult.script_id}<Button
   variant="ghost"
   href={`/?view=edit&script_id=${encodeURIComponent(lastResult.script_id)}`}>Assemble story & add music</Button>{/if}</div>
 {:else}
 <p class="font-medium">{['failed', 'reconciliation_required'].includes(opStatus)
     ? 'Existing work is preserved.'
     : opStatus === 'completed'
       ? 'Video complete.'
       : 'Render jobs submitted.'}</p>
          <p class="mt-1 text-muted-foreground">Open existing scenes to inspect completed work. Creating again starts a new pipeline and may repeat paid stages.</p>
          {/if}
          {#if !['awaiting_reference_review','awaiting_character_review','ready_to_render'].includes(lastResult.status)}<div class="mt-3 flex flex-wrap gap-2">
            <Button type="button" variant="secondary" onclick={() => goto('/?view=story')}>Open existing scenes</Button>
            {#if lastResult.render_job_ids?.length}<Button type="button" variant="outline" onclick={() => goto('/render')}>Open render jobs</Button>{/if}

          </div>{/if}
        </div>
      </div>{/if}
{/snippet}

<div class="mx-auto flex min-h-full w-full max-w-3xl flex-col {embedded?'py-2':'px-6 py-10'}">
  {#if busy || opId}
    <div class="w-full">
      <GenerationProgress
        automaticAssets={lastResult?.automatic_assets??stages.includes("assets")}
        characterReferences={automaticReferences}
        referenceCount={lastResult?.status === 'awaiting_character_review'
          ? referenceIds().length
          : (lastResult?.character_reference_ids?.length ?? null)}
        reviewHref={lastResult?.status === 'awaiting_character_review'
          ? referenceIds()[0]
            ? `/library?tab=media&asset_id=${encodeURIComponent(referenceIds()[0])}`
            : '/library?tab=characters'
          : undefined}
        storyboardsMade={lastResult?.storyboards_made??null}
        status={opStatus}
        {stages}
        error={opError}
        onopen={lastResult?undefined:() => goto('/?view=story')} />
      {#if !busy}{@render resultSummary()}<Button
        variant="ghost"
        class="mt-4"
        onclick={() => {
          opId = '';
          opStatus = '';
          lastResult = null;
          stages = [];
          submitted = false;
          automaticReferences = false;
        }}>{['failed','reconciliation_required'].includes(opStatus)?'Edit brief & try again':'Create another story'}</Button>{/if}
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

    {#if earlierFailure}<div
      role="note"
      class="mb-4 flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border bg-muted/40 p-3 text-sm"><span>Your last story attempt didn't finish. Anything it made is kept.</span><span
        class="flex gap-2"><Button
      type="button"
      size="sm"
      variant="outline"
      onclick={()=>{const id=earlierFailure!.id;void restoreOperation(id)}}>See what happened</Button><Button
      type="button"
      size="sm"
      variant="ghost"
      onclick={()=>earlierFailure=null}>Dismiss</Button></span></div>{/if}
    <form class="space-y-5" onsubmit={createVideo}>
      <div class="rounded-xl border border-border bg-card p-5 shadow-sm">
        <label class="mb-2 block text-sm font-medium" for="idea">What video do you want to make?</label>
        <textarea
          id="idea"
          value={idea}
          oninput={e=>{idea=e.currentTarget.value;creativeBrief={...creativeBrief,idea};briefConfirmed=false}}
          required
          placeholder="A warm 2D story about two siblings learning why families celebrate the Mid-Autumn Festival…"
          class="min-h-40 w-full resize-y rounded-lg border border-input bg-background px-3 py-2.5 text-sm outline-none focus:ring-2 focus:ring-ring"
        ></textarea>
      </div>


      <fieldset class="space-y-2 rounded-lg border p-3"><legend class="text-sm font-medium">Characters in this story</legend>
       <label class="flex items-start gap-2 text-sm"><input
         class="mt-1"
         type="checkbox"
         bind:checked={createCharacters}
         onchange={()=>briefConfirmed=false}/><span>Create missing characters from the story</span></label>
       <p class="text-xs leading-relaxed text-muted-foreground">{#if preflight?.character_references===false}New character profiles are saved when needed. Add reference images in Library to guide their appearance. Voice setup is separate.{:else}New character profiles are saved when needed. Reference sheets are generated automatically for story characters without images. Essential props and locations are matched against Library; missing images are generated. These image calls may incur provider charges. Scene images follow automatically. Voice setup is separate.{/if}</p>
       {#if characters.length}
       {#each characters as character(character.id)}<label class="flex items-center gap-2 text-sm"><input
         type="checkbox"
         bind:group={selectedCharacters}
         onchange={()=>briefConfirmed=false}
         value={character.id}/>{character.name}</label>{/each}
       <p class="text-xs text-muted-foreground">Select existing characters to reuse them. Their saved identities stay unchanged.</p>
       {/if}
      </fieldset>
      {#if styleOptions.find(p=>p.id===style)}{@const selected=styleOptions.find(p=>p.id===style)!}<details
        class="text-sm"><summary
        class="cursor-pointer text-muted-foreground">About {selected.name}</summary><p
        class="mt-2">{selected.description}</p><dl
        class="mt-2 space-y-2">{#each Object.entries(selected.guidance??{}) as [stage,text]}<div><dt
        class="font-medium capitalize">{stage}</dt><dd class="text-muted-foreground">{String(text)}</dd></div>{/each}</dl><a
        href="/settings#presets"
        class="mt-2 inline-block underline">Manage preset packs</a></details>{/if}
      <CreativeBriefEditor bind:value={creativeBrief} bind:confirmed={briefConfirmed} disabled={busy} onchange={syncBrief}/>

      <div class="rounded-lg border border-border bg-card p-4 text-sm">
        <h2 class="font-medium">Generation setup</h2>

        {#if preflightError}
          <p class="mt-2 text-destructive">Could not check provider setup: {preflightError}</p>
          <button type="button" class="underline" onclick={() => loadPreflight(aspectRatio, style)}>Check again</button>
        {:else if !preflight}
          <p class="mt-2 text-muted-foreground">Checking configured models and supported video settings…</p>
        {:else}
          <p class="mt-2 text-muted-foreground">{#if preflight.native_images}Codex creates the images, with {preflight.effective_routes.image_provider} selected for fallback.{:else}Your writing session plans the story. {preflight.effective_routes.image_provider} handles image generation.{/if} Render each scene when ready.</p>
          <details class="mt-3">
            <summary class="cursor-pointer font-medium">Models &amp; clip limits</summary>
            <p class="mt-2 break-words text-muted-foreground">Images: {#if preflight.native_images}Codex built-in · API fallback: {/if}{preflight.effective_routes.image_provider} / {preflight.effective_routes.image_model}. Video: {preflight.effective_routes.video_provider} / {preflight.effective_routes.video_model}.</p>
            <p class="mt-1 text-muted-foreground">{#if preflight.video_durations?.length}Supported clip durations: {preflight.video_durations.join(', ')} seconds.{:else}Clip durations are unavailable until the video provider is configured.{/if}</p>
          </details>
          {#each preflight.warnings ?? [] as warning}
            <p class="mt-2 text-amber-700 dark:text-amber-300">Unverified route: {warning.reason}</p>
          {/each}
          {#if sessionError}<p role="alert" class="mt-2 text-destructive">{sessionError}</p>{/if}
          {#if preflight.missing.length}
            <ul class="mt-2 list-disc space-y-1 pl-5 text-destructive">
              {#each preflight.missing as item}
                <li>{item.reason} {#if item.setting?.startsWith('/director')}<Button
                  size="sm"
                  variant="outline"
                  class="ml-1"
                  disabled={startingSession}
                  onclick={startSession}>{startingSession?'Starting…':'Start a session now'}</Button> <a
                    class="underline" href="/settings#ai">or open AI setup</a>{:else}<a class="underline" href={item.setting}>Open settings</a>{/if}</li>
              {/each}
            </ul>
          {:else}
            <p class="mt-2 text-emerald-700 dark:text-emerald-300">Configuration checked. Provider sign-in and quota are checked during generation.</p>
          {/if}
        {/if}
      </div>

      <details class="rounded-lg border border-border px-4 py-3 text-sm">
        <summary class="cursor-pointer font-medium">More options</summary>
        <div class="mt-3 grid gap-3 sm:grid-cols-2">
          <label class="grid gap-1.5 sm:col-span-2">
            <span class="text-muted-foreground">Direction for the AI</span>
            <input
              value={instruction}
              oninput={e => {
                instruction = e.currentTarget.value;
                creativeBrief = { ...creativeBrief, instruction };
                briefConfirmed = false;
              }}
              placeholder="Warm, clear dialogue with short lines…"
              class="rounded-lg border border-input bg-background px-3 py-2" />
          </label>
        </div>
      </details>

      {#if opError}
        <div
          role="alert"
          class="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">{#if opStatus==='failed'}<p
            class="mb-1 font-medium">Last attempt failed</p>{/if}<p>{opError}</p></div>
      {/if}

      {@render resultSummary()}

      <Button
        type="submit"
        size="lg"
        class="w-full sm:w-auto"
        disabled={!briefConfirmed ||
        missingBriefFields({ ...creativeBrief, idea: idea.trim() }).length > 0 ||
        !idea.trim() ||
        !preflight ||
        preflight.missing.length > 0 ||
        !!preflightError}>
        <Sparkles class="mr-2 size-4" />Create video
      </Button>
    </form>
  {/if}
</div>
