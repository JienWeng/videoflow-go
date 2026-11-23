<script lang="ts">
  import { toast } from 'svelte-sonner';
  import { put } from '$lib/api';
  import type { SavedSpokenLine, SceneSpeech, ScriptFit } from '$lib/api-types';
  import { errorMessage } from '$lib/errors';
  import { Button } from '$lib/components/ui/button';
  import { speakerLabel } from './spoken.mjs';

  // The spoken lines of one or more scenes before and after Fit script to clips. Nothing
  // is saved until Use fitted script is pressed.
  let {
    fits,
    characters = [],
    onapplied = undefined,
    ondismiss = undefined
  }: {
    fits: ScriptFit[];
    characters?: readonly { id: string; name?: string }[];
    onapplied?: (sceneIds: string[]) => void | Promise<void>;
    ondismiss?: () => void;
  } = $props();

  let saving = $state(false);
  let error = $state('');

  function shotOf(fit: SceneSpeech, line: SavedSpokenLine) {
    const shot = fit.shots.find(s => s.lines.some(l => l.id === line.id));
    return shot?.number ? `Shot ${shot.number}` : '';
  }
  const tone = (verdict: string) =>
    verdict === 'too_long' ? 'text-destructive' : verdict === 'tight' ? 'text-amber-700 dark:text-amber-300' : 'text-emerald-700 dark:text-emerald-300';

  async function apply() {
    if (saving) return;
    saving = true;
    error = '';
    const done: string[] = [];
    try {
      for (const fit of fits) {
        await put(`/scenes/${encodeURIComponent(fit.scene_id)}/spoken-lines`, {
          lines: fit.after,
          expected_updated_at: fit.expected_updated_at
        });
        done.push(fit.scene_id);
      }
      toast.success(fits.length === 1 ? 'Fitted script saved.' : `Fitted script saved for ${fits.length} scenes.`);
    } catch (e) {
      error = errorMessage(e, 'Could not save the fitted script.');
    } finally {
      saving = false;
      if (done.length) await onapplied?.(done);
    }
  }
</script>

<div class="space-y-3 rounded-md border border-border p-3" aria-label="Fitted script">
  {#each fits as fit (fit.scene_id)}
    <div class="space-y-2">
      {#if fits.length > 1}<p class="text-sm font-medium">{fit.before_fit.title}</p>{/if}
      <div class="grid gap-3 sm:grid-cols-2">
        <div>
          <p class="text-xs font-medium text-muted-foreground">Now</p>
          <p class="text-xs {tone(fit.before_fit.verdict)}">{fit.before_fit.summary}</p>
          <ol class="mt-1 space-y-1 text-sm">
            {#each fit.before as line (line.id)}
              <li><span class="text-xs text-muted-foreground">{shotOf(fit.before_fit, line)} {speakerLabel(line, characters)}:</span> {line.text}</li>
            {/each}
          </ol>
        </div>
        <div>
          <p class="text-xs font-medium text-muted-foreground">Fitted</p>
          <p class="text-xs {tone(fit.after_fit.verdict)}">{fit.after_fit.summary}</p>
          <ol class="mt-1 space-y-1 text-sm">
            {#each fit.after as line (line.id)}
              <li><span class="text-xs text-muted-foreground">{shotOf(fit.after_fit, line)} {speakerLabel(line, characters)}:</span> {line.text}</li>
            {/each}
          </ol>
        </div>
      </div>
    </div>
  {/each}
  <p class="text-xs text-muted-foreground">Only what is said changes. Saving makes the scene's clips again on the next render.</p>
  <div class="flex flex-wrap gap-2">
    <Button size="sm" disabled={saving} onclick={apply}>{saving ? 'Saving…' : 'Use fitted script'}</Button>
    {#if ondismiss}<Button size="sm" variant="ghost" disabled={saving} onclick={ondismiss}>Keep current script</Button>{/if}
  </div>
  {#if error}<p class="text-xs text-destructive" role="alert">{error}</p>{/if}
</div>
