<script lang="ts">
  import { errorMessage } from '$lib/errors';
  import { editHref } from '$lib/editor/links';
  import CreativeBriefEditor from '$lib/brief/CreativeBriefEditor.svelte';
  import { normalizeBrief, missingBriefFields, confirmedBriefRequest } from '$lib/brief/brief.mjs';
  let newRenderVersion = $state(false);
  const operationScope = new AbortController();
  onDestroy(() => operationScope.abort());
  import ProductionControls from '$lib/production/ProductionControls.svelte';
  let renderPlanId = $state(''),
    renderQuote = $state({ calls: 0, seconds: 0, storyboards: 0 });
  import { onDestroy, untrack } from 'svelte';
  import { del as apiDel, get, patch as apiPatch, post as apiPost, put as apiPut } from '$lib/api';
  import ScriptFitReview from '$lib/scenes/ScriptFitReview.svelte';
  import { normalizeChatIntent } from './intent.mjs';
  import { runBackgroundOp, type Op } from '$lib/ops';
  import type {
    AssetPlan,
    CharacterSheetPlan,
    PlannedAsset,
    ProjectStylePreset,
    Scene,
    ScriptFit,
    Shot,
    StyleFields
  } from '$lib/api-types';
  import type { ChatIntent, ChatOptions, ChatProjectState, RunContext } from './types';

  import { Badge } from '$lib/components/ui/badge';
  import { Button } from '$lib/components/ui/button';
  import * as Item from '$lib/components/ui/item';
  import { Textarea } from '$lib/components/ui/textarea';
  import Lightbulb from '@lucide/svelte/icons/lightbulb';
  import LoaderCircle from '@lucide/svelte/icons/loader-circle';
  import Play from '@lucide/svelte/icons/play';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import { toast } from 'svelte-sonner';

  interface IdeaOption {
    title: string;
    premise: string;
    hook: string;
    why_it_works: string;
  }

  interface IdeaOptions {
    options: IdeaOption[];
    recommended_index: number;
    reasoning: string;
  }

  /** Settles the chat's in-conversation progress row for a background op. */
  interface OpHandle {
    done: (op: Op) => unknown;
    fail: (op: Op) => unknown;
  }

  let {
    intent: rawIntent,
    options,
    projectState = null,
    onran,
    onfocus,
    onop,
    onsuggest,
    onbriefchange,
    onreference,
    auto = false,
    full = false,
    onautostart
  }: {
    /** Auto mode: run this step right away when it is free and ready. */
    auto?: boolean;
    /** Full auto: also start the paid steps in AUTO_FULL by themselves. */
    full?: boolean;
    onautostart?: () => void;
    intent: ChatIntent;
    options: ChatOptions;
    projectState?: ChatProjectState | null;
    onran?: (result: unknown, ctx?: RunContext) => void | Promise<void>;
    onfocus?: (id: string) => void;
    /** Called when a background op launches; returns done/fail callbacks. */
    onop?: (label: string, ctx: RunContext) => OpHandle | undefined;
    onsuggest?: (text: string) => void;
    onbriefchange?: (brief: ReturnType<typeof normalizeBrief>) => void;
    onreference?: (preset: ProjectStylePreset, style: StyleFields) => void;
  } = $props();

  let intent = $derived(normalizeChatIntent(rawIntent));
  function proposalPath(path: string): string {
    return intent.director_proposal
      ? `${path}${path.includes('?') ? '&' : '?'}director_proposal=${encodeURIComponent(intent.director_proposal)}`
      : path;
  }
  const post = <T = unknown>(path: string, body?: unknown) => apiPost<T>(proposalPath(path), body);
  const del = <T = unknown>(path: string) => apiDel<T>(proposalPath(path));
  const patch = <T = unknown>(path: string, body: unknown) => apiPatch<T>(proposalPath(path), body);

  // Each card gets one fixed intent, so plain init from props is fine.
  let busy = $state(false);
  let sceneSetupBlocking = $state(true);
  // svelte-ignore state_referenced_locally
  let idea = $state(intent.idea ?? '');
  let creationBrief = $state(untrack(() => normalizeBrief(intent.creation_brief, intent.idea ?? '')));
  let briefConfirmed = $state(false);
  function editBrief(brief: ReturnType<typeof normalizeBrief>) {
    creationBrief = brief;
    briefConfirmed = false;
    onbriefchange?.(JSON.parse(JSON.stringify(brief)));
  }
  // svelte-ignore state_referenced_locally
  let characterId = $state(intent.character_id ?? '');
  // update_character: the assistant's rewritten fields, editable before saving.
  const changeFields = [
    { key: 'appearance', label: 'Appearance' },
    { key: 'description', label: 'Description' },
    { key: 'personality', label: 'Personality' }
  ] as const;
  let characterDraft = $state<Record<string, string>>(untrack(() => ({ ...(intent.character_changes ?? {}) })));
  let remakeReference = $state(true);
  const changedCharacter = $derived(options.characters.find(c => c.id === characterId));
  let storyFormat = $state(untrack(() => (intent.action === 'generate_image' ? '1:1' : '9:16')));
  let imageType = $state(untrack(() => intent.asset_type ?? 'other'));
  // svelte-ignore state_referenced_locally
  let castIds: string[] = $state(
    Array.isArray(intent.cast_ids) && intent.cast_ids.length
      ? [...intent.cast_ids]
      : intent.character_id
        ? [intent.character_id]
        : []
  );
  // svelte-ignore state_referenced_locally
  let sceneId = $state(intent.scene_id ?? '');
  // svelte-ignore state_referenced_locally
  let shotId = $state(intent.shot_id ?? '');
  // svelte-ignore state_referenced_locally
  let outputId = $state(intent.output_id ?? '');
  let shots = $state<{ id: string; shot_order: number; prompt: string }[]>([]);
  let maxAssets = $state(4);
  let autoAssets = $state(true);
  let plannedAssets = $state<PlannedAsset[] | null>(null);
  // fit_script: the scene's lines before and after, shown until the owner saves them.
  let scriptFit = $state<ScriptFit | null>(null);
  // Ideation phase for generate_script: null = phase 1 (raw idea),
  // set = phase 2 (pick one of the developed concepts).
  let ideaOptions = $state<IdeaOptions | null>(null);
  let selectedIdea = $state(0);
  let developing = $state(false);

  let repairStoryboard = $state(false);
  const editorPanels: Record<string, { label: string; help: string; panel: string }> = {
    voice: {
      label: 'Open Voice from script',
      help: 'In Edit video → Audio, Voice from script speaks your scene lines with saved character voices or a narrator voice. Only new lines use speech credits.',
      panel: 'audio'
    },
    music: {
      label: 'Open Audio in the editor',
      help: 'Add music from your Library or upload a track in Edit video → Audio, then set its volume and fades.',
      panel: 'audio'
    },
    captions: {
      label: 'Open Captions in the editor',
      help: 'Edit video → Text makes captions from the script, timed to the sound of each clip, and can translate them.',
      panel: 'text'
    },
    export: { label: 'Open Export', help: 'Export renders your edit to an MP4 you can download.', panel: 'export' },
    edit: {
      label: 'Open Edit video',
      help: 'Trim, reorder and combine scene videos on the timeline. Nothing is generated.',
      panel: ''
    }
  };
  const editorTarget = $derived(editorPanels[intent.editor_panel as string] ?? editorPanels.edit);
  const labels: Record<string, string> = {
    update_character: 'Save changes',
    generate_script: 'Create story & references',
    generate_image: 'Generate image',
    generate_character_reference: 'Generate character reference',
    generate_character_references: 'Make pictures for all characters',
    generate_scenes: 'Expand scene',
    generate_shots: 'Generate shots',
    storyboard: 'Generate storyboard',
    audit_storyboard: 'Run visual audit',
    render_scene: 'Update storyboard & render',
    render_shot: 'Render shot',
    retry_render: 'Fix & re-render',
    generate_assets: 'Generate assets',
    refine_scene: 'AI refine scene',
    refine_shot: 'AI refine shot',
    delete_scene: 'Delete scene',
    style_ingest: 'Ingest style from story',
    plan_assets: 'Suggest assets',
    fit_script: 'Fit script to clips'
  };

  // generate_character_references: who is drawn, and whether the picture service can do it now.
  const sheetCharacters = $derived(intent.characters ?? []);
  let sheetProblem = $state('');
  $effect(() => {
    if (intent.action !== 'generate_character_references' || !intent.character_ids?.length) return;
    const ids = intent.character_ids.join(',');
    untrack(() => {
      get<CharacterSheetPlan>(`/characters/reference-sheets?character_ids=${encodeURIComponent(ids)}`)
        .then(plan => (sheetProblem = plan.ready ? '' : (plan.problem ?? '')))
        .catch(() => (sheetProblem = '')); // the button's own request checks again before spending
    });
  });

  const needsShot = (action: string) => action === 'render_shot' || action === 'refine_shot';

  /** Example chips for the unknown card, filtered to what the project can
   * actually do right now (each chip needs its previous pipeline stage). */
  const exampleChips = $derived.by(() => {
    const scenes = projectState?.scenes ?? [];
    const hasShots = scenes.some(s => s.has_shots);
    const hasStoryboard = scenes.some(s => s.has_storyboard);
    const hasRendered = scenes.some(s => s.rendered);
    return [
      'Generate a script',
      ...(!projectState || hasShots ? ['生成分镜图'] : []),
      ...(!projectState || hasStoryboard ? ['Render scene'] : []),
      ...(!projectState || hasRendered ? ['Add captions'] : []),
      '生成场景道具',
      '帮我改一下场景',
      'Suggest assets'
    ];
  });

  async function loadShots() {
    shotId = intent.shot_id && sceneId === intent.scene_id ? intent.shot_id : '';
    shots = [];
    if (!sceneId) return;
    try {
      shots = await get(`/scenes/${sceneId}/shots`);
      if (shotId && !shots.some(s => s.id === shotId)) shotId = '';
    } catch (e) {
      toast.error(`Failed to load shots: ${errorMessage(e)}`);
    }
  }

  // svelte-ignore state_referenced_locally
  if (needsShot(intent.action) && sceneId) loadShots();

  const ready = $derived.by(() => {
    switch (intent.action) {
      case 'generate_image':
        return !!idea.trim();
      case 'generate_character_reference':
        return !!characterId;
      case 'generate_character_references':
        return !!intent.character_ids?.length && !sheetProblem;
      case 'update_character':
        return !!characterId && Object.values(characterDraft).some(v => v?.trim());
      case 'generate_script':
        return creationBrief.idea.trim().length > 0 && missingBriefFields(creationBrief).length === 0 && briefConfirmed;
      case 'generate_scenes':
      case 'generate_shots':
      case 'audit_storyboard':
      case 'storyboard':
      case 'render_scene':
      case 'generate_assets':
      case 'delete_scene':
      case 'plan_assets':
      case 'fit_script':
        return !!sceneId;
      case 'refine_scene':
        return !!sceneId && idea.trim().length > 0;
      case 'render_shot':
        return !!sceneId && !!shotId;
      case 'refine_shot':
        return !!sceneId && !!shotId && idea.trim().length > 0;
      case 'style_ingest':
        return true;
      case 'retry_render':
        return !!outputId;
      default:
        return false;
    }
  });

  function videoLabel(o: ChatOptions['outputs'][number]): string {
    const file = o.video?.split('/').pop();
    return file ?? o.id;
  }

  /** Phase 1 -> 2: develop the raw idea into two mature concept options. */
  async function developIdeas() {
    developing = true;
    try {
      const result = await post<IdeaOptions>('/ideas/develop', { idea: idea.trim() });
      ideaOptions = result;
      selectedIdea = result.recommended_index ?? 0;
      idea = composedIdea();
      editBrief({ ...creationBrief, idea });
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      developing = false;
    }
  }

  /** Compose the chosen concept into the idea passed to script generation. */
  function composedIdea(): string {
    const o = ideaOptions?.options[selectedIdea];
    if (!o) return idea.trim();
    return [`${o.title} — ${o.premise}`, o.hook].filter(Boolean).join(' ').trim();
  }

  /** Long generations run as background ops: busy holds until the op is terminal. */
  async function runInBackground(
    path: string,
    body: unknown,
    label: string,
    focus: (op: Op) => string | undefined,
    ctx?: RunContext
  ) {
    busy = true;
    const handle = ctx ? onop?.(label, ctx) : undefined;
    try {
      await runBackgroundOp(proposalPath(path), body, {
        signal: operationScope.signal,
        label,
        onDone: async op => {
          busy = false;
          await handle?.done(op); // settle the chat progress row (thumbs + nav chips) first
          await onran?.(op.result_json ?? op, ctx); // let the canvas refresh before zooming to the new node
          const focusId = focus(op);
          if (focusId) onfocus?.(focusId);
        },
        onFail: op => {
          busy = false;
          handle?.fail(op);
        }
      });
    } catch (e) {
      busy = false;
      handle?.fail({ error: errorMessage(e) } as Op);
      toast.error(errorMessage(e));
    }
  }

  // Free planning steps the assistant may carry out itself in Auto mode. Nothing here
  // spends image, video or voice credits, and each result can be changed again.
  const AUTO_FREE = new Set(['refine_scene', 'refine_shot', 'generate_shots', 'plan_assets']);
  // Paid steps that start by themselves only in Full auto, where the user already agreed
  // that the assistant spends image credits without asking again.
  const AUTO_FULL = new Set([
    'generate_character_references',
    'generate_character_reference',
    'generate_scenes',
    'storyboard',
    'audit_storyboard',
    'generate_assets',
    'render_scene',
    'render_shot',
    'retry_render',
    'generate_image',
    'style_ingest',
    'fit_script'
  ]);
  let autoStarted = false;
  $effect(() => {
    if (!auto || autoStarted || busy || !ready || !(AUTO_FREE.has(intent.action) || (full && AUTO_FULL.has(intent.action))))
      return;
    autoStarted = true;
    onautostart?.();
    untrack(() => void run());
  });
  async function run() {
    const ctx: RunContext = {
      action: intent.action,
      characterId: characterId || undefined,
      sceneId: sceneId || undefined,
      outputId: outputId || undefined
    };
    switch (intent.action) {
      case 'generate_script': {
        if (!ready) {
          toast.error('Complete and confirm the creative brief first');
          return;
        }
        const brief = confirmedBriefRequest(creationBrief, briefConfirmed);
        briefConfirmed = false;
        return runInBackground(
          '/videos/generate?background=true',
          { ...brief, character_ids: castIds, create_characters: true, review_before_render: true },
          'Story & reference images',
          op => op.result_json?.scene_ids?.[0],
          ctx
        );
      }
      case 'generate_image':
        return runInBackground(
          '/assets/generate',
          { prompt: idea.trim(), aspect_ratio: storyFormat, type: imageType },
          'Image generation',
          op => op.result_json?.asset_ids?.[0],
          ctx
        );
      case 'update_character': {
        const changes = Object.fromEntries(
          Object.entries(characterDraft)
            .map(([k, v]) => [k, v.trim()])
            .filter(([, v]) => v)
        );
        busy = true;
        try {
          await patch(`/characters/${encodeURIComponent(characterId)}`, changes);
          toast.success(`${changedCharacter?.name ?? 'Character'} updated`);
        } catch (e) {
          busy = false;
          toast.error(errorMessage(e));
          return;
        }
        busy = false;
        if (remakeReference)
          return runInBackground(
            `/characters/${characterId}/reference-sheets`,
            {},
            'Character reference',
            () => characterId,
            ctx
          );
        await onran?.({ character_id: characterId }, ctx);
        onfocus?.(characterId);
        return;
      }
      case 'generate_character_references':
        return runInBackground(
          '/characters/reference-sheets',
          { character_ids: intent.character_ids ?? [], expected_image_calls: intent.image_calls },
          'Character pictures',
          () => intent.character_ids?.[0],
          ctx
        );
      case 'generate_character_reference':
        return runInBackground(
          `/characters/${characterId}/reference-sheets`,
          {},
          'Character reference',
          () => characterId,
          ctx
        );
      case 'audit_storyboard':
        return runInBackground(
          `/scenes/${sceneId}/storyboard/audit`,
          { repair: repairStoryboard },
          'Storyboard visual audit',
          op => op.result_json?.asset_id ?? sceneId,
          ctx
        );
      case 'storyboard':
        return runInBackground(
          `/scenes/${sceneId}/storyboard`,
          undefined,
          'Storyboard',
          op => op.result_json?.asset_id ?? op.scene_id ?? sceneId,
          ctx
        );
      case 'render_scene': {
        const quote = { ...renderQuote };
        const anotherVersion = newRenderVersion;
        newRenderVersion = false;
        return runInBackground(
          `/scenes/${sceneId}/render`,
          {
            prepare_storyboard: true,
            use_scene_cast: true,
            new_version: anotherVersion,
            confirmed_plan_id: renderPlanId,
            confirmed_calls: quote.calls,
            confirmed_seconds: quote.seconds,
            confirmed_storyboard_calls: quote.storyboards
          },
          'Update storyboard & render',
          op => op.result_json?.job_id,
          ctx
        );
      }
      case 'generate_shots':
        return runInBackground(
          `/scenes/${sceneId}/shots/generate`,
          { auto_assets: autoAssets },
          'Shot generation',
          op => op.result_json?.scene_id ?? op.scene_id ?? sceneId,
          ctx
        );
      case 'generate_assets':
        return runInBackground(
          `/scenes/${sceneId}/assets/generate`,
          { instruction: idea.trim(), max_assets: maxAssets },
          'Asset generation',
          op => op.result_json?.asset_ids?.[0],
          ctx
        );
    }
    busy = true;
    try {
      let result: unknown;
      let focusId: string | undefined;
      switch (intent.action) {
        case 'generate_scenes': {
          const scene = await post<Partial<Scene>>(`/scenes/${sceneId}/generate`, {});
          result = scene;
          focusId = scene.id ?? sceneId;
          toast.success('Scene expanded');
          break;
        }
        case 'render_shot': {
          const started = await post<{ job_id: string }>('/render/from-shot', { scene_id: sceneId, shot_id: shotId });
          result = started;
          focusId = started.job_id;
          toast.success(`Render started — job ${started.job_id}`);
          break;
        }
        case 'refine_scene': {
          const refined = await post<{ scene: Scene; note?: string }>(`/scenes/${sceneId}/refine`, {
            instruction: idea.trim()
          });
          result = refined;
          focusId = sceneId;
          toast.success(refined.note || 'Scene refined');
          break;
        }
        case 'refine_shot': {
          const refined = await post<{ shot: Shot; note?: string }>(`/shots/${shotId}/refine`, {
            instruction: idea.trim()
          });
          result = refined;
          focusId = shotId;
          toast.success(refined.note || 'Shot refined');
          break;
        }
        case 'delete_scene': {
          const deleted = await del<{ shots_deleted?: number }>(`/scenes/${sceneId}`);
          result = deleted;
          toast.success(`Scene deleted — ${deleted.shots_deleted ?? 0} shots removed`);
          break;
        }
        case 'style_ingest':
          result = await post('/style/ingest');
          toast.success('Style updated');
          break;
        case 'plan_assets': {
          const plan = await post<AssetPlan>(`/scenes/${sceneId}/assets/plan`, {
            instruction: idea.trim(),
            max_assets: maxAssets
          });
          result = plan;
          plannedAssets = plan.assets ?? [];
          focusId = sceneId;
          toast.success(`${plan.assets?.length ?? 0} asset suggestions`);
          break;
        }
        case 'fit_script': {
          const fit = await post<ScriptFit>(`/scenes/${sceneId}/spoken-lines/fit`);
          result = fit;
          focusId = sceneId;
          if (full) {
            // Full auto saves the fitted lines itself; the card still shows what changed.
            await apiPut(`/scenes/${encodeURIComponent(sceneId)}/spoken-lines`, {
              lines: fit.after,
              expected_updated_at: fit.expected_updated_at
            });
            toast.success(`Script fitted: ${fit.after_fit.summary}`);
          }
          scriptFit = fit;
          break;
        }
        case 'retry_render': {
          const started = await post<{ job_id: string }>(`/outputs/${outputId}/retry`);
          result = started;
          focusId = started.job_id;
          toast.success(`Corrective re-render started — job ${started.job_id}`);
          break;
        }
      }
      await onran?.(result, ctx); // let the canvas refresh before zooming to the new node
      if (focusId) onfocus?.(focusId);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      busy = false;
    }
  }

  // Card inputs are bg-muted/50 so the chat composer stays the primary input.
  const selectClass = 'w-full rounded-md border border-input bg-muted/50 px-2 py-1.5 text-sm';
  const labelClass = 'block text-xs text-muted-foreground mb-1';
