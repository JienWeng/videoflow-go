<script lang="ts">
  import { editSceneHref } from '$lib/editor/links';
  import { untrack } from 'svelte';
  import { get, put } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import type { ProductionPlan, ProductionSettings } from '$lib/api-types';
  let {
    scene,
    newVersion = $bindable(false),
    disabled = false,
    onstate,
    onplan,
    onask
  }: {
    scene: string;
    newVersion?: boolean;
    disabled?: boolean;
    onstate?: (s: { blocking: boolean; loading: boolean }) => void;
    onplan?: (id: string, cost: { calls: number; seconds: number; storyboards: number }) => void;
    onask?: (text: string) => void;
  } = $props();
  const cost = $derived.by(() => {
    const extra = newVersion ? (plan?.units ?? []).filter(u => u.status === 'succeeded') : [];
    const calls = (plan?.calls ?? 0) + extra.length;
    return {
      calls,
      seconds: (plan?.new_seconds ?? 0) + extra.reduce((n, u) => n + u.duration, 0),
      reused: (plan?.reused ?? 0) - extra.length,
      storyboards: calls > 0 && !plan?.storyboard_ready && plan?.video_input !== 'references' ? 1 : 0
    };
  });
  let plan = $state<ProductionPlan | null>(null),
    settings = $state<ProductionSettings | null>(null),
    loading = $state(false),
    saving = $state(false),
    dirty = $state(false),
    error = $state('');
  let seq = 0;
  function report() {
    onstate?.({
      blocking: loading || saving || dirty || !plan || !!plan.blocked || !plan.references_ready,
      loading: loading || saving
    });
  }
  async function load() {
    const token = ++seq;
    loading = true;
    error = '';
    report();
    try {
      const result = await get<ProductionPlan | null>(`/scenes/${encodeURIComponent(scene)}/production`);
      if (token !== seq) return;
      if (!result?.id || !Array.isArray(result.units) || !result.settings)
        throw Error('The render plan is incomplete. Recheck before generating.');
      plan = result;
      settings = structuredClone(result.settings);
      dirty = false;
    } catch (e) {
      if (token === seq) {
        error = String(e);
        plan = null;
      }
    } finally {
      if (token === seq) {
        loading = false;
        report();
      }
    }
  }
  $effect(() => {
    scene;
    untrack(() => {
      newVersion = false;
      void load();
    });
  });
  $effect(() => {
    const id = !dirty && plan ? plan.id : '';
    const quote = { calls: cost.calls, seconds: cost.seconds, storyboards: cost.storyboards };
    untrack(() => onplan?.(id, quote));
  });
  function changed() {
    dirty = true;
    error = '';
    if (settings && settings.audio_mode !== 'native') settings.music_prompt = '';
    report();
  }
  async function save() {
    if (disabled || loading || saving || !plan) return;
    const token = seq;
    saving = true;
    error = '';
    report();
    try {
      const result = await put<ProductionPlan>(`/scenes/${encodeURIComponent(scene)}/production`, {
        updated_at: plan.updated_at,
        settings: JSON.parse(JSON.stringify(settings))
      });
      if (token !== seq) return;
      plan = result;
      settings = structuredClone(result.settings);
      dirty = false;
    } catch (e) {
      if (token === seq) error = String(e);
    } finally {
      if (token === seq) {
        saving = false;
        report();
      }
    }
  }
