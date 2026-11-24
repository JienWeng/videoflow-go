<script lang="ts">
  import { untrack } from 'svelte';
  import { toast } from 'svelte-sonner';
  import { ChevronDown, ChevronUp, Plus, Trash2 } from '@lucide/svelte';
  import { get, post, put } from '$lib/api';
  import type { Character, Scene, SceneSpeech, ScriptFit } from '$lib/api-types';
  import { errorMessage } from '$lib/errors';
  import { randomId } from '$lib/ids';
  import { protectDraft } from '$lib/protect-draft';
  import { Badge } from '$lib/components/ui/badge';
  import { Button } from '$lib/components/ui/button';
  import ScriptFitReview from './ScriptFitReview.svelte';
  import { budgetText, fitBar, moveLine, sceneSpokenLines, secondsText, speakerLabel, type SpokenLine } from './spoken.mjs';

  // What the scene's characters and narrator say, as the story saved it. The panel loads
  // the scene itself so the story workspace and the canvas panel show the same lines.
  let {
    sceneId,
    version = '',
    characters: given = undefined,
    onchanged = undefined
  }: {
    sceneId: string;
    /** The scene's updated_at as the parent knows it; a change reloads unedited lines. */
    version?: string;
    characters?: Character[];
    onchanged?: () => void | Promise<void>;
  } = $props();

  let loadedCharacters = $state<Character[]>([]);
  const characters = $derived(given ?? loadedCharacters);
  let cast = $state<string[]>([]);
  let updatedAt = $state('');
  let lines = $state<SpokenLine[]>([]);
  let baseline = $state('[]');
  let loading = $state(false);
  let saving = $state(false);
  let error = $state('');
  const dirty = $derived(JSON.stringify(lines) !== baseline);
  protectDraft(() => dirty && !saving, 'The spoken script has unsaved changes. Discard them and leave?');

  // A speaker is offered when they are in the scene's cast, or already speak a line.
  const speakers = $derived(
    characters.filter(c => !cast.length || cast.includes(c.id) || lines.some(l => l.character_id === c.id))
  );

  function take(scene: Scene) {
    cast = scene.character_ids_json ?? [];
    updatedAt = scene.updated_at;
    lines = sceneSpokenLines(scene);
    baseline = JSON.stringify(lines);
  }

  let loadedFor = '';
  async function load(id: string) {
    loading = true;
    error = '';
    try {
      const [scene, chars] = await Promise.all([
        get<Scene>(`/scenes/${encodeURIComponent(id)}`),
        given ? Promise.resolve(null) : get<Character[]>('/characters')
      ]);
      if (id !== sceneId) return;
      if (chars) loadedCharacters = chars;
      take(scene);
      loadedFor = id;
    } catch (e) {
      error = errorMessage(e, 'Could not load the spoken script.');
    } finally {
      loading = false;
    }
  }
  $effect(() => {
    const id = sceneId;
    const v = version;
    untrack(() => {
      // Another view saved the scene: reload, unless that would throw away typing here.
      if (id !== loadedFor || (v && v !== updatedAt && !dirty)) void load(id);
    });
  });

  // The speech budget of the lines as they are now (saved or not): per shot, how long
  // its lines take against its seconds, and per clip whether everything can be said.
  let fit = $state<SceneSpeech | null>(null);
  let fitSeq = 0;
  $effect(() => {
    const id = sceneId;
    const draft = JSON.stringify(lines);
    if (!loadedFor) return;
    const seq = ++fitSeq;
    const timer = setTimeout(async () => {
      try {
        const next = await post<SceneSpeech>(`/scenes/${encodeURIComponent(id)}/speech-fit`, { lines: JSON.parse(draft) });
        if (seq === fitSeq) fit = next;
      } catch {
        if (seq === fitSeq) fit = null; // the budget is advice; the script can still be edited and saved
      }
    }, 300);
    return () => clearTimeout(timer);
  });
  const shotChoices = $derived((fit?.shots ?? []).filter(s => s.shot_id));
  const lineFit = $derived(new Map((fit?.shots ?? []).flatMap(s => s.lines.map(l => [l.id, { ...l, shot: s.number }] as const))));
  const toneClass = {
    ok: 'bg-emerald-500',
    tight: 'bg-amber-500',
    too_long: 'bg-destructive',
    empty: 'bg-muted-foreground/30'
  } as const;
  const textTone = {
    ok: 'text-muted-foreground',
    tight: 'text-amber-700 dark:text-amber-300',
    too_long: 'text-destructive',
    empty: 'text-muted-foreground'
  } as const;
  function setShot(line: SpokenLine, id: string) {
    if (id) line.shot_id = id;
    else delete line.shot_id;
    delete line.start_hint; // a start inside the old shot means nothing in the new one
  }

  // Fit script to clips: one turn of the local AI session rewrites only what is said, so
  // each shot's lines fill most of its seconds. Shown before and after; saved on a click.
  let proposal = $state<ScriptFit | null>(null);
  let fitting = $state(false);
  const needsFit = $derived(
    !!fit && (fit.verdict === 'too_long' || fit.shots.some(s => s.verdict === 'too_long'))
  );
  async function fitScript() {
    if (fitting || dirty) return;
    fitting = true;
    error = '';
    try {
      proposal = await post<ScriptFit>(`/scenes/${encodeURIComponent(sceneId)}/spoken-lines/fit`);
    } catch (e) {
      error = errorMessage(e, 'Could not fit the script.');
    } finally {
      fitting = false;
    }
  }
  async function fitted() {
    proposal = null;
    await load(sceneId);
    await onchanged?.();
  }

  function addLine() {
    const last = lines[lines.length - 1];
    lines = [
      ...lines,
      {
        id: `line_${randomId().slice(0, 8)}`,
        character_id: last?.character_id ?? '',
        language: last?.language || 'English',
        text: '',
        visible: last?.visible ?? false
      }
    ];
  }
  function setSpeaker(line: SpokenLine, id: string) {
    line.character_id = id;
    if (!id) line.visible = false; // the narrator is heard, never seen speaking
  }

  async function save() {
    if (saving || !dirty) return;
    saving = true;
    error = '';
    try {
      const scene = await put<Scene>(`/scenes/${encodeURIComponent(sceneId)}/spoken-lines`, {
        lines,
        expected_updated_at: updatedAt
      });
      take(scene);
      toast.success('Spoken script saved. The scene video will be made again from these lines.');
      await onchanged?.();
    } catch (e) {
      error = errorMessage(e, 'Could not save the spoken script.');
    } finally {
      saving = false;
    }
  }
  function discard() {
    lines = JSON.parse(baseline);
    error = '';
  }
