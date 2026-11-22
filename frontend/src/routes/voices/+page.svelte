<script lang="ts">
  import { errorMessage } from '$lib/errors';
  import SpeechPicker from '$lib/voices/SpeechPicker.svelte';
  import { protectDraft } from '$lib/protect-draft';
  import { onMount } from 'svelte';
  import { page } from '$app/state';
  import { get, put, post, del, upload, mediaUrl } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import { Badge } from '$lib/components/ui/badge';
  import { Mic, RefreshCw } from '@lucide/svelte';

  type Profile = { provider: string; model: string; voice_id: string; approved: boolean };
  type Document = { id: string; revision: number; data: Partial<Profile> };
  const providers = ['fish', 'elevenlabs'];
  const providerName = (p: string) => p === 'elevenlabs' ? 'ElevenLabs' : 'Fish Audio';

  let selectedCharacter = $state('');
  let project = $state('');
  let primary = $state('fish');
  let profiles = $state<Document[]>([]);
  // Each character's voice sample file, and a counter so a replaced sample is fetched again.
  let samples = $state<Record<string, string>>({});
  let sampleVersion = $state(0);
  let forms = $state<Record<string, Profile>>({});
  let baselines = $state<Record<string, string>>({});
  let characters = $state<any[]>([]);
  let busy = $state('');
  let error = $state('');
  let notice = $state('');
  let loaded = $state(false);

  const profileFor = (id: string) => profiles.find(p => p.id === id);
  const scoped = (path: string) => `${path}?project_id=${encodeURIComponent(project)}`;
  const changed = (id: string) => baselines[id] !== undefined && JSON.stringify(forms[id]) !== baselines[id];
  protectDraft(() => Object.keys(forms).some(changed), 'You have unsaved voice changes. Discard them and leave?');

  // Voices saved by earlier versions may name a provider that is no longer offered,
  // or carry fields this page does not use. Such a voice has to be chosen again.
  function formFor(id: string): Profile {
    const saved = profileFor(id)?.data;
    if (!saved || !providers.includes(saved.provider ?? '')) return { provider: primary, model: 'auto', voice_id: '', approved: false };
    return { provider: saved.provider!, model: saved.model || 'auto', voice_id: saved.voice_id ?? '', approved: !!saved.approved };
  }
  function resetForm(id: string) { forms[id] = formFor(id); baselines[id] = JSON.stringify(forms[id]); }

  async function refresh(reset = false) {
    const state = await get(project ? scoped('/voice/state') : '/voice/state');
    if (project && state.project_id !== project) throw new Error('Project changed. Reload this page.');
    project = state.project_id;
    primary = state.primary_provider ?? 'fish';
    profiles = state.profiles;
    samples = state.samples ?? {};
    characters = await get('/characters');
    if (selectedCharacter && !characters.some(c => c.id === selectedCharacter)) selectedCharacter = '';
    for (const c of characters) if (reset || !forms[c.id]) resetForm(c.id);
  }
  onMount(() => {
    selectedCharacter = page.url.searchParams.get('character_id') ?? '';
    refresh(true).catch(e => error = e.message).finally(() => loaded = true);
  });

  async function action(key: string, fn: () => Promise<unknown>) {
    if (busy) return;
    busy = key; error = ''; notice = '';
    try { await fn(); } catch (e) { error = errorMessage(e); } finally { busy = ''; }
  }
  async function setPrimary(p: string) {
    await put('/voice/preferences', { primary_provider: p });
    primary = p;
    // Characters with no saved voice and no edits follow the new preference.
    for (const c of characters) if (!profileFor(c.id) && !changed(c.id)) resetForm(c.id);
  }
  async function saveProfile(id: string, name: string) {
    const submitted: Profile = { ...forms[id], voice_id: forms[id].voice_id.trim() };
    const saved: Document = await put(scoped(`/voice/profiles/${id}`), { revision: profileFor(id)?.revision ?? 0, data: submitted });
    profiles = [...profiles.filter(p => p.id !== id), saved];
    resetForm(id);
    notice = `Voice saved for ${name}.`;
  }
  // A voice sample comes from a recording the user uploads, or is spoken by the saved voice.
  async function saveSample(id: string, name: string, file?: File) {
    let form: FormData | undefined;
    if (file) { form = new FormData(); form.append('file', file); }
    const saved = await (form ? upload(scoped(`/voice/samples/${id}`), form) : post(scoped(`/voice/samples/${id}`)));
    samples = { ...samples, [id]: saved.file_path };
    sampleVersion++;
    notice = `Voice sample saved for ${name}.`;
  }
  async function removeSample(id: string, name: string) {
    await del(scoped(`/voice/samples/${id}`));
    const { [id]: _, ...rest } = samples;
    samples = rest;
    notice = `Voice sample removed for ${name}.`;
  }
