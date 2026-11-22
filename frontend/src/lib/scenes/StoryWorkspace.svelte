<script lang="ts">
  import { errorMessage } from '$lib/errors';
  import { completeSceneRenders } from '$lib/production/completion.mjs';
  let { embedded = false }: { embedded?: boolean } = $props();
  const operationScope = new AbortController();
  onDestroy(() => operationScope.abort());
  import { onMount, onDestroy } from 'svelte';
  import { changedFields, mergeDraft } from './drafts.mjs';
  import { afterNavigate, beforeNavigate, replaceState } from '$app/navigation';
  import { page } from '$app/state';
  import { storyCreation } from '$lib/create/story-creation.svelte';
  import { get, patch, post, put, del, mediaUrl } from '$lib/api';
  import { runBackgroundOp } from '$lib/ops';
  import { Button } from '$lib/components/ui/button';
  import * as Dialog from '$lib/components/ui/dialog';
  import { Skeleton } from '$lib/components/ui/skeleton';
  import { PaneGroup, Pane, Handle } from '$lib/components/ui/resizable';
  import { toast } from 'svelte-sonner';
  import {
    Wand2,
    LayoutGrid,
    Video,
    Images,
    Trash2,
    Lightbulb,
    Clock,
    Plus,
    Clapperboard,
    ListVideo,
    Search,
    X,
    AlertTriangle,
    Sparkles
  } from '@lucide/svelte';
  import SceneListItem from '$lib/scenes/SceneListItem.svelte';
  import SceneDetail from '$lib/scenes/SceneDetail.svelte';
  import type { SceneRenderRequest } from '$lib/scenes/types';
  import type {
    Asset,
    AssetPlan,
    Character,
    PlannedAsset,
    ProjectProgress,
    RenderJob,
    Scene,
    Script,
    Shot,
    StyleGuide
  } from '$lib/api-types';

  let scenes: Scene[] = $state([]);
  let scripts: Script[] = $state([]);
  let progress = $state<ProjectProgress | null>(null);
  const progressFor = (id: string) => progress?.scenes?.find(p => p.scene_id === id) ?? null;
  let characters: Character[] = $state([]);
  let shotsByScene: Record<string, Shot[]> = $state({});
  let storyboards: Record<string, Asset> = $state({});
  // A scene's small picture: its storyboard, or a connected picture when it has none (its
  // clips may use connected pictures and never get a storyboard).
  let previews: Record<string, Asset | null> = $state({});
  // The scene's clips use its connected pictures, so nothing waits on a storyboard.
  const usesReferences = (id: string) => !!progressFor(id)?.reference_mode;
  let renderedScenes: Record<string, boolean> = $state({});
  let error = $state('');
  // Per-key busy gating: only the keys of in-flight actions are true, so
  // unrelated buttons stay live while one op runs (replaces the old single lock).
  let busy: Record<string, boolean> = $state({});
  let loaded = $state(false);
  // Long generations run as background ops — per-key so several can run at once.
  let opBusy: Record<string, boolean> = $state({});

  let idea = $state('');
  let storyCast: string[] = $state([]);
  let targetDuration: number | '' = $state('');
  let sceneCount: number | '' = $state('');
  let castSelection: Record<string, string[]> = $state({});
  let deleteTarget = $state<Scene | null>(null);
  let sceneRefine: Record<string, string> = $state({});
  let shotRefine: Record<string, string> = $state({});
  let autoProps: Record<string, boolean> = $state({});
  let assetInstr: Record<string, string> = $state({});
  let assetMax: Record<string, number> = $state({});
  let generatedAssets: Record<string, Asset[]> = $state({});
  let assetPlans: Record<string, AssetPlan | null> = $state({});
  let planSelected: Record<string, boolean[]> = $state({});
  let style = $state<StyleGuide | null>(null);
  let conversationInstruction = $state('');
  // Storyboard lightbox: the asset currently shown enlarged in a Dialog.
  let lightbox = $state<Asset | null>(null);
  // Destructive-confirm: { title, message, run } shown in a dialog when content
  // would be overwritten (regenerate shots / re-expand / regenerate storyboard).
  let confirmAction: { title: string; message: string; run: () => void } | null = $state(null);

  // ── Dirty tracking ────────────────────────────────────────────────────────
  // Baseline = the last server-loaded value of each editable field, keyed by
  // scene/shot id. A row is "dirty" when its live (bound) value differs. We
  // preserve dirty edits across refresh() (so an op elsewhere never silently
  // discards typing) and guard navigation while anything is unsaved.
  const SCENE_FIELDS = ['title', 'summary', 'duration', 'aspect_ratio'] as const;
  const SHOT_FIELDS = ['prompt', 'duration', 'camera', 'movement'] as const;
  type SceneSnapshot = Pick<Scene, (typeof SCENE_FIELDS)[number]>;
  type ShotSnapshot = Pick<Shot, (typeof SHOT_FIELDS)[number]>;
  let sceneBaseline: Record<string, SceneSnapshot> = $state({});
  let shotBaseline: Record<string, ShotSnapshot> = $state({});

  function snapScene(s: Scene): SceneSnapshot {
    return { title: s.title, summary: s.summary, duration: s.duration, aspect_ratio: s.aspect_ratio };
  }
  function snapShot(sh: Shot): ShotSnapshot {
    return { prompt: sh.prompt, duration: sh.duration, camera: sh.camera, movement: sh.movement };
  }
  function sceneDirty(s: Scene): boolean {
    const base = sceneBaseline[s.id];
    if (!base) return false;
    return SCENE_FIELDS.some(f => s[f] !== base[f]);
  }
  function shotDirty(sh: Shot): boolean {
    const base = shotBaseline[sh.id];
    if (!base) return false;
    return SHOT_FIELDS.some(f => sh[f] !== base[f]);
  }
  // Any unsaved scene OR shot edit anywhere — drives the nav/unload guard.
  const dirtyCount = $derived(
    scenes.filter(s => sceneDirty(s)).length +
      Object.values(shotsByScene)
        .flat()
        .filter(sh => shotDirty(sh)).length
  );

  // Master–detail selection (in-memory). The "Create story" generator opens in a
  // dedicated dialog; null selectedId = empty workspace.
  let selectedId: string | null = $state(null);
  let mobileDetail = $state(false);
  export async function openScene(id: string) {
    await reload();
    if (scenes.some(s => s.id === id)) {
      selectScene(id);
    }
  }
  export async function reload() {
    try {
      await refresh();
      error = '';
    } catch (e) {
      error = String(e);
    }
  }
  // A link to /?view=story&scene_id=… while Story is already open selects that scene with fresh data.
  afterNavigate(() => {
    if (!loaded || page.url.searchParams.get('view') !== 'story') return;
    const id = page.url.searchParams.get('scene_id');
    if (id && id !== selectedId) void openScene(id);
    else void reload();
  });

  // ── List UX: search + stage filter + keyboard nav ──────────────────────────
  let search = $state('');
  // '' = all; otherwise the furthest-stage label we filter on.
  let stageFilter = $state<'' | 'new' | 'expanded' | 'shots' | 'storyboard' | 'rendered'>('');
  let sceneSort = $state<'latest' | 'story'>('latest');

  const orderedScenes = $derived.by(() => {
    const storyPosition = new Map<string, { time: number; order: number }>();
    const sortedScripts = scripts.slice().sort((a, b) => (a.created_at ?? '').localeCompare(b.created_at ?? ''));
    for (let scriptIndex = 0; scriptIndex < sortedScripts.length; scriptIndex += 1) {
      const script = sortedScripts[scriptIndex];
      const scriptScenes = scenes
        .filter(scene => scene.script_id === script.id)
        .sort(
          (a, b) =>
            (a.scene_order ?? Number.MAX_SAFE_INTEGER) - (b.scene_order ?? Number.MAX_SAFE_INTEGER) ||
            (a.created_at ?? '').localeCompare(b.created_at ?? '')
        );
      for (const [sceneIndex, scene] of scriptScenes.entries()) {
        storyPosition.set(scene.id, {
          time: new Date(script.created_at ?? scene.created_at ?? 0).getTime(),
          order: scriptIndex * 10000 + (scene.scene_order ?? sceneIndex)
        });
      }
    }
    return scenes.slice().sort((a, b) => {
      if (sceneSort === 'latest') {
        return (b.updated_at ?? b.created_at ?? '').localeCompare(a.updated_at ?? a.created_at ?? '');
      }
      const aPosition = storyPosition.get(a.id) ?? { time: new Date(a.created_at ?? 0).getTime(), order: 0 };
      const bPosition = storyPosition.get(b.id) ?? { time: new Date(b.created_at ?? 0).getTime(), order: 0 };
      return aPosition.time - bPosition.time || aPosition.order - bPosition.order;
    });
  });

  function stageOf(s: Scene): 'new' | 'expanded' | 'shots' | 'storyboard' | 'rendered' {
    if (renderedScenes[s.id]) return 'rendered';
    if (storyboards[s.id]) return 'storyboard';
    if ((shotsByScene[s.id]?.length ?? 0) > 0) return 'shots';
    if (isExpanded(s)) return 'expanded';
    return 'new';
  }

  // Each story with scenes is one episode of the project's series, oldest first.
  const episodes = $derived(
    scripts
      .slice()
      .sort((a, b) => (a.created_at ?? '').localeCompare(b.created_at ?? ''))
      .filter(script => scenes.some(scene => scene.script_id === script.id))
      .map((script, i) => ({ id: script.id, number: i + 1, title: script.title || 'Untitled' }))
  );
  const episodeOf = (scene: Scene) => episodes.find(e => e.id === scene.script_id);
  let episodeFilter = $state('');
  let episodeSortSet = false;
  $effect(() => {
    if (episodes.length > 1 && !episodeSortSet) {
      episodeSortSet = true;
      sceneSort = 'story';
    }
  });
  $effect(() => {
    if (episodeFilter && !episodes.some(e => e.id === episodeFilter)) episodeFilter = '';
  });
  const nextEpisode = () =>
    window.dispatchEvent(
      new CustomEvent('videoflow-ask-assistant', { detail: { text: 'Make the next episode of this series. ' } })
    );

  const visibleScenes = $derived(
    orderedScenes.filter(s => {
      if (stageFilter && stageOf(s) !== stageFilter) return false;
      if (episodeFilter && s.script_id !== episodeFilter) return false;
      const q = search.trim().toLowerCase();
      if (!q) return true;
      return (s.title ?? '').toLowerCase().includes(q) || (s.summary ?? '').toLowerCase().includes(q);
    })
  );

  const selectedScene = $derived(scenes.find(s => s.id === selectedId) ?? null);

  // Keep selection valid as scenes change: default to first visible, fall back
  // on delete. Never auto-switch away from a still-present selection.
  $effect(() => {
    if (!loaded) return;
    if (selectedId !== null && scenes.some(s => s.id === selectedId)) return;
    selectedId = visibleScenes.length ? visibleScenes[0].id : null;
  });

  // ── Expand detection ────────────────────────────────────────────────────────
  // A stage is "done" if it OR any later stage is done. Expansion can't be told
  // apart from a stub by scene_json key-count alone (manual scenes, partial
  // specs), so the presence of shots / a storyboard / a render IMPLIES expanded.
  function hasExpandJson(s: Scene): boolean {
    return !!(s.scene_json && Object.keys(s.scene_json).length > 1);
  }
  function isExpanded(s: Scene): boolean {
    return hasExpandJson(s) || (shotsByScene[s.id]?.length ?? 0) > 0 || !!storyboards[s.id] || !!renderedScenes[s.id];
  }

  // Merge fresh server rows over the previous state WITHOUT clobbering unsaved
  // edits: a dirty scene/shot keeps its in-memory field values; everything else
  // (and all non-editable fields) take the server's values, and the baseline is
  // (re)set to the server snapshot so saved rows go clean.
  function mergeScenes(fresh: Scene[]) {
    const prevById = new Map(scenes.map(s => [s.id, s]));
    const baseline: Record<string, SceneSnapshot> = {};
    const merged = fresh.map(srv => {
      // Snapshot the baseline from the UNTOUCHED server row first, so it always
      // reflects the latest server field values — never the kept edits below.
      baseline[srv.id] = snapScene(srv);
      const prev = prevById.get(srv.id);
      if (prev && sceneDirty(prev)) {
        // keep edits; the baseline (captured above) stays at the server snapshot
        // so the row remains dirty until the user actually Saves.
        Object.assign(srv, mergeDraft(srv, prev, sceneBaseline[srv.id], SCENE_FIELDS));
      }
      return srv;
    });
    sceneBaseline = baseline;
    return merged;
  }
  function mergeShots(sceneId: string, fresh: Shot[]) {
    const prevById = new Map((shotsByScene[sceneId] ?? []).map(s => [s.id, s]));
    const merged = fresh.map(srv => {
      // Snapshot the baseline from the UNTOUCHED server row first (before the
      // dirty-keep loop overwrites srv), so a kept edit still reads as dirty.
      const oldBaseline = shotBaseline[srv.id];
      shotBaseline[srv.id] = snapShot(srv);
      const prev = prevById.get(srv.id);
      if (prev && shotDirty(prev)) {
        Object.assign(srv, mergeDraft(srv, prev, oldBaseline, SHOT_FIELDS));
      }
      return srv;
    });
    shotBaseline = shotBaseline; // re-assign so dirty derivations re-run
    return merged;
  }

  async function refresh() {
    const [freshScenes, chars, freshScripts] = await Promise.all([
      get<Scene[]>('/scenes'),
      get<Character[]>('/characters'),
      get<Script[]>('/scripts')
    ]);
    characters = chars;
    scripts = freshScripts;
    const [allAssets, jobs, freshProgress] = await Promise.all([
      get<Asset[]>('/assets'),
      get<RenderJob[]>('/render-jobs').catch((): RenderJob[] => []),
      get<ProjectProgress>('/production/progress').catch(() => null)
    ]);
    progress = freshProgress;
    generatedAssets = Object.fromEntries(
      freshScenes.map(scene => [
        scene.id,
        allAssets.filter(
          a =>
            a.type !== 'storyboard' && (scene.asset_ids_json?.includes(a.id) || a.metadata_json?.scene_id === scene.id)
        )
      ])
    );
    storyboards = {};
    for (const a of allAssets) {
      const sid = a.metadata_json?.scene_id;
      if (
        sid &&
        !storyboards[sid] &&
        a.type === 'storyboard' &&
        freshScenes.find(scene => scene.id === sid)?.asset_ids_json?.includes(a.id)
      )
        storyboards[sid] = a;
    }
    const picture = (a: Asset) =>
      /\.(png|jpe?g|webp|gif)$/i.test(a.file_path ?? '') &&
      a.type !== 'storyboard' &&
      a.metadata_json?.consistency_review !== 'rejected';
    previews = Object.fromEntries(
      freshScenes.map(scene => [
        scene.id,
        storyboards[scene.id] ??
          allAssets.find(
            a =>
              picture(a) &&
              (scene.asset_ids_json?.includes(a.id) ||
                (a.type === 'character_reference' && !!a.character_id && scene.character_ids_json?.includes(a.character_id)))
          ) ??
          null
      ])
    );
    renderedScenes = Object.fromEntries([...completeSceneRenders(jobs)].map(id => [id, true]));
    scenes = mergeScenes(freshScenes);
    await Promise.all(
      scenes.map(async s => {
        // One scene's shots failing must not blank the whole list.
        const fresh = await get<Shot[]>(`/scenes/${s.id}/shots`).catch((): Shot[] => []);
        shotsByScene[s.id] = mergeShots(s.id, fresh);
      })
    );
    shotsByScene = shotsByScene;
  }
  onMount(() => {
    refresh()
      .then(() => {
        const sceneID = new URL(window.location.href).searchParams.get('scene_id');
        if (sceneID && scenes.some(s => s.id === sceneID)) selectScene(sceneID);
      })
      .catch(e => (error = e.message))
      .finally(() => (loaded = true));
    get<StyleGuide>('/style')
      .then(s => (style = s))
      .catch(() => {});

    const onUnload = (e: BeforeUnloadEvent) => {
      if (dirtyCount > 0) {
        e.preventDefault();
        e.returnValue = '';
      }
    };
    window.addEventListener('beforeunload', onUnload);
    return () => window.removeEventListener('beforeunload', onUnload);
  });

  beforeNavigate(nav => {
    if (dirtyCount > 0 && !confirm('You have unsaved scene/shot edits. Leave anyway?')) nav.cancel();
  });

  async function run(key: string, fn: () => Promise<unknown>, doneMsg = '') {
    busy[key] = true;
    error = '';
    try {
      await fn();
      await refresh();
      if (doneMsg) toast.success(doneMsg);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      busy[key] = false;
    }
  }

  const expandScene = (s: Scene) =>
    run(`expand-${s.id}`, () => post(`/scenes/${s.id}/generate`, { character_ids: castSelection[s.id] ?? [] }));

  // Re-expand overwrites the expanded scene + drops its shots → confirm first.
  function expandSceneGuarded(s: Scene) {
    if (isExpanded(s)) {
      confirmAction = {
        title: 'Re-expand scene?',
        message:
          'Re-expanding rewrites this scene from the cast and may replace its shots. Unsaved edits on this scene will be lost. Continue?',
        run: () => expandScene(s)
      };
    } else {
      expandScene(s);
    }
  }

  async function generateShots(s: Scene) {
    const key = `shots-${s.id}`;
    opBusy[key] = true;
    try {
      await runBackgroundOp(
        `/scenes/${s.id}/shots/generate`,
        { auto_assets: autoProps[s.id] ?? true },
        {
          signal: operationScope.signal,
          label: 'Shot generation',
          onDone: async () => {
            opBusy[key] = false;
            // Shots changed (and auto props may have generated assets) — refetch
            // this scene's shots and drop any now-stale suggestion plan.
            shotsByScene[s.id] = mergeShots(s.id, await get<Shot[]>(`/scenes/${s.id}/shots`));
            shotsByScene = shotsByScene;
            assetPlans[s.id] = null;
          },
          onFail: () => (opBusy[key] = false)
        }
      );
    } catch (e) {
      opBusy[key] = false;
      toast.error(errorMessage(e));
    }
  }

  // Regenerating replaces every existing shot → confirm when shots exist.
  function generateShotsGuarded(s: Scene) {
    if ((shotsByScene[s.id]?.length ?? 0) > 0) {
      confirmAction = {
        title: 'Regenerate shots?',
        message:
          'This replaces every existing shot for this scene with a fresh AI breakdown. Unsaved shot edits will be lost. Continue?',
        run: () => generateShots(s)
      };
    } else {
      generateShots(s);
    }
  }

  async function generateStoryboard(s: Scene) {
    const key = `sb-${s.id}`;
    opBusy[key] = true;
    try {
      await runBackgroundOp(`/scenes/${s.id}/storyboard`, undefined, {
        signal: operationScope.signal,
        label: 'Storyboard',
        onDone: async () => {
          opBusy[key] = false;
          await refresh();
        },
        onFail: () => (opBusy[key] = false)
      });
    } catch (e) {
      opBusy[key] = false;
      toast.error(errorMessage(e));
    }
  }

  // Regenerating replaces the existing 分镜图 → confirm when one exists.
  function generateStoryboardGuarded(s: Scene) {
    if (storyboards[s.id]) {
      confirmAction = {
        title: 'Regenerate storyboard?',
        message:
          'Create a new storyboard candidate? Previous images stay in Library. Review the new image before rendering.',
        run: () => generateStoryboard(s)
      };
    } else {
      generateStoryboard(s);
    }
  }

  async function renderScene(s: SceneRenderRequest) {
    const key = `render-${s.id}`;
    if (busy[key]) return;
    busy[key] = true;
    error = '';
    try {
      await runBackgroundOp(
        `/scenes/${s.id}/render`,
        {
          prepare_storyboard: true,
          use_scene_cast: true,
          confirmed_plan_id: s.production_plan_id,
          new_version: !!s.new_version,
          confirmed_calls: s.confirmed_calls,
          confirmed_seconds: s.confirmed_seconds,
          confirmed_storyboard_calls: s.confirmed_storyboard_calls
        },
        {
          signal: operationScope.signal,
          label: 'Update storyboard & render',
          onDone: async () => {
            busy[key] = false;
            await refresh();
            toast.success('Render queued. Track it in Videos.');
          },
          onFail: op => {
            busy[key] = false;
            error = op.error ?? 'Render preparation failed.';
          }
        }
      );
    } catch (e) {
      busy[key] = false;
      error = String(e);
      toast.error(error);
    }
  }

  // Pipeline stepper: one entry per stage, computed from data already loaded.
  // Each stage's "done" uses the later-done rule so the stepper never shows an
  // earlier stage as incomplete once a later one exists.
  function sceneSteps(s: Scene) {
    const hasShots = (shotsByScene[s.id]?.length ?? 0) > 0;
    const hasStoryboard = !!storyboards[s.id];
    const hasRender = !!renderedScenes[s.id];
    return [
      {
        key: 'expand',
        label: 'Expand',
        icon: Wand2,
        done: isExpanded(s),
        busy: !!busy[`expand-${s.id}`],
        busyLabel: 'Expanding…',
        action: () => expandSceneGuarded(s)
      },
      {
        key: 'shots',
        label: 'Shots',
        icon: LayoutGrid,
        done: hasShots || hasStoryboard || hasRender,
        busy: !!opBusy[`shots-${s.id}`],
        busyLabel: 'Generating shots…',
        action: () => generateShotsGuarded(s)
      },
      {
        key: 'storyboard',
        signal: operationScope.signal,
        label: 'Storyboard',
        icon: Images,
        done: hasStoryboard || hasRender || (hasShots && usesReferences(s.id)),
        busy: !!opBusy[`sb-${s.id}`],
        busyLabel: 'Generating storyboard…',
        action: () => generateStoryboardGuarded(s)
      },
      {
        key: 'render',
        label: 'Render',
        icon: Video,
        done: hasRender,
        busy: !!busy[`render-${s.id}`],
        busyLabel: 'Submitting…',
        action: () => renderScene(s)
      }
    ];
  }

  // Furthest-completed stage index (-1 = nothing done) → drives the left accent
  // colour and the at-a-glance status line.
  function progressIndex(s: Scene) {
    const steps = sceneSteps(s);
    let last = -1;
    for (let i = 0; i < steps.length; i++) if (steps[i].done) last = i;
    return last;
  }

  // Status-dot colour by furthest stage (mirrors the old left-accent palette).
  const DOT = [
    'bg-sky-500/80', // expanded
    'bg-violet-500/80', // shots
    'bg-amber-500/80', // storyboard
    'bg-emerald-500/80' // rendered
  ];
  function accentDot(s: Scene) {
    const i = progressIndex(s);
    return i >= 0 ? DOT[i] : 'bg-border';
  }

  // One-line, human status derived purely from already-loaded data.
  function statusLine(s: Scene): string {
    const shots = shotsByScene[s.id]?.length ?? 0;
    const parts: string[] = [];
    const p = progressFor(s.id);
    if (p) {
      parts.push(p.label);
      if (shots > 0) parts.push(`${shots} shot${shots === 1 ? '' : 's'}`);
      if (s.duration) parts.push(`${s.duration}s`);
      return parts.join(' · ');
    }
    if (renderedScenes[s.id]) parts.push('Rendered');
    else if (storyboards[s.id]) parts.push('Storyboard ready');
    else if (shots > 0) parts.push('Shots ready');
    else if (isExpanded(s)) parts.push('Expanded');
    else parts.push('New — needs expanding');
    if (shots > 0) parts.push(`${shots} shot${shots === 1 ? '' : 's'}`);
    if (s.duration) parts.push(`${s.duration}s`);
    return parts.join(' · ');
  }

  function toggleCast(sceneId: string, charId: string) {
    const cur = castSelection[sceneId] ?? [];
    castSelection[sceneId] = cur.includes(charId) ? cur.filter(c => c !== charId) : [...cur, charId];
  }

  const saveScene = (s: Scene) =>
    run(
      `save-${s.id}`,
      () => patch(`/scenes/${s.id}`, changedFields(s, sceneBaseline[s.id], SCENE_FIELDS)),
      'Scene saved.'
    );

  async function confirmDeleteScene() {
    const s = deleteTarget;
    if (!s) return;
    busy[`delete-${s.id}`] = true;
    try {
      const r = await del<{ deleted: string; shots_deleted: number }>(`/scenes/${s.id}`);
      deleteTarget = null;
      // If we just deleted the selected scene, drop selection so the $effect
      // falls back to the first remaining scene (or the empty state).
      if (selectedId === s.id) selectedId = null;
      toast.success(`Scene deleted (${r.shots_deleted} shots removed).`);
      await refresh();
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      busy[`delete-${s.id}`] = false;
    }
  }

  async function deleteShot(scene: Scene, shot: Shot) {
    busy[`delete-${shot.id}`] = true;
    try {
      await del(`/shots/${shot.id}`);
      delete shotBaseline[shot.id];
      toast.success('Shot deleted.');
      shotsByScene[scene.id] = mergeShots(scene.id, await get<Shot[]>(`/scenes/${scene.id}/shots`));
      shotsByScene = shotsByScene;
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      busy[`delete-${shot.id}`] = false;
    }
  }

  async function addShot(scene: Scene) {
    busy[`add-${scene.id}`] = true;
    try {
      await post(`/scenes/${scene.id}/shots`, {});
      toast.success('Shot added.');
      shotsByScene[scene.id] = mergeShots(scene.id, await get<Shot[]>(`/scenes/${scene.id}/shots`));
      shotsByScene = shotsByScene;
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      busy[`add-${scene.id}`] = false;
    }
  }

  // Move a shot up/down by swapping with its neighbour, then persist the order.
  async function moveShot(scene: Scene, shot: Shot, dir: -1 | 1) {
    const list = (shotsByScene[scene.id] ?? []).slice();
    const i = list.findIndex(s => s.id === shot.id);
    const j = i + dir;
    if (i < 0 || j < 0 || j >= list.length) return;
    [list[i], list[j]] = [list[j], list[i]];
    busy[`reorder-${scene.id}`] = true;
    try {
      const ordered = await put<Shot[]>(`/scenes/${scene.id}/shots/order`, {
        ordered_ids: list.map(s => s.id)
      });
      shotsByScene[scene.id] = mergeShots(scene.id, ordered);
      shotsByScene = shotsByScene;
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      busy[`reorder-${scene.id}`] = false;
    }
  }

  async function refineScene(s: Scene) {
    const instruction = (sceneRefine[s.id] ?? '').trim();
    if (!instruction) return;
    busy[`refine-${s.id}`] = true;
    try {
      const r = await post<{ scene: Scene; note?: string }>(`/scenes/${s.id}/refine`, { instruction });
      Object.assign(s, r.scene);
      sceneBaseline[s.id] = snapScene(s); // refined values are the new clean baseline
      sceneRefine[s.id] = '';
      toast.success(r.note || 'Refined');
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      busy[`refine-${s.id}`] = false;
    }
  }

  async function refineShot(scene: Scene, shot: Shot) {
    const instruction = (shotRefine[scene.id] ?? '').trim();
    if (!instruction) {
      toast.error('Type an instruction in the shot refine box first.');
      return;
    }
    busy[`refine-${shot.id}`] = true;
    try {
      const r = await post<{ shot: Shot; note?: string }>(`/shots/${shot.id}/refine`, { instruction });
      Object.assign(shot, r.shot);
      shotBaseline[shot.id] = snapShot(shot);
      toast.success(r.note || 'Refined');
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      busy[`refine-${shot.id}`] = false;
    }
  }

  async function convertAllScenesConversational() {
    const ids = scenes.map(s => s.id);
    if (!ids.length) {
      toast.info('No scenes to convert.');
      return;
    }
    const key = 'conv-all';
    busy[key] = true;
    error = '';
    try {
      await runBackgroundOp(
        '/scenes/conversationalize',
        {
          scene_ids: ids,
          include_shots: true,
          instruction: conversationInstruction.trim(),
          brief: conversationBrief(conversationInstruction.trim()),
          preview: false
        },
        {
          signal: operationScope.signal,
          label: 'Conversational conversion',
          onDone: async () => {
            busy[key] = false;
            await refresh();
          },
          onFail: () => {
            busy[key] = false;
          }
        }
      );
    } catch (e) {
      busy[key] = false;
      toast.error(errorMessage(e));
    }
  }

  function conversationBrief(goal: string) {
    return {
      goal: goal.slice(0, 500),
      tone: 'natural',
      language: '',
      relationship: '',
      speaker_order: [],
      max_words_per_line: 10,
      allow_narration: false
    };
  }

  function convertAllScenesConversationalGuarded() {
    if (!scenes.length) {
      toast.info('No scenes found.');
      return;
    }
    confirmAction = {
      title: 'Conversationalize all scenes?',
      message:
        'This will run AI conversion on every scene and shot in the active project. This can take time and cannot be undone.',
      run: convertAllScenesConversational
    };
  }

  async function startAssetGeneration(s: Scene, instruction: string, maxAssets: number, items?: PlannedAsset[]) {
    const key = `assets-${s.id}`;
    opBusy[key] = true;
    try {
      await runBackgroundOp(
        `/scenes/${s.id}/assets/generate`,
        { instruction, max_assets: maxAssets, items },
        {
          signal: operationScope.signal,
          label: 'Asset generation',
          onDone: async op => {
            opBusy[key] = false;
            const ids = op.result_json?.asset_ids ?? [];
            const all = await get<Asset[]>('/assets');
            generatedAssets[s.id] = all.filter(a => ids.includes(a.id));
            assetPlans[s.id] = null; // any pending suggestion plan is now out of date
            // Generation binds reviewed references to the scene; refresh its shots.
            shotsByScene[s.id] = mergeShots(s.id, await get<Shot[]>(`/scenes/${s.id}/shots`));
            shotsByScene = shotsByScene;
          },
          onFail: () => (opBusy[key] = false)
        }
      );
    } catch (e) {
      opBusy[key] = false;
      toast.error(errorMessage(e));
    }
  }

  const generateAssets = (s: Scene) => startAssetGeneration(s, (assetInstr[s.id] ?? '').trim(), assetMax[s.id] ?? 4);

  async function suggestAssets(s: Scene) {
    busy[`plan-${s.id}`] = true;
    assetPlans[s.id] = null; // hide any stale plan while re-planning
    try {
      const plan = await post<AssetPlan>(`/scenes/${s.id}/assets/plan`, {
        instruction: (assetInstr[s.id] ?? '').trim(),
        max_assets: assetMax[s.id] ?? 4
      });
      assetPlans[s.id] = plan;
      planSelected[s.id] = (plan.assets ?? []).map(() => true);
      if (!plan.assets?.length) toast.info('No new assets suggested for this scene.');
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      busy[`plan-${s.id}`] = false;
    }
  }

  async function generateSelectedAssets(s: Scene) {
    const plan = assetPlans[s.id];
    if (!plan) return;
    const selected = plan.assets.filter((_, i) => planSelected[s.id]?.[i]);
    if (!selected.length) return;
    const userInstruction = (assetInstr[s.id] ?? '').trim();
    const instruction =
      'Generate exactly these assets: ' +
      selected.map(a => `${a.name} — ${a.description}`).join('; ') +
      (userInstruction ? `. ${userInstruction}` : '');
    await startAssetGeneration(s, instruction, selected.length, selected);
  }

  const saveShot = (shot: Shot) =>
    run(
      `save-${shot.id}`,
      () => patch(`/shots/${shot.id}`, changedFields(shot, shotBaseline[shot.id], SHOT_FIELDS)),
      'Shot saved.'
    );

  function selectScene(id: string) {
    selectedId = id;
    mobileDetail = true;
    const url = new URL(page.url);
    url.searchParams.set('scene_id', id);
    replaceState(url, { ...page.state });
  }

  // Arrow-key navigation in the left list (when not typing in a field).
  function listKeydown(e: KeyboardEvent) {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
    const list = visibleScenes;
    if (!list.length) return;
    const i = list.findIndex(s => s.id === selectedId);
    const next = e.key === 'ArrowDown' ? Math.min(list.length - 1, i + 1) : Math.max(0, i - 1);
    if (next !== i || i === -1) {
      e.preventDefault();
      selectScene(list[next === -1 ? 0 : next].id);
    }
  }

  const STAGE_FILTERS: { value: typeof stageFilter; label: string }[] = [
    { value: '', label: 'All' },
    { value: 'new', label: 'New' },
    { value: 'expanded', label: 'Expanded' },
    { value: 'shots', label: 'Shots' },
    { value: 'storyboard', label: 'Storyboard' },
    { value: 'rendered', label: 'Rendered' }
  ];