</script>

<section class="my-3 rounded-lg border border-border p-3" aria-labelledby="spoken-{sceneId}">
  <div class="flex flex-wrap items-center gap-2">
    <h3 id="spoken-{sceneId}" class="text-sm font-medium">Spoken script</h3>
    <Badge variant="secondary">{lines.length ? `${lines.length} ${lines.length === 1 ? 'line' : 'lines'}` : 'Silent'}</Badge>
    {#if dirty}
      <Badge variant="outline" class="border-amber-500/50 text-amber-800 dark:text-amber-300">Unsaved</Badge>
    {/if}
    <div class="ml-auto flex items-center gap-2">
      {#if dirty}<Button variant="ghost" size="sm" disabled={saving} onclick={discard}>Cancel</Button>{/if}
      <Button size="sm" variant={dirty ? 'default' : 'secondary'} disabled={saving || !dirty || loading} onclick={save}>
        {saving ? 'Saving…' : dirty ? 'Save script *' : 'Saved'}
      </Button>
    </div>
  </div>

  {#if fit && fit.lines > 0}
    <div class="mt-3 space-y-2 rounded-md bg-muted/40 p-2" aria-label="Speech budget">
      {#each fit.clips.filter(c => c.summary) as clip (clip.index)}
        <p class="text-xs font-medium {textTone[fitBar(clip.used, clip.seconds, clip.verdict).tone]}" role="status">{clip.summary}</p>
        {#if clip.room}<p class="text-xs text-muted-foreground">{clip.room}</p>{/if}
      {/each}
      {#each fit.shots as shot (shot.shot_id || 'scene')}
        {@const bar = fitBar(shot.used, shot.seconds, shot.verdict)}
        <div class="text-xs">
          <div class="flex flex-wrap items-baseline gap-x-2">
            <span class="font-medium">{shot.number ? `Shot ${shot.number}` : 'Whole scene'} · {secondsText(shot.seconds)}</span>
            <span class={textTone[bar.tone]}>{shot.lines.length ? bar.label : 'no lines'}{bar.tone === 'too_long' ? ' — too long' : bar.tone === 'tight' ? ' — tight' : ''}</span>
            {#if budgetText(shot.budget)}<span class="ml-auto text-muted-foreground">aim for {budgetText(shot.budget)}</span>{/if}
          </div>
          <div class="mt-1 h-1.5 w-full overflow-hidden rounded-full bg-muted" role="meter" aria-label="Speech in shot {shot.number || 1}" aria-valuemin="0" aria-valuemax={shot.seconds} aria-valuenow={Math.min(shot.used, shot.seconds)} aria-valuetext={bar.label}>
            <div class="h-full rounded-full {toneClass[bar.tone]}" style="width: {bar.percent}%"></div>
          </div>
        </div>
      {/each}
      <p class="text-[11px] text-muted-foreground">Estimated at a calm speaking pace. Lines without a chosen shot are placed by time.</p>
      <div class="flex flex-wrap items-center gap-2">
        <Button size="sm" variant={needsFit ? 'default' : 'outline'} disabled={fitting || saving || dirty} onclick={fitScript}
          title={dirty ? 'Save or cancel your edits first' : 'Rewrite only what is said so each shot can say it in time (no credits)'}>
          {fitting ? 'Fitting…' : 'Fit script to clips'}
        </Button>
        {#if dirty}<span class="text-[11px] text-muted-foreground">Save or cancel your edits first.</span>{/if}
      </div>
    </div>
  {/if}
  {#if proposal}
    <div class="mt-3"><ScriptFitReview fits={[proposal]} {characters} onapplied={fitted} ondismiss={() => (proposal = null)} /></div>
  {/if}

  {#if loading && !lines.length}
    <p class="mt-2 text-xs text-muted-foreground">Loading spoken script…</p>
  {:else if !lines.length}
    <p class="mt-2 text-xs text-muted-foreground">No spoken lines — this scene is silent.</p>
  {:else}
    <ol class="mt-3 space-y-3">
      {#each lines as line, i (i)}
        <li class="rounded-md border border-border/70 p-2">
          <div class="flex flex-wrap items-center gap-2">
            <span class="w-5 text-right text-xs text-muted-foreground">{i + 1}</span>
            <label class="sr-only" for="speaker-{sceneId}-{i}">Speaker for line {i + 1}</label>
            <select
              id="speaker-{sceneId}-{i}"
              class="min-w-0 flex-1 basis-32 rounded-md border border-input bg-background px-2 py-1 text-xs"
              value={line.character_id}
              onchange={e => setSpeaker(line, (e.currentTarget as HTMLSelectElement).value)}>
              <option value="">Narrator</option>
              {#each speakers as c (c.id)}<option value={c.id}>{c.name}</option>{/each}
              {#if line.character_id && !speakers.some(c => c.id === line.character_id)}
                <option value={line.character_id}>{speakerLabel(line, characters)}</option>
              {/if}
            </select>
            <label class="sr-only" for="language-{sceneId}-{i}">Language for line {i + 1}</label>
            <input
              id="language-{sceneId}-{i}"
              class="w-24 rounded-md border border-input bg-background px-2 py-1 text-xs"
              placeholder="Language"
              bind:value={line.language} />
            <label
              class="inline-flex items-center gap-1 text-xs text-muted-foreground"
              title={line.character_id ? 'The character is seen saying this line' : 'The narrator is heard, not seen'}>
              <input type="checkbox" class="w-auto" disabled={!line.character_id} bind:checked={line.visible} />
              On screen
            </label>
            {#if shotChoices.length}
              {@const est = lineFit.get(line.id)}
              <label class="sr-only" for="shot-{sceneId}-{i}">Shot for line {i + 1}</label>
              <select
                id="shot-{sceneId}-{i}"
                class="rounded-md border border-input bg-background px-2 py-1 text-xs"
                value={line.shot_id && shotChoices.some(s => s.shot_id === line.shot_id) ? line.shot_id : ''}
                onchange={e => setShot(line, (e.currentTarget as HTMLSelectElement).value)}>
                <option value="">{est && !line.shot_id ? `By time (shot ${est.shot})` : 'By time'}</option>
                {#each shotChoices as s (s.shot_id)}<option value={s.shot_id}>Shot {s.number} · {secondsText(s.seconds)}</option>{/each}
              </select>
            {/if}
            {#if lineFit.get(line.id)}
              {@const est = lineFit.get(line.id)!}
              <span class="text-xs {textTone[est.verdict === 'too_long' ? 'too_long' : est.verdict === 'tight' ? 'tight' : 'ok']}" title="Estimated time to say this line">≈{secondsText(est.seconds)}</span>
            {/if}
            <div class="ml-auto flex items-center">
              <Button variant="ghost" size="icon" class="size-7" title="Move up" disabled={i === 0}
                onclick={() => (lines = moveLine(lines, i, -1))}><ChevronUp class="size-3.5" /></Button>
              <Button variant="ghost" size="icon" class="size-7" title="Move down" disabled={i === lines.length - 1}
                onclick={() => (lines = moveLine(lines, i, 1))}><ChevronDown class="size-3.5" /></Button>
              <Button variant="ghost" size="icon" class="size-7 text-destructive hover:text-destructive" title="Delete line"
                onclick={() => (lines = lines.filter((_, j) => j !== i))}><Trash2 class="size-3.5" /></Button>
            </div>
          </div>
          <label class="sr-only" for="text-{sceneId}-{i}">What {speakerLabel(line, characters)} says</label>
          <textarea
            id="text-{sceneId}-{i}"
            class="mt-2 min-h-[46px] w-full resize-y rounded border border-input bg-background px-2 py-1 text-sm"
            placeholder="What is said"
            maxlength={1000}
            bind:value={line.text}></textarea>
        </li>
      {/each}
    </ol>
  {/if}
  <Button variant="outline" size="sm" class="mt-3" disabled={loading || saving} onclick={addLine}>
    <Plus class="mr-1 size-3" />Add line
  </Button>
  {#if error}<p class="mt-2 text-xs text-destructive" role="alert">{error}</p>{/if}
</section>
