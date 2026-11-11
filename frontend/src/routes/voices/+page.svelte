<script lang="ts">
  import { onMount } from 'svelte';
  import { API_BASE, get, post, put, upload, mediaUrl, isImage } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import { Badge } from '$lib/components/ui/badge';
  import { Mic, RefreshCw, Plus, Trash2, Play, FileText, ArrowRight } from '@lucide/svelte';

  type Profile = { character_id?: string; mode: string; sample_asset_id: string; voice_id: string; approved: boolean };
  type Line = { id: string; character_id: string; language: string; text: string; visible: boolean };
  type Document<T> = { id: string; revision: number; data: T };
  type VoiceTask = { id: string; kind: string; model: string; status: string; fingerprint: string; upstream_id: string; error?: string; preview?: Record<string, unknown>; bindings: {kind: string; id: string; revision?: number}[]; result?: any };
  let project = $state('');
  let profiles = $state<Document<Profile>[]>([]);
  let profileForms = $state<Record<string, Profile>>({});
  let scripts = $state<Document<{ approved: boolean; lines: Line[] }>[]>([]);
  let tasks = $state<VoiceTask[]>([]);
  let characters = $state<any[]>([]);
  let assets = $state<any[]>([]);
  let scenes = $state<any[]>([]);
  let outputs = $state<any[]>([]);
  let lines = $state<Line[]>([]);
  let scriptID = $state('main');
  let scriptRevision = $state(0);
  let approved = $state(false);
  let busy = $state('');
  let error = $state('');
  let notice = $state('');
  let loaded = $state(false);
  let selectedOutput = $state('');
  let selectedAudio = $state('');
  let selectedImages = $state<string[]>([]);
  let nativePrompt = $state('');
  let nativeDuration = $state(5);
  let nativeAspect = $state('16:9');
  let reconciliation = $state<Record<string, string>>({});
  let listeningReview = $state<Record<string, boolean>>({});
  const profileFor = (id: string) => profiles.find(p => p.id === id);
  const characterName = (id: string) => characters.find(c => c.id === id)?.name ?? id;
  const scoped = (path: string) => `${path}${path.includes('?') ? '&' : '?'}project_id=${encodeURIComponent(project)}`;
  const sampleAssets = $derived(assets.filter(a => /\.(wav|mp3|flac|m4a)$/i.test(a.file_path)));
  const imageAssets = $derived(assets.filter(a => isImage(a.file_path)));

  function loadScript(id: string) {
    scriptID = id;
    const doc = scripts.find(s => s.id === id);
    scriptRevision = doc?.revision ?? 0;
    approved = doc?.data.approved ?? false;
    lines = structuredClone(doc?.data.lines ?? []);
  }
  async function refresh(reset = false) {
    const state = await get(project ? scoped('/voice/state') : '/voice/state');
    if (project && state.project_id !== project) throw new Error('Project changed. Reload this workspace.');
    project = state.project_id;
    profiles = state.profiles;
    scripts = state.scripts;
    tasks = state.tasks;
    const [allCharacters, allAssets, allScenes, jobs] = await Promise.all([get('/characters'), get('/assets'), get('/scenes'), get('/render-jobs')]);
    characters = allCharacters; assets = allAssets; scenes = allScenes;
    const details = await Promise.all(jobs.filter((j: any) => j.status === 'succeeded').map((j: any) => get(`/render-jobs/${j.id}`)));
    outputs = details.flatMap((d: any) => d.outputs.map((o: any) => ({ ...o, model: d.job.model })));
    if (reset) {
      profileForms = Object.fromEntries(characters.map(c => [c.id, structuredClone(profileFor(c.id)?.data ?? { mode: 'saved', sample_asset_id: '', voice_id: '', approved: false })]));
      loadScript(scriptID);
    }
  }
  onMount(() => { refresh(true).catch(e => error = e.message).finally(() => loaded = true); });
  async function action(key: string, fn: () => Promise<unknown>, reset = false) {
    if (busy) return;
    busy = key; error = ''; notice = '';
    try { await fn(); await refresh(reset); } catch (e: any) { error = e.message; } finally { busy = ''; }
  }
  async function saveProfile(id: string) {
    await put(scoped(`/voice/profiles/${id}`), { revision: profileFor(id)?.revision ?? 0, data: profileForms[id] });
    notice = 'Character voice revision saved. Prepared actions using older revisions require a new review.';
  }
  async function uploadSample(id: string, event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    const file = input.files?.[0]; if (!file) return;
    await action(`upload-${id}`, async () => {
      const form = new FormData(); form.set('file', file); form.set('type', 'audio'); form.set('character_id', id);
      const asset = await upload(`/assets/upload?project_id=${encodeURIComponent(project)}`, form);
      profileForms[id].sample_asset_id = asset.id;
      profileForms[id].approved = false;
      notice = 'Audio uploaded. Review and save the character voice before preparing a paid action.';
    });
    input.value = '';
  }
  async function saveScript() {
    const saved = await put(scoped(`/voice/scripts/${encodeURIComponent(scriptID)}`), { revision: scriptRevision, data: { approved, lines } });
    scriptRevision = saved.revision;
    notice = 'Exact spoken lines saved. Captions and speech from older script revisions are stale.';
  }
  function addLine() { lines.push({ id: crypto.randomUUID(), character_id: characters[0]?.id ?? '', language: 'en', text: '', visible: false }); approved = false; }
  function importDialogue() {
    const imported: Line[] = scenes.flatMap(scene => {
      let data = scene.scene_json; if (typeof data === 'string') { try { data = JSON.parse(data); } catch { return []; } }
      return (data?.spoken_lines ?? []).map((line: any) => ({ id: line.id || crypto.randomUUID(), character_id: line.character_id || '', language: line.language || 'en', text: line.text || '', visible: Boolean(line.visible) }));
    });
    if (!imported.length) { error = 'These scenes have no structured spoken lines. Add dialogue below; scene summaries are not captions.'; return; }
    lines = imported; approved = false; notice = 'Imported dialogue draft. Check speakers, wording and stable IDs before approving.';
  }
  async function prepare(kind: string, extra: Record<string, unknown> = {}) {
    const task = await post(scoped('/voice/tasks/prepare'), { kind, script_id: scriptID, ...extra });
    notice = task.status === 'draft' ? 'Action prepared. Review the exact route and revisions below, then use Run to approve the paid request.' : 'Recovered the existing action. Refresh its saved receipt; it will not submit again.';
  }
  async function taskAction(task: VoiceTask, endpoint: string, body: unknown = {}) {
    const result = await post(scoped(`/voice/tasks/${task.id}/${endpoint}`), body);
    if (endpoint === 'accept-voice') profileForms[result.id] = structuredClone(result.data);
  }
  function chooseImage(id: string, checked: boolean) { selectedImages = checked ? [...selectedImages, id] : selectedImages.filter(v => v !== id); }
  const resultURL = (task: VoiceTask) => mediaUrl(task.result?.local_path) || task.result?.audio?.url || null;