</script>

<svelte:head>{#if !embedded}<title>Scene studios · VideoFlow</title>{/if}</svelte:head>

<div class="story-workspace flex h-full min-h-0 flex-col {mobileDetail?'show-detail':''}">
  <!-- Page header (unchanged copy) -->
  <div class="px-6 pt-4 pb-3 shrink-0">
    {#if !embedded}<h1 class="text-lg font-semibold">Scene studios</h1>{/if}
    <div class="flex flex-wrap items-center gap-x-4 gap-y-2">
    {#if progress?.total}
      <div data-tour="progress" class="flex min-w-0 flex-wrap items-center gap-x-4 gap-y-2" role="status" aria-label="Project progress">
        <div class="flex min-w-0 items-center gap-3">
          <div
            class="h-1.5 w-20 shrink-0 overflow-hidden rounded-full bg-muted"
            aria-hidden="true"><div
            class="h-full rounded-full bg-emerald-500/80"
            style:width={`${Math.round(100 * progress.video_ready / progress.total)}%`}></div></div>
          <p class="text-sm">{progress.summary}</p>
        </div>
        {#if progress.next}{@const next = progress.next}<Button
          size="sm"
          variant={next.stage === 'needs_attention' ? 'default' : 'outline'}
          onclick={() => { selectedId = next.scene_id; mobileDetail = true; }}>Next: {next.next.label} · {next.title}</Button>
        {:else if progress.video_ready === progress.total}<Button size="sm" href="/?view=edit">Open editor</Button>{/if}
      </div>
    {/if}
    <details class="dialogue-tools relative ml-auto"><summary
      class="inline-flex min-h-9 cursor-pointer list-none items-center gap-1.5 rounded-md px-2 text-sm text-muted-foreground hover:bg-accent hover:text-foreground"><Sparkles
      class="size-3.5" />Dialogue tools</summary><div
      class="absolute right-0 top-full z-30 mt-1 w-80 max-w-[calc(100vw-2rem)] space-y-2 rounded-lg border border-border bg-popover p-3 shadow-lg">
      <p class="text-xs text-muted-foreground">Rewrite every scene so characters talk to each other. Uses the AI writer.</p>
      <input
        bind:value={conversationInstruction}
        aria-label="Dialogue direction"
        placeholder="Direction, e.g. “more playful” (optional)"
        class="w-full rounded-md border border-input bg-background px-2 py-1.5 text-sm"
      />
      <Button size="sm" class="w-full" disabled={busy['conv-all']} onclick={convertAllScenesConversationalGuarded}>
        {busy['conv-all'] ? 'Rewriting…' : 'Make story conversational'}
      </Button>
    </div></details>
    </div>
  </div>

  <PaneGroup direction="horizontal" class="flex-1 min-h-0 border-t border-border">
    <!-- LEFT: scene list (the monitor) -->
    <Pane defaultSize={28} minSize={20} class="scene-list-pane min-w-0">
      <div class="flex h-full flex-col">
        <!-- Create story affordance + search + stage filter -->
        <div class="shrink-0 border-b border-border p-3 space-y-2.5">
          {#if !embedded}          <Button variant="default" size="sm" class="w-full justify-start"
            onclick={() => storyCreation.show()}>
            <Plus class="size-4 mr-1.5" />Create story
          </Button>{/if}

          <div class="relative">
            <Search class="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground/70 pointer-events-none" />
            <input
              bind:value={search}
              placeholder="Search scenes…"
              aria-label="Search scenes"
              class="w-full rounded-md border border-input bg-background pl-8 pr-8 py-1.5 text-sm" />
            {#if search}
              <button type="button" title="Clear search"
                class="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground/70 hover:text-foreground"
                onclick={() => (search = '')}>
                <X class="size-3.5" />
              </button>
            {/if}
          </div>

          {#if episodes.length}
            <div class="flex items-center gap-2">
              {#if episodes.length > 1}
                <select
                  bind:value={episodeFilter}
                  aria-label="Episode"
                  class="min-h-9 min-w-0 flex-1 rounded-md border border-input bg-background px-2 text-sm">
                  <option value="">All {episodes.length} episodes</option>
                  {#each episodes as e (e.id)}<option value={e.id}>Episode {e.number} · {e.title}</option>{/each}
                </select>
              {:else}<span class="min-w-0 flex-1 truncate text-xs text-muted-foreground">Episode 1 · {episodes[0].title}</span>{/if}
              <Button
                size="sm"
                variant="secondary"
                class="shrink-0"
                title="Ask the assistant to write the next episode with the same characters and format"
                onclick={nextEpisode}><Plus class="size-4 mr-1" />Next episode</Button>
            </div>
          {/if}
          <details class="rounded-md border border-border px-2.5">
            <summary class="cursor-pointer py-2 text-xs font-medium">Filter & sort{stageFilter ? ' · Filtered' : ''}</summary>
            <div class="space-y-2 pb-2">
              <label class="grid gap-1 text-xs text-muted-foreground">Stage
                <select bind:value={stageFilter} class="min-h-9 rounded border border-input bg-background px-2 text-sm text-foreground">
                  {#each STAGE_FILTERS as filter (filter.value)}
                    <option value={filter.value}>{filter.label}</option>
                  {/each}
                </select>
              </label>
              <label class="grid gap-1 text-xs text-muted-foreground">Order
                <select bind:value={sceneSort} class="min-h-9 rounded border border-input bg-background px-2 text-sm text-foreground">
                  <option value="latest">Latest</option>
                  <option value="story">Story order</option>
                </select>
              </label>
            </div>
          </details>
        </div>

        <!-- The list itself -->
        <!-- Named button group also supports arrow-key selection from its container. -->
        <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
        <div
          class="min-h-0 flex-1 overflow-y-auto p-2 space-y-1.5 focus:outline-none focus-visible:outline-solid focus-visible:outline-2 focus-visible:outline-ring focus-visible:-outline-offset-2"
          tabindex="0"
          role="group"
          aria-label="Scenes"
          onkeydown={listKeydown}>
          {#if !loaded}
            {#each Array(5) as _, i (i)}
              <div class="flex items-start gap-2.5 rounded-lg border border-transparent px-2.5 py-2">
                <Skeleton class="size-10 shrink-0 rounded-md" />
                <div class="flex-1 space-y-1.5">
                  <Skeleton class="h-3.5 w-32" />
                  <Skeleton class="h-3 w-40" />
                  <Skeleton class="h-2 w-24" />
                </div>
              </div>
            {/each}
          {:else if error}
            <div class="rounded-lg border border-border p-3">
              <p class="text-sm text-destructive mb-2">Could not load scenes: {error}</p>
              <Button size="sm" variant="secondary"
                onclick={() => { error = ''; refresh().catch((e) => (error = e.message)); }}>Retry</Button>
            </div>
          {:else if orderedScenes.length === 0}
            <div class="rounded-lg border border-dashed border-border p-3">
              <div class="flex items-center gap-2 mb-1">
                <Lightbulb class="size-4" />
                <span class="font-medium text-sm">Create your first scene</span>
              </div>
              <p class="text-sm text-muted-foreground">
                Click <span class="font-medium">Create story</span> to generate scenes from a script.
              </p>
              <p class="text-sm text-muted-foreground mt-1">
                Choose existing characters or let Create story add the ones it needs.
              </p>
              <Button size="sm" class="mt-2" onclick={() => storyCreation.show()}>
                <Plus class="size-4 mr-1" />Create story
              </Button>
            </div>
          {:else if visibleScenes.length === 0}
            <div class="rounded-lg border border-dashed border-border p-3 text-center">
              <p class="text-sm text-muted-foreground">No scenes match this search/filter.</p>
              <Button size="sm" variant="secondary" class="mt-2"
                onclick={() => { search = ''; stageFilter = ''; }}>Clear filters</Button>
            </div>
          {:else}
            {#each visibleScenes as s, i (s.id)}
              {#if episodes.length > 1 &&
                sceneSort === 'story' &&
                !episodeFilter &&
                (i === 0 || visibleScenes[i - 1].script_id !== s.script_id)}{@const e = episodeOf(s)}{#if e}<p
                class="px-1 pt-2 text-xs font-medium text-muted-foreground">Episode {e.number} · {e.title}</p>{/if}{/if}
              <SceneListItem
                scene={s}
                selected={s.id === selectedId}
                statusLine={statusLine(s)}
                accentDot={accentDot(s)}
                dirty={sceneDirty(s) || (shotsByScene[s.id] ?? []).some(sh => shotDirty(sh))}
                storyboard={previews[s.id] ?? null}
                onselect={() => selectScene(s.id)}
              />
            {/each}
          {/if}
        </div>
      </div>
    </Pane>

    <Handle withHandle />

    <!-- RIGHT: selected scene detail (the workspace) -->
    <Pane defaultSize={72} minSize={40} class="scene-detail-pane min-w-0">
      <Button variant="ghost" class="m-2 md:hidden" onclick={()=>mobileDetail=false}>Back to scenes</Button>
      <div class="h-full overflow-y-auto">
        {#if !loaded}
          <div class="p-5 space-y-4">
            <div class="flex items-start gap-4">
              <Skeleton class="size-16 shrink-0 rounded-lg" />
              <div class="flex-1 space-y-2">
                <Skeleton class="h-5 w-56" />
                <Skeleton class="h-6 w-80" />
              </div>
              <Skeleton class="h-8 w-28" />
            </div>
            <Skeleton class="h-24 w-full" />
            <Skeleton class="h-40 w-full" />
          </div>
        {:else if selectedScene}
          {#key selectedScene.id}
            <SceneDetail
              s={selectedScene}
              steps={sceneSteps(selectedScene)}
              {characters}
              {style}
              {busy}
              {opBusy}
              sceneDirty={sceneDirty(selectedScene)}
              shotDirty={(sh: Shot) => shotDirty(sh)}
              storyboard={storyboards[selectedScene.id] ?? null}
              preview={previews[selectedScene.id] ?? null}
              shots={shotsByScene[selectedScene.id] ?? []}
              bind:castSelection
              bind:sceneRefine
              bind:shotRefine
              bind:autoProps
              bind:assetInstr
              bind:assetMax
              bind:generatedAssets
              bind:assetPlans
              bind:planSelected
              onExpand={expandSceneGuarded}
              onSave={saveScene}
              onRefineScene={refineScene}
              onToggleCast={toggleCast}
              onGenerateShots={generateShotsGuarded}
              onAddShot={addShot}
              onMoveShot={moveShot}
              onSaveShot={saveShot}
              onRefineShot={refineShot}
              onDeleteShot={deleteShot}
              onSuggestAssets={suggestAssets}
              onGenerateAssets={generateAssets}
              onGenerateSelectedAssets={generateSelectedAssets}
              onGenerateStoryboard={generateStoryboardGuarded}
              onRenderScene={renderScene}
              onDelete={(s: Scene) => (deleteTarget = s)}
              onLightbox={(a: Asset | null) => (lightbox = a)}
              onReferencesChanged={refresh}
              progress={progressFor(selectedScene.id)}
            />
          {/key}
        {:else}
          <div class="flex h-full flex-col items-center justify-center text-center text-muted-foreground p-8">
            <ListVideo class="size-10 mb-3 opacity-40" />
            <p class="text-sm">Choose a scene or start a story</p>
            <p class="text-xs mt-1 max-w-xs">Select a scene, or click <span class="font-medium">Create story</span>.</p>
          </div>
        {/if}
      </div>
    </Pane>
  </PaneGroup>
</div>

<Dialog.Root open={lightbox !== null} onOpenChange={(open) => !open && (lightbox = null)}>
  <Dialog.Content class="max-w-4xl">
    <Dialog.Header>
      <Dialog.Title class="flex items-center gap-2"><Images class="size-4" />Storyboard</Dialog.Title>
    </Dialog.Header>
    {#if lightbox}
      <img class="w-full rounded-lg" src={mediaUrl(lightbox.file_path)} alt="Storyboard" />
    {/if}
  </Dialog.Content>
</Dialog.Root>

<!-- Destructive-action confirm (regenerate shots / re-expand / regenerate storyboard) -->
<Dialog.Root open={confirmAction !== null} onOpenChange={(open) => !open && (confirmAction = null)}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title class="flex items-center gap-2">
        <AlertTriangle class="size-4 text-amber-500" />{confirmAction?.title}
      </Dialog.Title>
      <Dialog.Description>{confirmAction?.message}</Dialog.Description>
    </Dialog.Header>
    <Dialog.Footer>
      <Button variant="outline" size="sm" onclick={() => (confirmAction = null)}>Cancel</Button>
      <Button size="sm" onclick={() => { const a = confirmAction; confirmAction = null; a?.run(); }}>
        Continue
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<Dialog.Root open={deleteTarget !== null} onOpenChange={(open) => !open && (deleteTarget = null)}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>
        Delete scene and its {(deleteTarget && shotsByScene[deleteTarget.id]?.length) ?? 0} shots?
      </Dialog.Title>
      <Dialog.Description>
        "{deleteTarget?.title}" and all of its shots will be permanently deleted.
        Rendered videos are kept.
      </Dialog.Description>
    </Dialog.Header>
    <Dialog.Footer>
      <Button variant="outline" size="sm" onclick={() => (deleteTarget = null)}>Cancel</Button>
      <Button variant="destructive" size="sm" disabled={busy[`delete-${deleteTarget?.id}`]} onclick={confirmDeleteScene}>
        <Trash2 class="size-3 mr-1" />{busy[`delete-${deleteTarget?.id}`] ? 'Deleting…' : 'Delete'}
      </Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>

<style>
  @media (max-width: 767px) {
    .story-workspace :global(.scene-list-pane) {
      flex: 1 !important;
    }
    .story-workspace :global(.scene-detail-pane),
    .story-workspace :global([data-slot='resizable-handle']) {
      display: none;
    }
    .story-workspace.show-detail :global(.scene-list-pane) {
      display: none;
    }
    .story-workspace.show-detail :global(.scene-detail-pane) {
      display: block;
      flex: 1 !important;
    }
  }
</style>