</script>
<section aria-label="Video production plan" class="space-y-3 rounded-lg border border-border p-3 text-sm">
 <div class="flex items-center justify-between gap-3"><h3 class="font-medium">Video & sound</h3><Button
   type="button"
   size="sm"
   variant="ghost"
   disabled={disabled||loading||saving}
   onclick={load}>Recheck plan</Button></div>
 {#if loading}<p role="status" class="text-muted-foreground">Checking clip limits and reusable videos…</p>{/if}
 {#if plan&&settings}
 <fieldset disabled={disabled||loading||saving} class="space-y-3">
 <label class="block">Sound<select
   aria-label="Video sound"
   class="mt-1 min-h-11 w-full rounded-md border border-input bg-background p-2"
   value={settings.audio_mode}
   onchange={e=>{settings!.audio_mode=e.currentTarget.value;changed()}}>
 {#if plan.audio_supported}<option
   value="native">Sound made with the clip · voices, ambience & effects</option>{/if}<option value="silent">No sound</option></select></label>
 <p class="text-xs text-muted-foreground">{settings.audio_mode === 'native'
     ? 'The video model makes the sound together with the picture. Exact voices, wording and cut timing are not guaranteed.'
     : 'The clip has no voices, ambience or music. You can still add voices later in Edit video → Audio → Voice from script.'}</p>{#if settings.audio_mode==='silent'&&plan.lines?.length}<p
       role="note"
       class="text-xs text-amber-600 dark:text-amber-400">This scene has {plan.lines.length} spoken line{plan.lines.length===1?'':'s'} that won't be heard. Choose Sound made with the clip to hear them in the video.</p>{/if}
 <label class="block">Pictures<select
   aria-label="How clips use the scene's pictures"
   class="mt-1 min-h-11 w-full rounded-md border border-input bg-background p-2"
   value={settings.video_input??''}
   onchange={e=>{settings!.video_input=e.currentTarget.value||undefined;changed()}}><option
     value="">Automatic · connected pictures when the video model takes them</option><option
     value="references">Connected pictures as references</option><option value="first_frame">Start from a storyboard picture</option></select></label>
 {#if plan.video_input==='references'&&plan.pictures?.length}<p
   class="text-xs text-muted-foreground">Sent with each clip that shows them: {plan.pictures
       .map(
         (p, i) =>
           `@Image ${i + 1} ${p.name} (${p.kind === 'character' ? (p.sheet ? 'character sheet' : 'character') : p.kind})`
       )
       .join(' · ')}. No storyboard picture is made.</p>{:else}<p
   class="text-xs text-muted-foreground">Each clip starts from a storyboard picture of its opening{settings.video_input !== 'first_frame' && plan.video_input === 'first_frame'
       ? ' (clips already made this way are kept; choose Connected pictures to remake them from references)'
       : ''}.</p>{/if}
 <details><summary class="min-h-11 cursor-pointer py-2">Clips & music options</summary><div class="space-y-3 pb-2">
 <label class="block">Clip strategy<select
   aria-label="Clip strategy"
   class="mt-1 min-h-11 w-full rounded-md border border-input bg-background p-2"
   bind:value={settings.strategy}
   onchange={()=>{settings!.dialogue_shots={};changed()}}><option
     value="auto">Automatic · group consecutive shots</option><option value="shots">Separate clip for each shot</option></select></label>
 {#if settings.audio_mode==='native'}<label class="block">Music direction <span class="text-muted-foreground">(optional)</span><textarea
   aria-label="Native music direction"
   rows="2"
   maxlength="1500"
   class="mt-1 w-full rounded-md border border-input bg-background p-2"
   bind:value={settings.music_prompt}
   oninput={changed}
   placeholder="Gentle piano that warms as Pip finds Mama"></textarea></label><p
     class="text-xs text-muted-foreground">Leave empty for dialogue and ambience only. For music spanning several clips, add one continuous track in the editor.</p>{/if}
 </div></details>
 {#if plan.units.length>1&&settings.audio_mode==='native'&&plan.lines?.length}<div class="space-y-2"><p
   class="text-xs text-muted-foreground">Assign each line to one clip so it is not repeated in every clip. Save the strategy first if you changed the grouping.</p>{#each plan.lines as line (line.id)}<label
   class="block text-xs">{line.text}<select
   aria-label={`Clip for dialogue ${line.id}`}
   class="mt-1 min-h-11 w-full rounded-md border border-input bg-background p-2"
   value={settings.dialogue_shots?.[line.id]??''}
   onchange={e => {
     settings!.dialogue_shots = { ...settings!.dialogue_shots, [line.id]: e.currentTarget.value };
     changed();
   }}><option value="">Choose clip…</option>{#each plan.units as unit}<option
     value={unit.shot_ids[0]}>Clip {unit.index+1} · {unit.start}–{unit.start+unit.length}s</option>{/each}</select></label>{/each}</div>{/if}
 {#if dirty}<div class="flex flex-wrap items-center gap-2"><Button
   type="button"
   size="sm"
   disabled={saving}
   onclick={save}>{saving?'Saving…':'Save video options'}</Button><span class="text-xs text-muted-foreground">Save before generating</span></div>{/if}
 </fieldset>
 <label class="flex items-start gap-2 text-xs"><input
   type="checkbox"
   class="mt-1"
   checked={newVersion}
   onchange={e=>{newVersion=e.currentTarget.checked;report()}}
   disabled={disabled||saving||loading}/>Generate another paid version of completed clips. Active clips are always reused.</label><div
   role="status"
   aria-label="Generation cost summary"
   class="space-y-1 border-t border-border pt-3"><p
     class="font-medium">{plan.units.length} clip{plan.units.length===1?'':'s'} · {plan.edited_seconds}s final edit</p><p>{cost.calls} new paid video call{cost.calls===1?'':'s'} · {cost.seconds}s to generate · {cost.reused} existing clip{cost.reused===1?'':'s'} reused</p><p
     class="text-xs text-muted-foreground">Video model: {plan.model} via {plan.provider}. {plan.generated_seconds > plan.edited_seconds
         ? `${plan.generated_seconds - plan.edited_seconds}s of model minimum-duration padding will be trimmed in the editor. `
         : ''}{cost.storyboards
         ? 'A missing or outdated storyboard may also use image-generation quota.'
         : cost.calls
           ? 'Current storyboard reused.'
           : 'Existing videos reused without image generation.'} Provider credits apply; no currency estimate is available.</p></div>
 {#if plan.blocked}<p role="alert" class="text-xs text-destructive">{plan.blocked}</p>{/if}
 {#if !plan.references_ready}<p role="alert"
   class="text-xs text-destructive">Some character or scene pictures are missing or excluded. Fix them before making this video; saved work is kept.</p>{#if onask}<Button
     type="button"
     size="sm"
     variant="outline"
     disabled={disabled}
     onclick={() =>
       onask?.(
         'Help fix the missing or excluded character and scene pictures for this scene. Keep the pictures that are usable.'
       )}>Fix pictures in chat</Button>{/if}{/if}
 <Button type="button" size="sm" variant="outline" href={editSceneHref(scene)}>Trim clips & add music</Button>
 {/if}
 {#if error}<p role="alert" class="text-xs text-destructive">{error}</p>{/if}
</section>