</script>

<svelte:head><title>Voices & script captions — VideoFlow</title></svelte:head>

<div class="workspace">
  <header class="page-head">
    <div><p class="eyebrow">CHARACTER PERFORMANCE</p><h1><Mic size={28} /> Voices & script captions</h1><p>Keep each speaker's identity and the approved wording through speech, video and final captions.</p></div>
    <Button variant="outline" disabled={!!busy} onclick={() => action('refresh', () => refresh())}><RefreshCw size={15} /> Refresh</Button>
  </header>
  <nav class="links"><a href="/characters">Characters</a><a href="/scenes">Script & scenes</a><a href="/render">My videos</a></nav>
  {#if error}<div role="alert" class="message error">{error}</div>{/if}
  {#if notice}<div role="status" class="message">{notice}</div>{/if}
  {#if !loaded}<p class="muted">Loading this project's voices…</p>{:else}
  <section aria-labelledby="voice-heading">
    <div class="section-head"><div><p class="eyebrow">01 / SPEAKERS</p><h2 id="voice-heading">A saved voice for every character</h2></div><p>Saved voices are the default. Native reference audio is an explicit alternative.</p></div>
    {#if !characters.length}<p class="empty">Add a character in <a href="/characters">Characters</a> to bind an approved voice.</p>{/if}
    <div class="profile-grid">
      {#each characters as character (character.id)}
        {@const profile = profileForms[character.id]}
        {#if profile}
        {@const sample = assets.find(a => a.id === profile.sample_asset_id)}
        <article class="panel">
          <div class="row"><h3>{character.name}</h3><Badge variant="secondary">revision {profileFor(character.id)?.revision ?? 0}</Badge></div>
          <label>Voice mode<select bind:value={profile.mode} onchange={() => profile.approved = false}><option value="saved">Saved character voice (default)</option><option value="native_reference">Native reference audio → video</option></select></label>
          <label>Approved audio sample<select bind:value={profile.sample_asset_id} onchange={() => profile.approved = false}><option value="">Select an uploaded sample</option>{#each sampleAssets as a}<option value={a.id}>{a.name}</option>{/each}</select></label>
          <label class="upload">Upload sample<input type="file" accept="audio/wav,audio/mpeg,audio/flac,audio/mp4,.wav,.mp3,.flac,.m4a" disabled={!!busy} onchange={e => uploadSample(character.id, e)} /></label>
          {#if sample}<audio controls preload="none" src={mediaUrl(sample.file_path) ?? ''}></audio>{/if}
          {#if profile.mode === 'saved'}<label>Provider voice ID<input bind:value={profile.voice_id} placeholder="Bind an existing MiniMax voice ID, or create from sample" onchange={() => profile.approved = false} /></label><p class="hint">Voice creation needs a sample of at least 10 seconds. Use the created voice with speech within 7 days to retain it.</p>{:else}<p class="hint">Native video uses a WAV/MP3 sample up to 15 seconds and 15 MB plus an approved visual reference. Review identity and spoken wording after generation.</p>{/if}
          <label class="check"><input type="checkbox" bind:checked={profile.approved} /> I approve this character's sample / provider voice ID.</label>
          <div class="actions"><Button disabled={!!busy} onclick={() => action(`profile-${character.id}`, () => saveProfile(character.id))}>Save voice revision</Button>{#if profile.mode === 'saved'}<Button variant="outline" disabled={!!busy || !profileFor(character.id)?.data.approved || !profileFor(character.id)?.data.sample_asset_id} onclick={() => action(`clone-${character.id}`, () => prepare('clone', { character_id: character.id }))}>Prepare voice creation</Button>{/if}</div>
        </article>
        {/if}
      {/each}
    </div>
  </section>

  <section aria-labelledby="script-heading">
    <div class="section-head"><div><p class="eyebrow">02 / EXACT WORDING</p><h2 id="script-heading">Approved spoken lines</h2></div><p>Untimed lines are drafts. Final audio supplies timing; the script supplies every word.</p></div>
    <div class="panel">
      <div class="row"><label>Spoken script<select value={scriptID} onchange={e => loadScript(e.currentTarget.value)}><option value="main">Main dialogue</option>{#each scripts.filter(s => s.id !== 'main') as doc}<option value={doc.id}>{doc.id}</option>{/each}</select></label><span class="muted">revision {scriptRevision}</span><Button variant="outline" disabled={!!busy} onclick={importDialogue}>Import scene dialogue draft</Button></div>
      {#each lines as line, i (line.id)}
        <div class="utterance">
          <div class="line-meta"><label>Speaker<select bind:value={line.character_id} onchange={() => approved = false}><option value="">Select character</option>{#each characters as c}<option value={c.id}>{c.name}</option>{/each}</select></label><label>Language<input bind:value={line.language} oninput={() => approved = false} placeholder="en" /></label><label class="check"><input type="checkbox" bind:checked={line.visible} onchange={() => approved = false} /> Visible speaker (lip sync needed)</label><button class="icon-button" aria-label={`Remove spoken line ${i + 1}`} onclick={() => { lines = lines.filter(l => l.id !== line.id); approved = false; }}><Trash2 size={16} /></button></div>
          <label>Exact spoken wording<textarea rows="2" bind:value={line.text} oninput={() => approved = false} placeholder="Dialogue or narration only"></textarea></label><p class="hint">Utterance {line.id}</p>
          <div class="actions"><Button variant="outline" disabled={!!busy || !scripts.find(s => s.id === scriptID)?.data.approved} onclick={() => action(`speech-${line.id}`, () => prepare('speech', { line_id: line.id }))}>Prepare saved-voice speech</Button><Button variant="outline" disabled={!!busy || !selectedOutput || !selectedAudio} onclick={() => action(`lip-${line.id}`, () => prepare('lipsync', { line_id: line.id, output_id: selectedOutput, audio_asset_id: selectedAudio }))}>Prepare lip sync</Button>{#if profileFor(line.character_id)?.data.mode === 'native_reference'}<Button variant="outline" disabled={!!busy} onclick={() => action(`native-${line.id}`, () => prepare('native_reference', { line_id: line.id, prompt: nativePrompt, duration: nativeDuration, aspect_ratio: nativeAspect, image_asset_ids: selectedImages }))}>Prepare native reference video</Button>{/if}</div>
        </div>
      {/each}
      <div class="actions"><Button variant="outline" disabled={!!busy || !characters.length} onclick={addLine}><Plus size={15} /> Add spoken line</Button><label class="check"><input type="checkbox" bind:checked={approved} /> I approve the exact wording, speaker and language.</label><Button disabled={!!busy || !lines.length} onclick={() => action('script', saveScript)}>Save script revision</Button></div>
    </div>
  </section>

  <section aria-labelledby="timing-heading">
    <div class="section-head"><div><p class="eyebrow">03 / PERFORMANCE & TIMING</p><h2 id="timing-heading">Final audio, lip sync and captions</h2></div><p>Replacing audio or changing script invalidates alignment. Mixing audio alone does not synchronize mouths.</p></div>
    <div class="panel">
      <div class="two-columns"><label>Final output for alignment / lip sync<select bind:value={selectedOutput}><option value="">Select a rendered output</option>{#each outputs as out}<option value={out.id}>{out.id} · {out.model}</option>{/each}</select></label><label>Saved-voice speech for lip sync<select bind:value={selectedAudio}><option value="">Select generated speech audio</option>{#each sampleAssets.filter(a => a.metadata_json?.voice_task_id) as a}<option value={a.id}>{characterName(a.character_id)} · {a.id}</option>{/each}</select></label></div>
      <p class="hint">Lip sync uses the selected spoken line and its matching speech asset. Audio and video duration must match within 250 ms. Narration can use a separate audio track in compositions.</p>
      {#if selectedOutput}<div class="actions"><Button disabled={!!busy || !scripts.find(s => s.id === scriptID)?.data.approved} onclick={() => action('alignment', () => prepare('alignment', { output_id: selectedOutput }))}>Prepare script → final audio alignment</Button><a href={`/editor/${selectedOutput}`}>Edit / burn existing captions <ArrowRight size={14} /></a>{#each ['srt', 'vtt', 'json'] as format}<a href={`${API_BASE}${scoped(`/voice/outputs/${selectedOutput}/captions/export?format=${format}`)}`}>{format.toUpperCase()}</a>{/each}</div>{/if}
      <details><summary>Native reference video settings</summary><label>Reviewed scene prompt<textarea rows="3" bind:value={nativePrompt} placeholder="Describe the scene using the selected character references (@Image1…)"></textarea></label><div class="row"><label>Duration (seconds)<input type="number" min="4" max="15" bind:value={nativeDuration} /></label><label>Aspect ratio<select bind:value={nativeAspect}><option>16:9</option><option>9:16</option><option>1:1</option><option>4:3</option><option>3:4</option><option>21:9</option></select></label></div><p class="hint">Select 1–9 images. This reference endpoint preserves audio constraints and does not support strict first/last frame mode.</p><div class="reference-list">{#each imageAssets as asset}<label class="check"><input type="checkbox" checked={selectedImages.includes(asset.id)} onchange={e => chooseImage(asset.id, e.currentTarget.checked)} />{asset.name}</label>{/each}</div></details>
    </div>
  </section>

  <section aria-labelledby="requests-heading">
    <div class="section-head"><div><p class="eyebrow">04 / RUN & RECOVER</p><h2 id="requests-heading">Prepared actions and saved receipts</h2></div><p>Only Run submits paid work. Refresh and reconciliation retrieve existing work and never resubmit.</p></div>
    {#if !tasks.length}<p class="empty">Prepare an action above to review its exact source revisions and provider route.</p>{/if}
    {#each tasks as task (task.id)}
      <article class="panel task"><div class="row"><h3>{task.kind.replaceAll('_', ' ')}</h3><Badge variant="secondary">{task.status}</Badge></div><p class="route">fal / {task.model}</p><p class="hint">{task.bindings.map(b => `${b.kind}: ${b.id}${b.revision ? ` @ revision ${b.revision}` : ''}`).join(' · ')}</p><p class="hint">Local task {task.id}{#if task.upstream_id}<br />Upstream request {task.upstream_id}{/if}</p>
        {#if task.preview && Object.keys(task.preview).length}<details><summary>Exact approved payload</summary><pre class="payload">{JSON.stringify(task.preview, null, 2)}</pre></details>{/if}
        {#if task.error}<p class="message error">{task.error}</p>{/if}
        {#if task.status === 'draft'}<p>Run approves this exact prepared action and may incur provider charges. Editing a source revision requires preparing a new action.</p><Button disabled={!!busy} onclick={() => action(task.id, () => taskAction(task, 'run', { confirmed: true, fingerprint: task.fingerprint }))}><Play size={15} /> Run paid {task.kind.replaceAll('_', ' ')}</Button>
        {:else if ['submitting', 'uncertain'].includes(task.status) && !task.upstream_id}<p>Submission is fenced. Review fal activity, then attach the existing request ID.</p><label>Existing fal request ID<input bind:value={reconciliation[task.id]} /></label><Button variant="outline" disabled={!!busy || !reconciliation[task.id]} onclick={() => action(task.id, () => taskAction(task, 'reconcile', { reviewed: true, upstream_id: reconciliation[task.id] }))}>Reconcile reviewed receipt</Button>
        {:else if task.upstream_id && !['completed', 'review_required', 'failed'].includes(task.status)}<Button variant="outline" disabled={!!busy} onclick={() => action(task.id, () => taskAction(task, 'refresh'))}><RefreshCw size={14} /> Refresh existing request</Button>{/if}
        {#if resultURL(task) && task.kind !== 'native_reference' && task.kind !== 'lipsync'}<audio controls preload="none" src={resultURL(task) ?? ''}></audio>{/if}
        {#if task.result?.output_id}<a href={`/editor/${task.result.output_id}`}>Review generated video <ArrowRight size={14} /></a>{/if}
        {#if task.result?.asset_id}<a href={mediaUrl(task.result.local_path) ?? '#'} target="_blank" rel="noreferrer">Open generated speech</a>{/if}
        {#if task.status === 'review_required' && task.kind === 'clone'}<label class="check"><input type="checkbox" bind:checked={listeningReview[task.id]} /> I listened to and approve the created character voice.</label><Button disabled={!!busy || !listeningReview[task.id]} onclick={() => action(task.id, () => taskAction(task, 'accept-voice', { reviewed: true }))}>Bind approved saved voice</Button>{/if}
        {#if task.status === 'review_required' && task.kind === 'alignment'}<p class="hint">{task.result?.review_warning ?? 'Alignment does not verify spoken accuracy. Review omissions and drift.'}</p>{#if task.result?.segments?.length}<ol class="cues">{#each task.result.segments as cue}<li><span>{cue.start.toFixed(2)}–{cue.end.toFixed(2)}s</span> {cue.text}</li>{/each}</ol><label class="check"><input type="checkbox" bind:checked={listeningReview[task.id]} /> I listened to the final audio and checked wording, omissions and cue timing.</label><Button disabled={!!busy || !listeningReview[task.id]} onclick={() => action(task.id, () => taskAction(task, 'apply-captions', { reviewed: true }))}>Apply to editable captions</Button>{/if}{/if}
      </article>
    {/each}
  </section>
  {/if}
</div>

<style>
  .workspace{max-width:1160px;margin:0 auto;padding:32px 28px 80px}.page-head,.section-head,.row,.actions,.links{display:flex;align-items:center;gap:14px}.page-head,.section-head{justify-content:space-between}.page-head{align-items:flex-start}.page-head h1{display:flex;align-items:center;gap:12px;font-size:28px;font-weight:650;letter-spacing:-.035em;margin:8px 0}.page-head p,.section-head>p{color:var(--muted-foreground);max-width:650px}.eyebrow{font-size:10px;font-weight:650;letter-spacing:.14em;color:var(--muted-foreground)}.links{margin:20px 0 32px;font-size:13px}section{margin-top:38px}.section-head{align-items:flex-end;margin-bottom:16px}.section-head h2{font-size:20px;letter-spacing:-.025em;margin-top:6px}.section-head>p{max-width:390px;font-size:12px}.profile-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.panel{border:1px solid var(--border);border-radius:12px;background:var(--card);padding:22px;margin-bottom:16px}.panel h3{font-size:16px;font-weight:600}.row{flex-wrap:wrap}.row h3{flex:1}.row>label{flex:1;min-width:100px}.actions{flex-wrap:wrap;margin-top:16px}.hint,.muted{font-size:11px;color:var(--muted-foreground);line-height:1.7;overflow-wrap:anywhere}.hint{margin:8px 0}.message{border:1px solid var(--border);background:var(--muted);padding:12px 16px;border-radius:8px;font-size:13px;margin:14px 0}.message.error{color:var(--destructive);border-color:color-mix(in srgb,var(--destructive) 25%,transparent)}label{display:flex;flex-direction:column;gap:6px;font-size:12px;margin:12px 0;color:var(--foreground)}input:not([type=checkbox]),select,textarea{width:100%;min-height:36px;border:1px solid var(--input);background:var(--background);color:var(--foreground);border-radius:6px;padding:8px 10px;font:inherit}textarea{resize:vertical}.check{display:flex;flex-direction:row;align-items:center;gap:8px;line-height:1.5}.check input{accent-color:var(--primary)}audio{display:block;width:100%;height:36px;margin:12px 0}a{display:inline-flex;align-items:center;gap:6px;text-decoration:underline;text-underline-offset:4px;font-size:12px}.empty{border:1px dashed var(--border);border-radius:10px;padding:24px;color:var(--muted-foreground);font-size:13px}.utterance{border-top:1px solid var(--border);margin-top:18px;padding-top:12px}.line-meta{display:grid;grid-template-columns:1fr 90px 1fr auto;gap:12px;align-items:center}.icon-button{padding:8px;color:var(--muted-foreground)}.two-columns{display:grid;grid-template-columns:1fr 1fr;gap:18px}details{border-top:1px solid var(--border);padding-top:18px;margin-top:22px}summary{cursor:pointer;font-size:13px;font-weight:600}.reference-list{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:0 16px}.payload{white-space:pre-wrap;overflow-wrap:anywhere;font-size:12px;padding:14px 0}.route{font-size:12px;margin-top:8px;overflow-wrap:anywhere}.task>p:not(.hint):not(.route){font-size:12px;margin:12px 0}.cues{list-style:decimal;padding-left:20px;max-height:250px;overflow:auto;font-size:13px}.cues li{margin:8px 0}.cues span{font-variant-numeric:tabular-nums;color:var(--muted-foreground);font-size:11px;margin-right:8px}.upload input{font-size:11px}.task a{margin:10px 10px 10px 0}@media(max-width:760px){.workspace{padding:24px 16px 60px}.page-head,.section-head{align-items:flex-start;flex-direction:column}.profile-grid,.two-columns{grid-template-columns:1fr}.line-meta{grid-template-columns:1fr 70px}.reference-list{grid-template-columns:1fr 1fr}.page-head h1{font-size:24px}}
</style>