</script>

<svelte:head><title>Voices · VideoFlow</title></svelte:head>

<div class="workspace">
  <header class="page-head">
    <div><h1><Mic
      size={22} /> Voices</h1><p>Choose a voice for each character. When you add narration in the editor, each character's lines are spoken with the voice saved here.</p></div>
    <Button variant="outline" disabled={!!busy} onclick={() => action('refresh', () => refresh())}><RefreshCw size={15} /> Refresh</Button>
  </header>
  <nav class="links" aria-label="Related pages"><a href="/library?tab=characters">Back to Characters</a><a
    href="/settings#providers">Voice service keys</a></nav>
  {#if error}<div role="alert" class="message error">{error}</div>{/if}
  {#if notice}<div role="status" class="message">{notice}</div>{/if}
  {#if !loaded}<p class="muted">Loading this project's voices…</p>{:else}
  <div class="preferred"><label>Preferred voice service<select
    aria-label="Preferred voice service"
    value={primary}
    disabled={!!busy}
    onchange={e => {
      const p = e.currentTarget.value;
      e.currentTarget.value = primary;
      void action('primary', () => setPrimary(p));
    }}>{#each providers as p}<option value={p}>{providerName(p)}</option>{/each}</select></label><p
      class="hint">Used first when a character has no voice yet. Voices already saved keep their service.</p></div>
  <section aria-labelledby="voice-heading">
    <div class="section-head"><h2 id="voice-heading">Character voices</h2></div>
    {#if !characters.length}<p class="empty">Add a character in <a href="/library?tab=characters">Characters</a>, then choose its voice here.</p>{:else}
    <label class="filter">Character<select
      aria-label="Voice character"
      bind:value={selectedCharacter}><option value="">All characters</option>{#each characters as c}<option
        value={c.id}>{c.name}</option>{/each}</select></label>
    <div class="profile-grid">
      {#each characters.filter(c => !selectedCharacter || c.id === selectedCharacter) as character (character.id)}
        {@const profile = forms[character.id]}
        {@const saved = profileFor(character.id)?.data}
        {#if profile}
        <article class="panel">
          <div
            class="row"><h3>{character.name}</h3>{#if saved?.approved && saved.voice_id && providers.includes(saved.provider ?? '')}<Badge>Voice saved</Badge>{:else}<Badge
            variant="secondary">No voice yet</Badge>{/if}</div>
          <label>Voice service<select
            aria-label={`Voice service for ${character.name}`}
            value={profile.provider}
            onchange={e => {
              profile.provider = e.currentTarget.value;
              profile.model = 'auto';
              profile.voice_id = '';
              profile.approved = false;
            }}>{#each providers as p}<option value={p}>{providerName(p)}</option>{/each}</select></label>
          <SpeechPicker
            provider={profile.provider}
            character={character.id}
            bind:model={profile.model}
            bind:voice={profile.voice_id}
            onchange={() => profile.approved = false} />
          <label>Voice ID<input
            aria-label={`Voice ID for ${character.name}`}
            bind:value={profile.voice_id}
            placeholder="Choose a voice above, or paste its ID"
            oninput={() => profile.approved = false} /></label>
          <label class="check"><input
            type="checkbox"
            bind:checked={profile.approved}
            disabled={!profile.voice_id.trim()} /> Use this voice for {character.name}</label>
          <div class="actions"><Button
            disabled={!!busy || !changed(character.id)}
            onclick={() => action(`profile-${character.id}`, () => saveProfile(character.id, character.name))}>{busy === `profile-${character.id}` ? 'Saving…' : 'Save voice'}</Button>{#if changed(character.id)}<span
              class="hint">Not saved yet</span>{/if}</div>
          <div class="sample">
            <h4>Voice sample for videos</h4>
            <p class="hint">A few seconds of {character.name} speaking. Scene videos made from reference pictures are sent it, on video services that accept voice samples, so {character.name} sounds the same in every clip. Only the first 5 seconds are kept. Use only a voice you have the right to use.</p>
            {#if samples[character.id]}<audio
              controls
              preload="none"
              src={`${mediaUrl(samples[character.id])}?v=${sampleVersion}`}
              aria-label={`Voice sample of ${character.name}`}></audio>{/if}
            <div class="actions">
              <label class="upload"
                class:off={!!busy}>{busy === `sample-${character.id}` ? 'Saving…' : samples[character.id] ? 'Replace recording' : 'Upload a recording'}<input
                type="file"
                accept="audio/*,.mp3,.wav,.m4a,.flac"
                disabled={!!busy}
                aria-label={`Upload a voice sample for ${character.name}`}
                onchange={e => {
                  const file = e.currentTarget.files?.[0];
                  e.currentTarget.value = '';
                  if (file) void action(`sample-${character.id}`, () => saveSample(character.id, character.name, file));
                }} /></label>
              {#if saved?.approved && saved.voice_id}<Button
                variant="outline"
                disabled={!!busy}
                title="Uses a little speech credit"
                onclick={() => action(`sample-${character.id}`, () => saveSample(character.id, character.name))}>Make from saved voice</Button>{/if}
              {#if samples[character.id]}<Button
                variant="ghost"
                disabled={!!busy}
                onclick={() => action(`sample-${character.id}`, () => removeSample(character.id, character.name))}>Remove</Button>{/if}
            </div>
          </div>
        </article>
        {/if}
      {/each}
    </div>
    {/if}
  </section>
  {/if}
</div>

<style>
  .workspace {
    max-width: 1160px;
    margin: 0 auto;
    padding: 32px 28px 80px;
  }
  .page-head,
  .section-head,
  .row,
  .actions,
  .links {
    display: flex;
    align-items: center;
    gap: 14px;
  }
  .page-head {
    justify-content: space-between;
    align-items: flex-start;
  }
  .page-head h1 {
    display: flex;
    align-items: center;
    gap: 12px;
    font-size: 28px;
    font-weight: 650;
    letter-spacing: -0.035em;
    margin: 8px 0;
  }
  .page-head p {
    color: var(--muted-foreground);
    max-width: 650px;
  }
  .links {
    margin: 20px 0 24px;
    font-size: 13px;
  }
  .preferred {
    max-width: 420px;
  }
  section {
    margin-top: 32px;
  }
  .section-head {
    margin-bottom: 8px;
  }
  .section-head h2 {
    font-size: 20px;
    letter-spacing: -0.025em;
  }
  .filter {
    max-width: 320px;
  }
  .profile-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 16px;
    margin-top: 8px;
  }
  .panel {
    border: 1px solid var(--border);
    border-radius: 12px;
    background: var(--card);
    padding: 22px;
  }
  .panel h3 {
    font-size: 16px;
    font-weight: 600;
  }
  .row {
    flex-wrap: wrap;
  }
  .row h3 {
    flex: 1;
  }
  .actions {
    flex-wrap: wrap;
    margin-top: 16px;
  }
  .hint,
  .muted {
    font-size: 11px;
    color: var(--muted-foreground);
    line-height: 1.7;
    overflow-wrap: anywhere;
  }
  .message {
    border: 1px solid var(--border);
    background: var(--muted);
    padding: 12px 16px;
    border-radius: 8px;
    font-size: 13px;
    margin: 14px 0;
  }
  .message.error {
    color: var(--destructive);
    border-color: color-mix(in srgb, var(--destructive) 25%, transparent);
  }
  label {
    display: flex;
    flex-direction: column;
    gap: 6px;
    font-size: 12px;
    margin: 12px 0;
    color: var(--foreground);
  }
  input:not([type='checkbox']),
  select {
    width: 100%;
    min-height: 36px;
    border: 1px solid var(--input);
    background: var(--background);
    color: var(--foreground);
    border-radius: 6px;
    padding: 8px 10px;
    font: inherit;
  }
  .check {
    display: flex;
    flex-direction: row;
    align-items: center;
    gap: 8px;
    line-height: 1.5;
  }
  .check input {
    accent-color: var(--primary);
  }
  a {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    text-decoration: underline;
    text-underline-offset: 4px;
    font-size: 12px;
  }
  .empty {
    border: 1px dashed var(--border);
    border-radius: 10px;
    padding: 24px;
    color: var(--muted-foreground);
    font-size: 13px;
  }
  a:focus-visible,
  input:focus-visible,
  select:focus-visible {
    outline: 2px solid var(--ring);
    outline-offset: 3px;
  }
  @media (max-width: 760px) {
    .workspace {
      padding: 24px 16px 60px;
    }
    .page-head {
      flex-direction: column;
    }
    .profile-grid {
      grid-template-columns: 1fr;
    }
    .page-head h1 {
      font-size: 24px;
    }
  }
  .sample {
    margin-top: 18px;
    padding-top: 16px;
    border-top: 1px solid var(--border);
  }
  .sample h4 {
    font-size: 14px;
    font-weight: 600;
    margin-bottom: 4px;
  }
  .sample audio {
    width: 100%;
    margin-top: 10px;
  }
  .upload {
    display: inline-flex;
    align-items: center;
    min-height: 36px;
    padding: 0 14px;
    border: 1px solid var(--border);
    border-radius: 8px;
    font-size: 14px;
    cursor: pointer;
    margin: 0;
  }
  .upload:hover {
    background: var(--accent);
  }
  .upload:focus-within {
    outline: 2px solid var(--ring);
    outline-offset: 2px;
  }
  .upload.off {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .upload input {
    position: absolute;
    width: 1px;
    height: 1px;
    opacity: 0;
    overflow: hidden;
  }
</style>
