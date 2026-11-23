<script lang="ts">
  import { errorMessage } from '../errors';
  import { get, post } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import Mic from '@lucide/svelte/icons/mic';
  import type { Clip, VoiceLine as Item, VoiceoverResult, VoiceoverStatus, VoiceSound } from '$lib/composition/types';
  import type { ProviderInfo } from '$lib/api-types';
  import { canSpeedUp, voiceoverSummary } from './speech.mjs';
  let {
    clips,
    disabled = false,
    hasVoice = false,
    playhead = 0,
    onbusy,
    onvoices
  }: {
    clips: Clip[];
    disabled?: boolean;
    hasVoice?: boolean;
    playhead?: number;
    onbusy: (busy: boolean) => void;
    onvoices: (items: Item[], sound: VoiceSound, replace: boolean) => void | Promise<void>;
  } = $props();
  let typed = $state('');
  const STORE = 'videoflow.voiceover.narrator';
  let provider = $state(''),
    connected = $state<string[] | null>(null),
    voice = $state(''),
    voices = $state<{ id: string; name: string; description?: string }[]>([]),
    loadingVoices = $state(false),
    working = $state(false),
    lower = $state(true),
    error = $state(''),
    note = $state(''),
    // keep: the clips' own voices speak; replace: they are muted and voice lines speak
    // when the characters say the lines in the clips.
    mode = $state<'keep' | 'replace'>('replace'),
    chosen = $state(false),
    status = $state<VoiceoverStatus | null>(null),
    offerSpeedUp = $state(false);
  const sceneClips = $derived(clips.filter(c => String(c.asset_id).startsWith('output-')));
  const sceneKey = $derived(sceneClips.map(c => c.id + ':' + c.asset_id).join(','));
  // Which clips speak with their own voices, and whether the speech check heard them
  // say their lines. Free: it reads stored checks.
  let statusSeq = 0;
  $effect(() => {
    const key = sceneKey;
    const seq = ++statusSeq;
    if (!key) {
      status = null;
      return;
    }
    post<VoiceoverStatus>('/compositions/voiceover/status', { clips: sceneClips })
      .then(r => {
        if (seq !== statusSeq) return;
        status = r;
        if (!chosen) mode = r.recommended;
      })
      .catch(() => {
        if (seq === statusSeq) status = null;
      });
  });
  const nativeCount = $derived(status?.native.length ?? 0);
  const statusLine = $derived(
    !status || !nativeCount
      ? ''
      : status.verified === nativeCount
        ? `The speech check heard ${nativeCount === 1 ? 'this clip' : `all ${nativeCount} clips`} say their lines.`
        : status.flagged
          ? `The speech check flagged ${status.flagged} of ${nativeCount} clip${nativeCount === 1 ? '' : 's'}: ${status.native.find(n => n.check === 'flagged')?.finding ?? 'what is said differs from the script'}`
          : `${status.unchecked} of ${nativeCount} clip${nativeCount === 1 ? ' has' : 's have'} not had a speech check yet (Self-check → Speech check on the clip).`
  );
  const LABEL: Record<string, string> = { elevenlabs: 'ElevenLabs', fish: 'Fish Audio', gemini: 'Google Gemini' };
  let saved: { provider?: string; voice?: string } = {};
  try {
    saved = JSON.parse(localStorage.getItem(STORE) || 'null') ?? {};
  } catch {
    /* Defaults are fine. */
  }
  // Only offer providers that can actually speak (have a saved key).
  get<ProviderInfo[]>('/settings/providers')
    .then(list => {
      connected = ['elevenlabs', 'fish', 'gemini'].filter(n => list.some(p => p.name === n && p.configured));
      provider = connected.includes(saved.provider ?? '') ? saved.provider! : (connected[0] ?? '');
      if (provider === saved.provider && saved.voice) voice = saved.voice;
    })
    .catch(() => {
      connected = ['elevenlabs', 'fish', 'gemini'];
      provider = saved.provider ?? 'elevenlabs';
    });
  let sequence = 0;
  async function loadVoices(p: string) {
    if (!p) return;
    const seq = ++sequence;
    loadingVoices = true;
    error = '';
    try {
      const r = await get<{ voices?: { id: string; name: string; description?: string }[] }>(
        `/voice/catalog/${p}/voices?language=en`
      );
      if (seq !== sequence) return;
      voices = r.voices ?? [];
      if (voice && !voices.some(v => v.id === voice)) voices = [{ id: voice, name: 'Saved voice' }, ...voices];
      if (!voice && voices.length) voice = (voices.find(v => /narrat/i.test(v.name)) ?? voices[0]).id;
    } catch (e) {
      if (seq === sequence) {
        voices = [];
        error = 'Could not list ' + LABEL[p] + ' voices. ' + errorMessage(e);
      }
    } finally {
      if (seq === sequence) loadingVoices = false;
    }
  }
  $effect(() => {
    void loadVoices(provider);
  });
  function remember() {
    try {
      localStorage.setItem(STORE, JSON.stringify({ provider, voice }));
    } catch {
      /* Not critical. */
    }
  }
  async function generate(speedUp = false) {
    if (working || disabled) return;
    if (!speedUp && hasVoice && !confirm('Replace all voice lines on the Audio track, including typed ones? You can undo.')) return;
    working = true;
    onbusy(true);
    error = '';
    note = '';
    offerSpeedUp = false;
    remember();
    try {
      const r = await post<VoiceoverResult>('/compositions/voiceover', {
        clips,
        narrator: voice ? { provider, voice_id: voice } : {},
        mode,
        speed_up: speedUp
      });
      const items = r.items ?? [];
      const skipped = r.skipped ?? [];
      if (!items.length && !(r.mode === 'keep' && r.kept_native)) {
        note = skipped[0]?.reason ?? 'No script lines found. Add rendered scene videos from Media first.';
        return;
      }
      await onvoices(items, { mode, native: r.native_clip_ids ?? [], mute: r.mute_clip_ids ?? [], lowerMusic: lower }, true);
      note = voiceoverSummary(r) || 'Done.';
      if (!r.too_long?.length && !(r.overrun > 0.5)) note += ' Drag them on the Audio track to fine-tune.';
      offerSpeedUp = !speedUp && canSpeedUp(r);
    } catch (e) {
      error = errorMessage(e);
    } finally {
      working = false;
      onbusy(false);
    }
  }
  // A line that is not in the script: spoken by the narrator voice at the playhead.
  async function speakTyped() {
    const text = typed.trim();
    if (!text || working || disabled || !voice) return;
    working = true;
    onbusy(true);
    error = '';
    note = '';
    remember();
    try {
      const r = await post<VoiceoverResult>('/compositions/voiceover', {
        clips: [],
        narrator: { provider, voice_id: voice },
        lines: [{ text, start: Math.max(0, Math.round(playhead * 1000) / 1000) }]
      });
      const items = r.items ?? [];
      if (!items.length) {
        note = (r.skipped ?? [])[0]?.reason ?? 'Nothing was spoken.';
        return;
      }
      await onvoices(items, { mode, native: [], mute: [], lowerMusic: lower }, false);
      typed = '';
      note =
        `Added “${text.length > 40 ? text.slice(0, 40) + '…' : text}” at ${playhead.toFixed(1)}s` +
        (r.generated ? '' : ' (reused, no charge)') +
        '.';
    } catch (e) {
      error = errorMessage(e);
    } finally {
      working = false;
      onbusy(false);
    }
  }