</script>

<Item.Root variant="outline" class="mt-2 w-full items-start"><Item.Content class="min-w-0">
  {#if intent.action==='open_editor'}<Item.Title>Edit video</Item.Title>{:else if intent.action==='update_character'}<Item.Title>Update character</Item.Title><Item.Description>Check the new wording, then save. Nothing changes until you do.</Item.Description>{:else}<Item.Title>Creation options</Item.Title><Item.Description>Adjust the options, then choose an action to start. Editing fields does not run generation.</Item.Description>{/if}
  <div class="mt-3 space-y-3">
    {#if intent.action === 'update_character'}
      {#if changedCharacter}
        <p class="text-sm font-medium">{changedCharacter.name}</p>
        {#each changeFields as field (field.key)}{#if characterDraft[field.key]!==undefined}
          <label class="grid gap-1 text-xs">New {field.label.toLowerCase()}<Textarea bind:value={characterDraft[field.key]} rows={3}/></label>
          {#if changedCharacter[field.key]}<p class="text-xs text-muted-foreground">Was: {changedCharacter[field.key]}</p>{/if}
        {/if}{/each}
        {#if !Object.keys(characterDraft).length}<label class="grid gap-1 text-xs">New appearance<Textarea
          bind:value={characterDraft.appearance}
          rows={3}
          placeholder={changedCharacter.appearance}/></label>{/if}
        <label class="flex items-start gap-2 text-xs"><input
          type="checkbox"
          class="mt-0.5"
          bind:checked={remakeReference}
          disabled={busy}/>Then make a new reference picture so scenes use the new look (uses image credits)</label>
        <p class="text-xs text-muted-foreground">Scenes already rendered keep the old look until you render them again.</p>
        <div class="pt-1"><Button
          size="sm"
          class="rounded-full px-4"
          disabled={busy||!ready}
          onclick={run}>{#if busy}<LoaderCircle
            class="size-4 mr-1 animate-spin"/>{/if}{busy?'Saving…':remakeReference?'Save & make new picture':'Save changes'}</Button></div>
      {:else}<p class="text-xs text-muted-foreground">Choose the character to change.</p>
        <select class={selectClass} bind:value={characterId}><option
          value="">Choose a character…</option>{#each options.characters as c(c.id)}<option value={c.id}>{c.name}</option>{/each}</select>{/if}
    {:else if intent.action === 'open_editor'}
      <p class="text-xs text-muted-foreground">{editorTarget.help}</p>
      <div class="pt-1"><Button
        size="sm"
        class="rounded-full px-4"
        href={editorTarget.panel?`${editHref}&panel=${editorTarget.panel}`:editHref}>{editorTarget.label}</Button></div>
    {:else if intent.action === 'unknown'}
      {#if projectState?.next_steps?.length}
        <div class="text-sm space-y-1">
          <p class="font-medium">Next steps:</p>
          <ol class="list-decimal pl-5 space-y-0.5">
            {#each projectState.next_steps as step (step)}
              <li>{step}</li>
            {/each}
          </ol>
        </div>
      {/if}
      <details class="text-sm text-muted-foreground">
        <summary class="cursor-pointer text-xs hover:text-foreground select-none">
          What can I ask?
        </summary>
        <ul class="list-disc pl-4 mt-1 space-y-0.5">
          <li>start a new story from an idea</li>
          <li>plan shots, make a storyboard, or render a scene</li>
          <li>make a character picture, prop or background</li>
          <li>change a scene or shot in your own words</li>
          <li>add voices, music or captions, and export (opens the editor)</li>
          <li>ask why something failed and how to fix it</li>
        </ul>
      </details>
      <div class="flex flex-wrap gap-2">
        {#each exampleChips as s (s)}
          <Button variant="outline" size="sm" onclick={()=>onsuggest?.(s)}>{s}</Button>
        {/each}
      </div>
    {:else}
      {#if intent.action === 'generate_script'}
        {#if !ideaOptions}
          <div>
            <label class={labelClass} for="idea-{intent.action}">Idea</label>
            <Textarea
              id="idea-{intent.action}"
              value={creationBrief.idea}
              oninput={e=>{idea=e.currentTarget.value;editBrief({...creationBrief,idea})}}
              rows={3}
              class="bg-muted/50"
              placeholder="Describe the video idea…" />
          </div>
        {:else}
          <div class="space-y-2">
            {#each ideaOptions.options as o, i (i)}
              <button
                type="button"
                class="w-full rounded-md border p-2.5 text-left text-sm transition-colors {selectedIdea === i
                  ? 'border-primary bg-primary/5'
                  : 'border-input hover:border-muted-foreground/40'}"
                onclick={()=>{selectedIdea=i;idea=composedIdea();editBrief({...creationBrief,idea})}}
              >
                <div class="flex items-center gap-1.5 flex-wrap">
                  <span class="font-bold">{o.title}</span>
                  {#if i === ideaOptions.recommended_index}
                    <Badge class="px-1.5 py-0 text-[10px]">Recommended</Badge>
                  {/if}
                </div>
                <p class="mt-0.5">{o.premise}</p>
                {#if o.hook}<p class="mt-0.5 text-muted-foreground">{o.hook}</p>{/if}
                {#if o.why_it_works}
                  <p class="mt-0.5 italic text-muted-foreground">{o.why_it_works}</p>
                {/if}
              </button>
            {/each}
            {#if ideaOptions.reasoning}
              <p class="text-xs text-muted-foreground">{ideaOptions.reasoning}</p>
            {/if}
          </div>
        {/if}
        <CreativeBriefEditor
          bind:value={creationBrief}
          bind:confirmed={briefConfirmed}
          disabled={busy||developing}
          onchange={editBrief}
          onstyleapplied={onreference}/>

      {/if}

      {#if intent.action==='generate_image'}<label class="grid gap-1 text-xs">Image type<select
        class={selectClass} bind:value={imageType}><option value="other">General image</option><option
        value="prop">Prop</option><option value="location">Location</option><option
        value="background">Background</option></select></label><label
        class="grid gap-1 text-xs">Image prompt<Textarea bind:value={idea} rows={3}/></label><label
        class="grid gap-1 text-xs">Format<select class={selectClass} bind:value={storyFormat}><option
        value="1:1">Square · 1:1</option><option value="9:16">Vertical · 9:16</option><option
        value="16:9">Landscape · 16:9</option></select></label><p
        class="text-xs text-muted-foreground">Uses the current project style. Generated images are ready to use. Provider calls may incur charges.</p>{/if}
      {#if intent.action==='generate_character_reference'}<div><label class={labelClass} for="reference-character">Character</label><select
        id="reference-character"
        class={selectClass}
        bind:value={characterId}><option
          value="">Choose a character…</option>{#each options.characters as c(c.id)}<option
          value={c.id}>{c.name}</option>{/each}</select><p
          class="mt-2 text-xs text-muted-foreground">Generates a full-body reference candidate. Review it before use. Provider calls may incur charges.</p></div>{/if}
      {#if intent.action==='generate_character_references'}<div class="space-y-2"><p
        class="text-xs text-muted-foreground">Makes a reference picture for each of these characters, side by side:</p><ul
        class="list-disc pl-5 text-sm">{#each sheetCharacters as c (c.id)}<li>{c.name}</li>{/each}</ul><p
        class="text-xs text-muted-foreground">{intent.image_calls ?? sheetCharacters.length} picture {(intent.image_calls ?? sheetCharacters.length)===1?'call':'calls'} in all (one each, with the face close-up and full body in one picture). Uses image credits.</p>{#if sheetProblem}<p
        role="alert" class="text-xs text-destructive">{sheetProblem}</p>{/if}</div>{/if}

      {#if intent.action === 'generate_script' && options.characters.length}
      <fieldset class="space-y-1"><legend
        class="text-xs">Cast</legend>{#each options.characters as character(character.id)}<label class="flex items-center gap-2 text-xs"><input
        type="checkbox"
        bind:group={castIds}
        value={character.id}
        onchange={()=>briefConfirmed=false}/>{character.name}</label>{/each}</fieldset>
      {/if}
      {#if [
          'generate_scenes',
          'generate_shots',
          'audit_storyboard',
          'storyboard',
          'render_scene',
          'render_shot',
          'generate_assets',
          'refine_scene',
          'refine_shot',
          'delete_scene',
          'plan_assets',
          'fit_script'
        ].includes(intent.action)}
        <div>
          <label class={labelClass} for="scene-{intent.action}">Scene</label>
          <select
            id="scene-{intent.action}"
            bind:value={sceneId}
            onchange={() => needsShot(intent.action) && loadShots()}
            class={selectClass}
          >
            <option value="">choose…</option>
            {#each options.scenes as s (s.id)}<option value={s.id}>{s.title}</option>{/each}
          </select>
        </div>
      {/if}

      {#if needsShot(intent.action)}
        <div>
          <label class={labelClass} for="shot-{intent.action}">Shot</label>
          <select id="shot-{intent.action}" bind:value={shotId} disabled={!shots.length} class={selectClass}>
            <option value="">choose…</option>
            {#each shots as sh (sh.id)}
              <option value={sh.id}>#{sh.shot_order + 1} {sh.prompt.slice(0, 50)}</option>
            {/each}
          </select>
        </div>
      {/if}

      {#if intent.action==='audit_storyboard'}
        <label class="flex items-center gap-2 text-sm"><input type="checkbox" bind:checked={repairStoryboard} disabled={busy}/>Repair if needed</label>
        <p class="text-xs text-muted-foreground">Checks the image against the scene and its references. Repair can generate one replacement and check it. Provider charges may apply.</p>
      {/if}
      {#if intent.action==='render_scene'}<p
        class="text-xs text-muted-foreground">Uses the saved clip and sound plan below. Automatic mode groups consecutive shots to reduce paid calls. Matching clips are reused. Image and video generation may use account quota or provider credits.</p>{#if sceneId}<ProductionControls
          scene={sceneId}
          bind:newVersion={newRenderVersion}
          onplan={(id,cost)=>{renderPlanId=id;renderQuote=cost}}
          disabled={busy}
          onstate={s=>sceneSetupBlocking=s.blocking||s.loading}
          onask={text=>onsuggest?.(text)}/>{/if}{/if}
      {#if intent.action === 'generate_shots'}
        <label class="inline-flex items-center gap-1.5 text-xs text-muted-foreground cursor-pointer select-none">
          <input type="checkbox" class="w-auto" bind:checked={autoAssets} />
          auto props
        </label>
      {/if}

      {#if ['refine_scene', 'refine_shot'].includes(intent.action)}
        <div>
          <label class={labelClass} for="instr-{intent.action}">Instruction</label>
          <Textarea id="instr-{intent.action}" bind:value={idea} rows={2} class="bg-muted/50"
            placeholder="e.g. make the lighting warmer / 台词更简单" />
        </div>
      {/if}

      {#if intent.action === 'delete_scene'}
        <p class="text-xs text-destructive">
          Deletes the scene and its shots; renders are kept.
        </p>
      {/if}

      {#if intent.action === 'style_ingest'}
        <p class="text-xs text-muted-foreground">
          Derives style_prompt/palette/lighting/audience/tone from your story (keeps name + pinned references).
        </p>
      {/if}

      {#if ['generate_assets', 'plan_assets'].includes(intent.action)}
        <div>
          <label class={labelClass} for="instr-{intent.action}">Instruction (optional)</label>
          <Textarea id="instr-{intent.action}" bind:value={idea} rows={2} class="bg-muted/50"
            placeholder="e.g. 需要一个红色杯子 / a red cup" />
        </div>
        <div>
          <label class={labelClass} for="max-{intent.action}">Max assets</label>
          <input id="max-{intent.action}" type="number" min="1" max="8"
            bind:value={maxAssets} class={selectClass} />
        </div>
      {/if}

      {#if intent.action === 'fit_script'}
        {#if scriptFit && !full}
          <ScriptFitReview fits={[scriptFit]} characters={options.characters} onapplied={async () => { scriptFit = null; await onran?.({ scene_id: sceneId }, { action: 'fit_script', sceneId: sceneId || undefined }); }} ondismiss={() => (scriptFit = null)} />
        {:else if scriptFit}
          <p class="text-xs text-muted-foreground">Fitted and saved: {scriptFit.after_fit.summary}</p>
        {:else}
          <p class="text-xs text-muted-foreground">Rewrites only what is said, so each shot can say its lines in time. Free: one turn of your local AI session. {full ? 'Saved by itself in Full auto.' : 'You see the script before and after, and choose.'}</p>
        {/if}
      {/if}

      {#if intent.action === 'plan_assets' && plannedAssets}
        <div class="text-xs space-y-1">
          {#if plannedAssets.length === 0}
            <p class="text-muted-foreground">No missing assets — the scene is covered.</p>
          {:else}
            <ul class="list-disc pl-4 space-y-0.5">
              {#each plannedAssets as a (a.name)}
                <li>
                  <span class="font-medium">{a.name}</span>
                  <span class="text-muted-foreground">
                    ({a.asset_type}{a.shot_orders?.length
                      ? `, shots ${a.shot_orders.map((o) => o + 1).join(', ')}`
                      : ''})
                  </span>
                </li>
              {/each}
            </ul>
            <p class="text-muted-foreground">
              Generate them on the Scenes page or say '生成场景道具'.
            </p>
          {/if}
        </div>
      {/if}

      {#if intent.action === 'retry_render' && !options.outputs.length}
        <p role="status"
          class="text-sm text-muted-foreground">There is no finished video yet, so there is nothing to pick here. Make the scene's video first: ask "make the video for" and the scene name.</p>
      {:else if intent.action === 'retry_render'}
        <div>
          <label class={labelClass} for="output-{intent.action}">Output</label>
          <select id="output-{intent.action}" bind:value={outputId} class={selectClass}>
            <option value="">choose…</option>
            {#each options.outputs as o (o.id)}<option value={o.id}>{videoLabel(o)}</option>{/each}
          </select>
        </div>
      {/if}

      <div class="flex items-center gap-1.5 pt-1">
        {#if intent.action === 'generate_script' && !ideaOptions}
          <Button
            size="sm"
            class="rounded-full px-4"
            disabled={busy||developing||!ready}
            onclick={run}>{#if busy}<LoaderCircle class="size-4 mr-1 animate-spin"/>{:else}<Play
              class="size-4 mr-1"/>{/if}{busy?'Running…':'Create story & references'}</Button>
          <Button
            size="sm"
            variant="outline"
            class="rounded-full px-3"
            disabled={busy||developing||!idea.trim()}
            onclick={developIdeas}><Lightbulb class="size-4 mr-1"/>{developing?'Exploring…':'Explore ideas first'}</Button>
        {:else if intent.action === 'generate_script'}
          <Button size="sm" class="rounded-full px-4" disabled={busy} onclick={run}>
            {#if busy}
              <LoaderCircle class="size-4 mr-1 animate-spin" />
            {:else}
              <Play class="size-4 mr-1" />
            {/if}
            {busy ? 'Running…' : 'Create story & references'}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            class="rounded-full px-3"
            disabled={busy}
            onclick={() => (ideaOptions = null)}
          >
            Back
          </Button>
        {:else}
          <Button
            size="sm"
            variant={intent.action === 'delete_scene' ? 'destructive' : 'default'}
            class="rounded-full px-4"
            disabled={busy || !ready || (intent.action==='render_scene'&&sceneSetupBlocking)}
            onclick={run}
          >
            {#if busy}
              <LoaderCircle class="size-4 mr-1 animate-spin" />
            {:else if intent.action === 'delete_scene'}
              <Trash2 class="size-4 mr-1" />
            {:else}
              <Play class="size-4 mr-1" />
            {/if}
            {busy ? 'Running…' : intent.action==='render_scene'&&renderPlanId&&!renderQuote.storyboards ? 'Render scene' : (labels[intent.action] ?? 'Run')}
          </Button>
        {/if}
      </div>
    {/if}
  </div>
</Item.Content></Item.Root>