</script>
<section aria-label="Voice from script" class="space-y-2 rounded-md border border-border p-3">
 <p class="flex items-center gap-2 font-medium"><Mic class="size-4"/>Voice from script</p>
 <p class="text-xs text-muted-foreground">Speaks each scene's script lines. Characters with a saved voice in Voices use it; everyone else uses this narrator voice.</p>
 {#if nativeCount}<fieldset class="space-y-1.5 rounded-md bg-muted/40 p-2 text-xs"><legend class="sr-only">Voices in the clips</legend><p
   class="font-medium">{nativeCount === 1 ? 'A clip was' : `${nativeCount} clips were`} made with their own voices</p><label
   class="flex items-start gap-2"><input
   type="radio"
   name="voice-mode"
   class="mt-0.5"
   value="keep"
   bind:group={mode}
   onchange={()=>chosen=true}
   disabled={working}/><span><span class="font-medium">Keep the video's own voices</span> · only clips without sound get voice lines.</span></label><label
   class="flex items-start gap-2"><input
   type="radio"
   name="voice-mode"
   class="mt-0.5"
   value="replace"
   bind:group={mode}
   onchange={()=>chosen=true}
   disabled={working}/><span><span class="font-medium">Replace with character voices</span> · mutes those clips' own sound (their music and ambience too) and speaks each line when the character says it in the clip.</span></label>{#if statusLine}<p
   class="text-muted-foreground">{statusLine}</p>{/if}</fieldset>{/if}
 {#if connected&&!connected.length}<div
   role="note"
   class="space-y-2 rounded-md bg-muted/60 p-2 text-xs"><p>Connect ElevenLabs or Fish Audio to make voices. It takes one API key.</p><Button
   size="sm"
   variant="outline"
   href="/settings#providers">Connect a voice provider</Button></div>{:else}
 <div class="grid grid-cols-[auto_1fr] gap-2">
  <select
    aria-label="Speech provider"
    class="min-h-9 rounded-md border border-input bg-background px-2 text-sm"
    bind:value={provider}
    onchange={()=>{voice='';remember()}}
    disabled={working}>{#each connected??[] as name}<option value={name}>{LABEL[name]}</option>{/each}</select>
  <select
    aria-label="Narrator voice"
    class="min-h-9 min-w-0 rounded-md border border-input bg-background px-2 text-sm"
    bind:value={voice}
    onchange={remember}
    disabled={working||loadingVoices}>{#if loadingVoices}<option
      value={voice}>Loading voices…</option>{:else if !voices.length}<option
      value="">No voices available</option>{/if}{#each voices as v (v.id)}<option value={v.id}>{v.name||v.id}</option>{/each}</select>
 </div>
 <label class="flex items-start gap-2 text-xs"><input
   type="checkbox"
   class="mt-0.5"
   bind:checked={lower}
   disabled={working}/>Lower music under the voices</label>
 <Button
   class="w-full"
   disabled={disabled||working||!sceneClips.length}
   onclick={()=>generate()}><Mic class="size-4"/>{working?'Making voices…':hasVoice?'Redo voices from script':'Add voices from script'}</Button>
 {#if offerSpeedUp}<Button
   class="w-full"
   variant="outline"
   size="sm"
   disabled={disabled||working}
   onclick={()=>generate(true)}>Speed up the long lines (at most 1.15×)</Button>{/if}
 <p class="text-xs text-muted-foreground">{sceneClips.length
     ? 'Uses speech credits for new lines; unchanged lines are reused free.'
     : 'Add a rendered scene video to the timeline to voice its script.'}</p>
 <form
   class="flex gap-2"
   onsubmit={e=>{e.preventDefault();void speakTyped()}}><input
   aria-label="Line to speak"
   class="min-h-9 min-w-0 flex-1 rounded-md border border-input bg-background px-2 text-sm"
   placeholder="Or type any line…"
   maxlength="500"
   bind:value={typed}
   disabled={working}/><Button
   type="submit"
   size="sm"
   variant="outline"
   disabled={disabled||working||!typed.trim()||!voice}>Say at {playhead.toFixed(1)}s</Button></form>
 {/if}
 {#if note}<p role="status" class="text-xs">{note}</p>{/if}
 {#if error}<p role="alert" class="text-xs text-destructive">{error}</p>{/if}
</section>
