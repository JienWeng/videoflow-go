<script lang="ts">
 import { protectDraft } from '$lib/protect-draft';
 import GenerationRecovery from '$lib/GenerationRecovery.svelte';
  import { replaceState } from '$app/navigation';
  import { page } from '$app/state';
  import { onMount, tick } from 'svelte';
  import { get, put, post } from '$lib/api';
  import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '$lib/components/ui/card';
  import { Badge } from '$lib/components/ui/badge';
  import { Button } from '$lib/components/ui/button';
  import { Input } from '$lib/components/ui/input';
  import { Separator } from '$lib/components/ui/separator';
  import { Skeleton } from '$lib/components/ui/skeleton';
  import { toast } from 'svelte-sonner';
  import {
    RotateCcw,
    Plug,
    KeyRound,
    Sparkles,
    Palette,
    Sun,
    Moon,
    Monitor,
    Plus,
    Trash2,
    LoaderCircle,
    CircleCheck,
    Wifi
  } from '@lucide/svelte';

  // -------------------------------------------------------------- types
  type Agent = {
    agent: string;
    label: string;
    provider: string;
    model: string;
    default_provider: string;
    default_model: string;
  };
  type Provider = {
    name: string;
    label?: string;
    protocol?: string;
    configured: boolean;
    models: string[];
    suggested_models: string[];
    default_model: string;
    allow_custom: boolean;
    from_db: boolean;
  };
  type ProviderConfig = {
    name: string;
    configured: boolean;
    masked_key: string;
    base_url: string | null;
    from_db: boolean;
  };
  type AppSettings = {
 all_text_local?: boolean;
    default_aspect_ratio: string;
    default_video_provider: string;
    default_image_provider: string;
    default_scene_duration: number;
    caption_style: string;
    caption_language: string;
    whisper_model: string;
    dialogue_language: string;
    image_model: string;
    ref_image_model: string;
    video_model: string;
    vl_model: string;
    max_video_refs: number;
media_model_preferences:Record<string,Record<string,string>>;
    render_negatives: string[];
  };
  type TestResult = { checked?:string;message?:string;ok: boolean; latency_ms: number | null; error: string | null };
  type ProviderPreset = {
    label: string;
    protocol: 'chat' | 'responses' | 'anthropic' | 'codex' | 'media' | 'video';
    url: string;
    model: string;
    preset: string;
    mode: 'auto' | 'tools' | 'json' | 'prompt';
    vision: boolean;
  };

  // -------------------------------------------------------------- state
  let agents = $state<Agent[]>([]);
  let providers = $state<Provider[]>([]);
  let app = $state<AppSettings | null>(null);
  let loaded = $state(false);
  let error = $state('');
  let busy = $state('');
  let saveStatus = $state('');
  let confirmClear = $state('');
  let confirmReset = $state(false);
  let durationError = $state('');
  const providerDirty = (name:string) => !!provKeyInput[name]?.trim() || (provUrlInput[name] ?? '').trim() !== (provCfg[name]?.base_url ?? '');
  protectDraft(() => Object.keys(provKeyInput).some(providerDirty), 'You have unsaved provider credentials. Discard them and leave?');
  async function retryLoad() { error = ''; loaded = false; try { await refresh(); } catch (e: any) { error = e.message; } finally { loaded = true; } }
  let presets = $state<Record<string, any>>({});
  let newLabel = $state('');
  let newPreset = $state('openrouter');
  let newProtocol = $state('chat');
  let newUrl = $state('https://openrouter.ai/api/v1');
  let newModel = $state('openai/gpt-4o-mini');
  let newMode = $state('auto');
  let newVision = $state(true);
  function choosePreset() {
    const p = presets[newPreset];
    if (!p) return;
    newProtocol = p.protocol; newUrl = p.url ?? ''; newModel = p.model;
  }
  async function addConnection() {
    busy = 'add-connection';
    try {
      await post('/settings/connections', { label: newLabel.trim(), preset: newPreset,
        protocol: newProtocol, base_url: newUrl || null, model: newModel.trim(), mode: newMode, vision: newVision });
      newLabel = '';
      await refresh();
      toast.success('Connection added. Configure its credentials below, then assign agents.');
    } catch (e: any) { toast.error(e.message); }
    finally { busy = ''; }
  }
  async function discoverModels(name: string) {
    busy = `models-${name}`;
    try {
      const result = await get(providerEndpoint(name, 'models')+(name==='fal'?'?modality=all':name==='byteplus'?'?modality=video':''));
      if (result.error) { toast.error(result.error); return; }
      providers = providers.map(p => p.name === name ? {...p, suggested_models: result.models, models: result.models} : p);
      if (result.models.length) {
        testModelInput[name] = result.models[0];
        providerModelInput[name] = false;
      }
      toast.success(`Loaded ${result.models.length} supported models`);
    } catch (e: any) { toast.error(e.message); }
    finally { busy = ''; }
  }

  // Per-provider editable form fields (key/base_url) + configured pill state.
  let provCfg = $state<Record<string, ProviderConfig>>({});
  let provKeyInput = $state<Record<string, string>>({});
  let provUrlInput = $state<Record<string, string>>({});
  let provTest = $state<Record<string, TestResult | undefined>>({});
  let testModelInput = $state<Record<string, string>>({});
  async function verifyModel(name: string) {
    busy = `verify-${name}`;
    try {
      const result = await post(providerEndpoint(name, 'verify-model'), {model: testModelInput[name]?.trim()});
      if (result.ok) toast.success(result.generation_tested?'Model returned valid structured JSON':'Model catalog checked; no generation submitted.');
      else toast.error(result.error);
    } catch (e: any) { toast.error(e.message); }
    finally { busy = ''; }
  }

  // Theme radio — persisted to the shared localStorage key the shell reads.
  const THEME_KEY = 'videoflow.theme';
  let theme = $state<'light' | 'dark' | 'system'>('system');

  // Active settings tab (bound so clicking a trigger switches sections).
  let tab = $state('general');
  function settingsHash(){const h=window.location.hash.slice(1);tab=['providers','connections'].includes(h)?'connections':['engines','models'].includes(h)?'models':['agents','advanced'].includes(h)?'advanced':'general';}
  function chooseSection(value:string){tab=value;replaceState('#'+value,page.state);}

  // ------------------------------------------------------------- labels
  const providerLabels: Record<string, string> = { fal:'fal.ai',byteplus:'BytePlus ModelArk',openrouter:'OpenRouter' };
  const FALLBACK_PRESETS: Record<string, ProviderPreset> = {
    openrouter: { label: 'OpenRouter', protocol: 'chat', url: 'https://openrouter.ai/api/v1', model: 'openai/gpt-4o-mini', preset: 'openrouter', mode: 'auto', vision: true }
  };

  // One-line descriptions for each agent, plus a text/vision split.
  const VISION_AGENTS = new Set(['qa_agent', 'asset_recogniser']);
  const agentDesc: Record<string, string> = {
    script_agent: 'Turns your idea into a structured script.',
    scene_agent: 'Breaks the script into scenes.',
    shot_agent: 'Plans the shots within each scene.',
    prompt_agent: 'Writes the image / video generation prompts.',
    qa_agent: 'Looks at rendered frames and flags problems.',
    idea_agent: 'Develops and expands the core idea.',
    asset_planner: 'Decides which assets a scene needs.',
    refine_agent: 'Edits and refines copy on request.',
    intent_agent: 'Classifies what you are asking for.',
    asset_recogniser: 'Identifies characters / objects in images.',
    character_memory: 'Maintains the character bible.',
    style_agent: 'Designs the visual style.'
  };

  // -------------------------------------------------------------- options
  const ASPECT_RATIOS = ['9:16', '16:9', '1:1', '21:9', '4:3', '3:4'];
  const WHISPER_MODELS = ['openai/whisper-1', 'openai/whisper-large-v3', 'openai/whisper-large-v3-turbo'];
  const CAPTION_STYLES = ['clean', 'bold', 'minimal', 'cinematic', 'neon', 'kids', 'classic', 'comic'];
  const CAPTION_LANGUAGES = [
    { value: 'auto', label: 'Auto-detect' },
    { value: 'zh', label: 'Chinese' },
    { value: 'en', label: 'English' }
  ];
  const DIALOGUE_LANGUAGES = [
    { value: 'zh', label: 'Chinese' },
    { value: 'en', label: 'English' }
  ];
  const CUSTOM = '__custom__';
  const LOAD_MODELS = '__load_models__';

  const selectablePresets = $derived.by(() => {
    const entries = Object.entries(presets).filter(([id, preset]) => (preset as any).preset === id && preset.protocol === 'chat');
    return entries.length ? entries : Object.entries(FALLBACK_PRESETS);
  });

  function providerEndpoint(name: string, suffix?: string): string {
    const base = `/settings/providers/${encodeURIComponent(name)}`;
    return suffix ? `${base}/${suffix}` : base;
  }

  // ------------------------------------------------------------- derived
  const byName = $derived(Object.fromEntries(providers.map((p) => [p.name, p])));
  const configuredProviders = $derived(
    providers.filter((p) => p.configured && p.protocol === 'chat')
  );
  const primaryTextProvider = $derived(
    agents.find((agent) => agent.agent === 'script_agent')?.provider ?? 'openrouter'
  );
  const orderedProviders = $derived(
    providers
  );
  const textAgents = $derived(agents.filter((a) => !VISION_AGENTS.has(a.agent)));
  const visionAgents = $derived(agents.filter((a) => VISION_AGENTS.has(a.agent)));

  // -------------------------------------------------------------- load
  async function refresh() {
    try {
      presets = await get('/settings/connection-presets');
    } catch {
      presets = FALLBACK_PRESETS;
    }
    const [ag, pv, ap] = await Promise.all([
      get('/settings/agents'),
      get('/settings/providers'),
      get('/settings/app')
    ]);
    agents = ag;
    providers = pv;
    testModelInput = Object.fromEntries(
      pv.map((p: Provider) => [p.name, p.models?.[0] ?? p.suggested_models?.[0] ?? p.default_model ?? ''])
    );
    for (const p of providers) providerLabels[p.name] = p.label ?? p.name;
    app = ap;
    // Load each provider's secret config (masked key + base_url) in parallel.
    const cfgs: ProviderConfig[] = await Promise.all(
      (pv as Provider[]).map((p) => get(providerEndpoint(p.name)))
    );
    const cfgMap: Record<string, ProviderConfig> = {};
    const keyMap: Record<string, string> = {};
    const urlMap: Record<string, string> = {};
    for (const c of cfgs) {
      cfgMap[c.name] = c;
      keyMap[c.name] = provKeyInput[c.name] ?? '';
      urlMap[c.name] = c.base_url ?? '';
    }
    provCfg = cfgMap;
    provKeyInput = keyMap;
    provUrlInput = urlMap;
    await loadEngineCatalogs();
  }

  onMount(() => {
    settingsHash();
    // Hydrate the theme radio from the shared key (fallback to system).
    try {
      const stored = localStorage.getItem(THEME_KEY);
      if (stored === 'light' || stored === 'dark' || stored === 'system') theme = stored;
    } catch {
      /* private mode / disabled storage */
    }
    refresh()
      .catch((e) => (error = e.message))
      .finally(() => (loaded = true));
  });

  // ------------------------------------------------------------- agents
  function isDefault(a: Agent): boolean {
    return a.provider === a.default_provider && a.model === a.default_model;
  }
  function suggestedFor(provider: string): string[] {
    return byName[provider]?.suggested_models ?? [];
  }

  async function saveAgent(a: Agent, provider: string, model: string) {
    busy = `agent-${a.agent}`;
    try {
      const updated = await put(`/settings/agents/${a.agent}`, { provider, model });
      agents = agents.map((x) => (x.agent === a.agent ? updated : x));
      toast.success(`${a.label} → ${providerLabels[provider] ?? provider} / ${model}`);
    } catch (e: any) {
      toast.error(e.message);
      await refresh().catch(() => {});
    } finally {
      busy = '';
    }
  }

  async function onAgentProvider(a: Agent, provider: string) {
    if (provider === a.provider) return;
    const model = byName[provider]?.default_model ?? suggestedFor(provider)[0];
    if (!model) {
      toast.error('No models available for this provider');
      return;
    }
    await saveAgent(a, provider, model);
  }

  // Free-typed model: select offers suggestions + a "Custom…" sentinel that
  // swaps the control for a text input.
  let agentCustomOpen = $state<Record<string, boolean>>({});
  let agentCustomVal = $state<Record<string, string>>({});

  function onAgentModelSelect(a: Agent, value: string) {
    if (value === CUSTOM) {
      agentCustomOpen[a.agent] = true;
      agentCustomVal[a.agent] = a.model;
      return;
    }
    agentCustomOpen[a.agent] = false;
    if (value !== a.model) saveAgent(a, a.provider, value);
  }
  function commitAgentCustom(a: Agent) {
    const m = (agentCustomVal[a.agent] ?? '').trim();
    agentCustomOpen[a.agent] = false;
    if (m && m !== a.model) saveAgent(a, a.provider, m);
  }

  async function resetAgent(a: Agent) {
    busy = `agent-${a.agent}`;
    try {
      const updated = await put(`/settings/agents/${a.agent}`, { provider: null, model: null });
      agents = agents.map((x) => (x.agent === a.agent ? updated : x));
      agentCustomOpen[a.agent] = false;
      toast.success(`${a.label} reset to default`);
    } catch (e: any) {
      toast.error(e.message);
    } finally {
      busy = '';
    }
  }

  async function setAllText(provider: string) {
    if (!provider) return;
    const model = byName[provider]?.default_model ?? suggestedFor(provider)[0];
    if (!model) {
      toast.error('No models available for this provider');
      return;
    }
    busy = 'bulk-text';
    try {
      for (const a of textAgents) {
        const updated = await put(`/settings/agents/${a.agent}`, { provider, model });
        agents = agents.map((x) => (x.agent === a.agent ? updated : x));
      }
      toast.success(`All text agents → ${providerLabels[provider] ?? provider} / ${model}`);
    } catch (e: any) {
      toast.error(e.message);
      await refresh().catch(() => {});
    } finally {
      busy = '';
    }
  }

  async function resetAllAgents() {
    busy = 'bulk-reset';
    try {
      for (const a of agents) {
        const updated = await put(`/settings/agents/${a.agent}`, { provider: null, model: null });
        agents = agents.map((x) => (x.agent === a.agent ? updated : x));
      }
      agentCustomOpen = {};
      toast.success('All agents reset to defaults');
    } catch (e: any) {
      toast.error(e.message);
      await refresh().catch(() => {});
    } finally {
      busy = '';
    }
  }

  let bulkTextProvider = $state('');

  let providerModelInput = $state<Record<string, boolean>>({});
  let providerModelCustom = $state<Record<string, string>>({});

  function providerModelOptions(name: string): string[] {
    const p = byName[name];
    const dedup = new Map<string, true>();
    for (const m of [...(p?.models ?? []), ...(p?.suggested_models ?? [])]) {
      if (m) dedup.set(m, true);
    }
    if (p?.default_model) dedup.set(p.default_model, true);
    return [...dedup.keys()];
  }

  function onProviderModelSelect(name: string, value: string) {
    if (value === LOAD_MODELS) {
      discoverModels(name);
      return;
    }
    if (value === CUSTOM) {
      providerModelInput[name] = true;
      providerModelCustom[name] = testModelInput[name] ?? '';
      return;
    }
    providerModelInput[name] = false;
    testModelInput[name] = value;
  }

  function commitProviderCustomModel(name: string) {
    const m = (providerModelCustom[name] ?? '').trim();
    providerModelInput[name] = false;
    if (m) testModelInput[name] = m;
  }

  // ----------------------------------------------------------- providers
  async function saveProvider(name: string) {
    busy = `prov-${name}`;
    try {
      const body: { api_key?: string; base_url?: string } = {};
      // Only send api_key if the user typed something; an empty field leaves the
      // stored key untouched (so saving a base_url alone keeps the key).
      const k = provKeyInput[name] ?? '';
      if (k.trim()) body.api_key = k.trim();
      body.base_url = (provUrlInput[name] ?? '').trim();
      const updated: ProviderConfig = await put(providerEndpoint(name), body);
      provCfg[name] = updated;
      provKeyInput[name] = '';
      provUrlInput[name] = updated.base_url ?? '';
      // Reflect the new configured-state in the catalog (drives agent selects).
      providers = providers.map((p) =>
        p.name === name ? { ...p, configured: updated.configured, from_db: updated.from_db } : p
      );
      provTest[name] = undefined;
      toast.success(`${providerLabels[name] ?? name} saved`);
    } catch (e: any) {
      toast.error(e.message);
    } finally {
      busy = '';
    }
  }

  async function clearProvider(name: string) {
    busy = `prov-${name}`;
    try {
      const updated: ProviderConfig = await put(providerEndpoint(name), { api_key: '' });
      provCfg[name] = updated;
      provKeyInput[name] = '';
      providers = providers.map((p) =>
        p.name === name ? { ...p, configured: updated.configured, from_db: updated.from_db } : p
      );
      provTest[name] = undefined;
      confirmClear = '';
      toast.success(`${providerLabels[name] ?? name} key cleared`);
    } catch (e: any) {
      toast.error(e.message);
    } finally {
      busy = '';
    }
  }

  async function testProvider(name: string) {
    busy = `test-${name}`;
    provTest[name] = undefined;
    try {
      const res: TestResult = await post(providerEndpoint(name, 'test'));
      provTest[name] = res;
      if (res.ok) toast.success(res.checked==='configuration'?res.message!:`${providerLabels[name] ?? name}: connected (${res.latency_ms}ms)`);
      else toast.error(`${providerLabels[name] ?? name}: ${res.error ?? 'failed'}`);
    } catch (e: any) {
      provTest[name] = { ok: false, latency_ms: null, error: e.message };
      toast.error(e.message);
    } finally {
      busy = '';
    }
  }

  // -------------------------------------------------- app settings (engines/defaults)
  async function saveApp(patch: Partial<AppSettings>) {
    busy = 'app';
    saveStatus = 'Saving preferences…';
    try {
      const updated: AppSettings = await put('/settings/app', patch);
      app = updated;
if(patch.default_image_provider||patch.default_video_provider)await loadEngineCatalogs();
      saveStatus = 'Preferences saved';
    } catch (e: any) {
      saveStatus = 'Could not save preferences. Check your connection and try again.';
      toast.error(e.message);
      // Rebuild controls from the last confirmed values, including native selects.
      const previous = app;
      app = null;
      await tick();
      app = previous;
    } finally {
      busy = '';
    }
  }

  type VideoCapability = { id: string; supported_frame_images?: string[]; supported_durations?: number[]; supported_resolutions?: string[]; supported_aspect_ratios?: string[] };
  let engineCatalogs = $state<Record<string, string[]>>({
    image: ['openai/gpt-image-2'], video: ['google/veo-3.1-lite'],
    vision: ['qwen/qwen3-vl-30b-a3b-instruct'], transcription: WHISPER_MODELS
  });
  let videoCapabilities = $state<VideoCapability[]>([]);
  let catalogError = $state('');
  let catalogBusy = $state(false);
  let catalogSequence=0;
  async function loadEngineCatalogs() {
    if(!app)return;
    const sequence=++catalogSequence;
    const imageProvider=app.default_image_provider;
    const videoProvider=app.default_video_provider;
    catalogBusy=true;
    const failures:string[]=[];
    await Promise.all(['image','video','vision','transcription'].map(async modality=>{
      const provider=modality==='image'?imageProvider:modality==='video'?videoProvider:'openrouter';
      if(provider==='openrouter'&&!provCfg.openrouter?.configured){if(sequence===catalogSequence)engineCatalogs={...engineCatalogs,[modality]:[]};return}
      try{
        const result=await get(`/settings/providers/${encodeURIComponent(provider)}/models?modality=${modality}`);
        if(sequence!==catalogSequence)return;
        if(result.error){failures.push(`${provider} ${modality}: ${result.error}`);engineCatalogs={...engineCatalogs,[modality]:[]};return}
        engineCatalogs={...engineCatalogs,[modality]:result.models??[]};if(modality==='video')videoCapabilities=result.capabilities??[];
      }catch(e:any){failures.push(`${provider} ${modality}: ${e.message}`)}
    }));
    if(sequence===catalogSequence){catalogError=failures.join('; ');catalogBusy=false}
  }
  function mediaProviderChanged(kind:'image'|'video',provider:string){
    void saveApp({[kind==='image'?'default_image_provider':'default_video_provider']:provider});
  }
  function engineOptions(key: keyof AppSettings): string[] {
    return engineCatalogs[key === 'vl_model' ? 'vision' : key === 'video_model' ? 'video' : 'image'] ?? [];
  }
  function videoImageInputLimit(): number {
    return videoCapabilities.find(model => model.id === app?.video_model)?.supported_frame_images?.length ?? 0;
  }
  function videoImageInputDescription(): string {
    const model = videoCapabilities.find(model => model.id === app?.video_model);
    return model ? `Supported frame anchors: ${model.supported_frame_images?.join(', ') || 'none'}.` : 'Load the catalog to check frame support.';
  }
  function videoModelHint(): string {
    const model = videoCapabilities.find(model => model.id === app?.video_model);
    return model ? `Durations: ${model.supported_durations?.join(', ')} seconds. Resolutions: ${model.supported_resolutions?.join(', ')}.` : 'Capabilities are checked before the selected provider receives a render request.';
  }

  let engineCustomOpen = $state<Record<string, boolean>>({});
  let engineCustomVal = $state<Record<string, string>>({});

  function onEngineSelect(key: keyof AppSettings, value: string, current: string) {
    if (value === CUSTOM) {
      engineCustomOpen[key as string] = true;
      engineCustomVal[key as string] = current;
      return;
    }
    engineCustomOpen[key as string] = false;
    if (value !== current) saveApp({ [key]: value } as Partial<AppSettings>);
  }
  function commitEngineCustom(key: keyof AppSettings, current: string) {
    const v = (engineCustomVal[key as string] ?? '').trim();
    engineCustomOpen[key as string] = false;
    if (v && v !== current) saveApp({ [key]: v } as Partial<AppSettings>);
  }

  // Render negatives — editable list saved as a whole.
  let newNegative = $state('');
  function addNegative() {
    const v = newNegative.trim();
    if (!v || !app) return;
    if (app.render_negatives.includes(v)) {
      newNegative = '';
      return;
    }
    const next = [...app.render_negatives, v];
    newNegative = '';
    saveApp({ render_negatives: next });
  }
  function removeNegative(idx: number) {
    if (!app) return;
    const next = app.render_negatives.filter((_, i) => i !== idx);
    saveApp({ render_negatives: next });
  }

  // ------------------------------------------------------------- theme
  function setTheme(value: 'light' | 'dark' | 'system') {
    theme = value;
    try {
      localStorage.setItem(THEME_KEY, value);
    } catch {
      /* ignore */
    }
    // Data-attribute + class fallback so the choice applies immediately even
    // before the shell's ModeWatcher reacts. ModeWatcher owns the final state.
    const root = document.documentElement;
    root.setAttribute('data-theme', value);
    const prefersDark =
      typeof window !== 'undefined' &&
      window.matchMedia &&
      window.matchMedia('(prefers-color-scheme: dark)').matches;
    const dark = value === 'dark' || (value === 'system' && prefersDark);
    root.classList.toggle('dark', dark);
  }
</script>

<svelte:head><title>Settings · VideoFlow</title></svelte:head>

<div class="settings-page mx-auto max-w-6xl p-4 sm:p-8">
  <div class="mb-4">
    <h1 class="text-2xl font-semibold tracking-tight">Settings</h1>
  </div>

  {#if error}
    <Card class="mb-4 border-destructive/40">
      <CardContent class="space-y-3 p-4"><p role="alert" class="text-sm text-destructive">Settings could not load: {error}</p><Button variant="outline" onclick={retryLoad}>Try again</Button></CardContent>
    </Card>
  {/if}

  {#if !loaded}
    <div role="status" aria-busy="true" aria-label="Loading settings" class="grid gap-6 md:grid-cols-[200px_minmax(0,1fr)]">

      <div aria-hidden="true" class="grid grid-cols-2 gap-2 self-start md:block md:space-y-2">{#each Array(4) as _, i (i)}<Skeleton aria-hidden="true" class="h-12 motion-reduce:animate-none" />{/each}</div>
      <div class="space-y-6"><p class="settings-help">Loading settings…</p><Skeleton aria-hidden="true" class="h-8 w-36 motion-reduce:animate-none" /><Skeleton aria-hidden="true" class="h-24 w-full motion-reduce:animate-none" /><Skeleton aria-hidden="true" class="h-8 w-44 motion-reduce:animate-none" /><Skeleton aria-hidden="true" class="h-40 w-full motion-reduce:animate-none" /></div>
    </div>
  {:else if !error}
    <div class="grid gap-6 md:grid-cols-[200px_minmax(0,1fr)]">
      <nav aria-label="Settings sections" class="grid grid-cols-2 gap-2 self-start md:sticky md:top-8 md:block md:space-y-2">
        {#each [{id:'general',label:'General',hint:'Video defaults & appearance'},{id:'connections',label:'AI connections',hint:'Provider keys & access'},{id:'models',label:'Media models',hint:'Image, video & vision'},{id:'advanced',label:'Advanced',hint:'Routing & recovery'}] as section}
          <button type="button" aria-label={section.label} aria-current={tab===section.id?'page':undefined} onclick={()=>chooseSection(section.id)} class="min-h-11 flex-1 rounded-lg px-3 py-2 text-left md:w-full {tab===section.id?'bg-accent font-semibold':'hover:bg-accent text-muted-foreground'}"><span class="block text-sm">{section.label}</span><span class="hidden text-xs font-normal md:block">{section.hint}</span></button>
        {/each}
      </nav>
      <div class="min-w-0 space-y-6" data-testid="settings-content">
        <div class="space-y-1 text-sm text-muted-foreground"><p>{tab === 'connections' ? 'Save each provider key before checking its connection.' : 'Preferences save automatically when changed.'}</p><p role="status" aria-live="polite" class="min-h-5 text-foreground">{saveStatus}</p></div>
      <!-- ============================================ APPEARANCE -->
      {#if tab === 'general'}
      <section id="appearance" class="settings-section">
        <Card>
          <CardHeader>
            <CardTitle role="heading" aria-level={2} class="flex items-center gap-2 text-base">
              <Palette class="size-4" />Appearance
            </CardTitle>
            <CardDescription>Choose a theme for this device.</CardDescription>
          </CardHeader>
          <CardContent>
            <div role="radiogroup" aria-label="Theme" class="flex flex-wrap gap-2">
              {#each [{ v: 'light', label: 'Light', icon: Sun }, { v: 'dark', label: 'Dark', icon: Moon }, { v: 'system', label: 'System', icon: Monitor }] as opt (opt.v)}
                <button
                  type="button"
                  role="radio"
                  aria-checked={theme === opt.v}
                  class="flex items-center gap-2 rounded-lg border px-4 py-2 text-sm transition-colors
                         {theme === opt.v
                    ? 'border-primary bg-accent font-medium'
                    : 'border-border hover:bg-accent/50'}"
                  onclick={() => setTheme(opt.v as 'light' | 'dark' | 'system')}
                  tabindex={theme === opt.v ? 0 : -1}
                  onkeydown={(e) => { if (['ArrowRight','ArrowDown','ArrowLeft','ArrowUp','Home','End'].includes(e.key)) { e.preventDefault(); const options = ['light','dark','system'] as const; const index = e.key === 'Home' ? 0 : e.key === 'End' ? 2 : (options.indexOf(theme) + (['ArrowRight','ArrowDown'].includes(e.key) ? 1 : 2)) % 3; setTheme(options[index]); ((e.currentTarget as HTMLElement).parentElement?.querySelectorAll('button')[index] as HTMLElement)?.focus(); } }}
                >
                  <opt.icon class="size-4" />
                  {opt.label}
                </button>
              {/each}
            </div>
          </CardContent>
        </Card>
      </section>
      {/if}
      <!-- ============================================ PROVIDERS -->
      {#if tab === 'connections'}
      <section id="providers" class="settings-section">
        <Card>
          <CardHeader>
            <CardTitle role="heading" aria-level={2} class="flex items-center gap-2 text-base">
              <Plug class="size-4" />AI connections
            </CardTitle>
            <CardDescription>
              Configure fal.ai for images/video, OpenRouter for vision/transcription, and optional direct BytePlus Seedance video. Planning and writing use local AI sessions. Keys stay on the backend and are never shown again after saving.
            </CardDescription>
          </CardHeader>
          <CardContent class="space-y-5">
            {#each orderedProviders as p (p.name)}
              {@const cfg = provCfg[p.name]}
              {@const test = provTest[p.name]}
              <details open={p.name === primaryTextProvider || p.name === app?.default_image_provider || p.name === app?.default_video_provider}>
                <summary class="cursor-pointer rounded-md py-2 text-sm font-medium">
                  {providerLabels[p.name] ?? p.name}
                  <span class="ml-2 text-xs font-normal text-muted-foreground">{cfg?.configured ? 'Configured' : 'Not configured'}</span>
                </summary>
              <div data-provider={p.name}>
                {#if cfg?.configured}
                  <p class="settings-help mb-2">{cfg.from_db ? cfg.masked_key : 'Provided by the server environment'}</p>
                {/if}

                <div class="grid gap-2 sm:grid-cols-2">
                  <div>
                    <label class="mb-1 block text-xs text-muted-foreground" for="key-{p.name}">
                      API key
                    </label>
                    <div class="relative">
                      <KeyRound
                        class="absolute left-2 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground"
                      />
                      <Input
                        id="key-{p.name}"
                        type="password"
                        autocomplete="off"
                        class="pl-7"
                        placeholder={cfg?.configured ? '•••••• (leave blank to keep)' : 'Paste API key'}
                        bind:value={provKeyInput[p.name]}
                      />
                    </div>
                  </div>
                </div>
                <details class="mt-2">
                  <summary class="cursor-pointer text-xs text-muted-foreground">Provider endpoint</summary>
                  <label class="mt-2 block max-w-xl text-xs text-muted-foreground" for="url-{p.name}">Base URL</label>
                  <Input id="url-{p.name}" readonly value={cfg?.base_url ?? ''} />
                </details>

                <p class="settings-help mt-2" role="status">{providerDirty(p.name) ? 'Unsaved credential changes' : 'No unsaved credential changes'}</p>
                <div class="mt-2 flex flex-wrap items-center gap-2">
                  {#if providerModelInput[p.name]}
                    <Input
                      class="max-w-64"
                      aria-label={`Custom test model for ${p.label ?? p.name}`}
                      placeholder="Model ID to test"
                      bind:value={providerModelCustom[p.name]}
                      onkeydown={(e) => e.key === 'Enter' && commitProviderCustomModel(p.name)}
                    />
                    <Button
                      size="sm"
                      disabled={!providerModelCustom[p.name]?.trim() || busy === `verify-${p.name}`}
                      onclick={() => {
                        commitProviderCustomModel(p.name);
                        verifyModel(p.name);
                      }}
                    >
                      Verify
                    </Button>
                  {:else}
                    <select
                      aria-label={`Test model for ${p.label ?? p.name}`}
                      class="w-64 rounded-md border border-input bg-background px-2 py-1.5 text-sm"
                      disabled={!cfg?.configured || busy === `verify-${p.name}`}
                      value={testModelInput[p.name] ?? ''}
                      onchange={(e) => onProviderModelSelect(p.name, (e.currentTarget as HTMLSelectElement).value)}
                    >
                      {#if providerModelOptions(p.name).length}
                        {#each providerModelOptions(p.name) as m (m)}
                          <option value={m}>{m}</option>
                        {/each}
                      {:else}
                        <option value="" disabled>Load or enter model</option>
                      {/if}
                      <option value={LOAD_MODELS}>Load models…</option>
                      <option value={CUSTOM}>Custom…</option>
                    </select>
                    <Button
                      size="sm"
                      variant="outline"
                      disabled={!cfg?.configured || !testModelInput[p.name]?.trim() || busy === `verify-${p.name}`}
                      onclick={() => verifyModel(p.name)}
                    >
                      Verify
                    </Button>
                  {/if}
                  <span class="settings-help">{p.protocol === 'chat' ? 'Text model tests consume provider usage.' : 'Catalog checks do not submit generation or verify account/model access.'}</span>
                  <Button
                    size="sm"
                    disabled={!!busy || !providerDirty(p.name)}
                    onclick={() => saveProvider(p.name)}
                  >
                    {busy === `prov-${p.name}` ? 'Saving…' : 'Save'}
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={busy === `test-${p.name}` || !cfg?.configured}
                    onclick={() => testProvider(p.name)}
                  >
                    {#if busy === `test-${p.name}`}
                      <LoaderCircle class="size-3.5 mr-1 animate-spin" />Testing…
                    {:else}
                      <Wifi class="size-3.5 mr-1" />{p.protocol === 'chat' ? 'Test connection' : 'Check configuration'}
                    {/if}
                  </Button>
                  {#if cfg?.from_db}
                    <Button
                      size="sm"
                      variant="ghost"
                      class="text-destructive hover:text-destructive"
                      disabled={busy === `prov-${p.name}`}
                      onclick={() => (confirmClear = p.name)}
                    >
                      Clear key
                    </Button>
                  {/if}

                  {#if confirmClear === p.name}
                    <div class="w-full space-y-2 border-t border-border pt-3" role="group" aria-label={`Remove ${providerLabels[p.name] ?? p.name} key`}>
                      <p class="text-sm">Remove the saved {providerLabels[p.name] ?? p.name} key? Requests using it will stop working unless an environment key is available. Projects and outputs are kept.</p>
                      <Button size="sm" variant="destructive" disabled={!!busy} onclick={() => clearProvider(p.name)}>Remove saved key</Button>
                      <Button size="sm" variant="outline" onclick={() => (confirmClear = '')}>Keep key</Button>
                    </div>
                  {/if}
                  {#if test}
                    {#if test.ok}
                      <span class="flex items-center gap-1 text-xs text-emerald-500">
                        <CircleCheck class="size-3.5" />{test.checked === 'configuration' ? 'Configuration checked' : 'Connected'}
                        {#if test.checked !== 'configuration' && test.latency_ms != null}({test.latency_ms}ms){/if}
                      </span>
                    {:else}
                      <span class="text-xs text-destructive">{test.error ?? 'Connection failed'}</span>
                    {/if}
                  {/if}
                </div>
              </div>
              {#if p.name !== orderedProviders[orderedProviders.length - 1].name}
                <Separator />
              {/if}
              </details>
            {/each}
            <details class="rounded-lg border p-4">
              <summary class="cursor-pointer text-sm font-medium">Additional connections</summary>
              <div class="mt-3 space-y-3">
              <p class="settings-help">Only add a named OpenRouter connection if you need a separate account or configuration. To set up your primary account, use OpenRouter above.</p>
              <div class="grid gap-3 sm:grid-cols-2">
                <label class="text-xs">Name<Input bind:value={newLabel} placeholder="My OpenRouter" /></label>
                <label class="text-xs">Provider<select class="block w-full rounded border p-2 bg-background" bind:value={newPreset} onchange={choosePreset}>
                  {#each selectablePresets as [id, p]}<option value={id}>{p.label}</option>{/each}
                </select></label>
                <label class="text-xs">Protocol<select class="block w-full rounded border p-2 bg-background" bind:value={newProtocol}>
                  <option value="chat">OpenRouter Chat Completions</option>

                </select></label>
                <label class="text-xs">Default model<Input bind:value={newModel} placeholder="Model ID from your provider" /></label>
                {#if newProtocol !== 'codex'}
                  <label class="text-xs">Base URL<Input bind:value={newUrl} /></label>
                  <label class="text-xs">Chat Completions output mode<select class="block w-full rounded border p-2 bg-background" bind:value={newMode} disabled={newProtocol !== 'chat'}>
                    <option value="auto">Provider default</option><option value="prompt">JSON in prompt</option>
                  </select></label>
                {/if}
              </div>
              <label class="flex items-center gap-2 text-xs"><input type="checkbox" bind:checked={newVision} />Models on this connection accept images</label>
              <Button size="sm" disabled={!newLabel.trim() || busy === 'add-connection'} onclick={addConnection}>Add connection</Button>
              </div>
            </details>

          </CardContent>
        </Card>
      </section>
      {/if}

      <!-- ============================================ ENGINES -->
      {#if tab === 'models'}
      <section id="engines" class="settings-section">
        <Card>
          <CardHeader>
            <CardTitle role="heading" aria-level={2} class="flex items-center gap-2 text-base">
              <Sparkles class="size-4" />Media models
            </CardTitle>
            <CardDescription>
              Choose defaults for new media. Provider keys are managed in AI connections; planning and writing use local AI sessions.
            </CardDescription>
          </CardHeader>
          <CardContent class="settings-section">
            <Button size="sm" variant="outline" disabled={catalogBusy} onclick={loadEngineCatalogs}>{catalogBusy ? 'Loading catalogs…' : 'Refresh model catalogs'}</Button>
            {#if catalogError}<p class="text-sm text-destructive">{catalogError}</p>{/if}
            {#if app}
              {@const engines = [
                { key: 'image_model', label: 'Image model', hint: `Image generation via ${providerLabels[app.default_image_provider] ?? app.default_image_provider}.` },
                { key: 'ref_image_model', label: 'Character and prop image model', hint: `Character references and props via ${providerLabels[app.default_image_provider] ?? app.default_image_provider}.` },
                { key: 'video_model', label: 'Video model', hint: videoModelHint() },
                { key: 'vl_model', label: 'Vision model', hint: 'Reads frames for QA.' }
              ] as const}
              {#each engines as eng (eng.key)}
                {@const cur = app[eng.key] as string}
                <div class="flex flex-wrap items-center gap-3">
                  <div class="min-w-[180px] flex-1">
                    <div class="settings-label">{eng.label}</div>
                    <div class="settings-help">{eng.hint}</div>
                  </div>
                  <div class="flex items-center gap-2">
                    {#if engineCustomOpen[eng.key]}
                      <Input
                        class="h-8 w-64 text-sm"
                        placeholder="Custom model id"
                        bind:value={engineCustomVal[eng.key]}
                        onkeydown={(e) => e.key === 'Enter' && commitEngineCustom(eng.key, cur)}
                      />
                      <Button size="sm" disabled={busy === 'app'} onclick={() => commitEngineCustom(eng.key, cur)}>
                        Save
                      </Button>
                      <Button size="sm" variant="ghost" onclick={() => (engineCustomOpen[eng.key] = false)}>
                        Cancel
                      </Button>
                    {:else}
                      <select
                        aria-label={eng.label}
                        class="w-64 rounded-md border border-input bg-background px-2 py-1.5 text-sm"
                        disabled={busy === 'app'}
                        value={cur}
                        onchange={(e) => onEngineSelect(eng.key, (e.currentTarget as HTMLSelectElement).value, cur)}
                      >
                        {#if cur && !engineOptions(eng.key).includes(cur)}
                          <option value={cur}>{cur} — unverified custom ID</option>
                        {/if}
                        {#each engineOptions(eng.key) as m (m)}
                          <option value={m}>{m}</option>
                        {/each}
                        <option value={CUSTOM}>Custom…</option>
                      </select>
                    {/if}
                  </div>
                </div>
              {/each}

              <Separator />

              <div class="flex flex-wrap items-center gap-3">
                <div class="min-w-[180px] flex-1">
                  <div class="settings-label">Video provider</div>
                  <div class="settings-help">Used by one-click renders.</div>
                </div>
                <select
                  aria-label="Video provider"
                  class="w-64 rounded-md border border-input bg-background px-2 py-1.5 text-sm"
                  disabled={busy === 'app'}
                  value={app.default_video_provider}
                  onchange={(e) => mediaProviderChanged('video', (e.currentTarget as HTMLSelectElement).value)}
                >

                  <option value="fal">fal.ai</option><option value="openrouter">OpenRouter</option><option value="byteplus">BytePlus ModelArk (Seedance)</option>
                </select>
              </div>

              <div class="flex flex-wrap items-center gap-3">
                <div class="min-w-[180px] flex-1">
                  <div class="settings-label">Image provider</div>
                  <div class="settings-help">Used for storyboard keyframes.</div>
                </div>
                <select
                  aria-label="Image provider"
                  class="w-64 rounded-md border border-input bg-background px-2 py-1.5 text-sm"
                  disabled={busy === 'app'}
                  value={app.default_image_provider}
                  onchange={(e) => mediaProviderChanged('image', (e.currentTarget as HTMLSelectElement).value)}
                >

                  <option value="fal">fal.ai</option><option value="openrouter">OpenRouter</option>
                </select>
              </div>

              <Separator />

              <div class="flex flex-wrap items-center gap-3">
              <div class="min-w-[180px] flex-1">
                  <div class="settings-label">Image references accepted by the selected video route</div>
                  <div class="settings-help">
                    {videoImageInputDescription()}
                  </div>
                </div>
                <span class="rounded-md border border-border px-3 py-1.5 text-sm tabular-nums">{videoImageInputLimit()} image references</span>
              </div>
            {/if}
          </CardContent>
        </Card>
      </section>
      {/if}

      <!-- ============================================ DEFAULTS -->
      {#if tab === 'general'}
      <section id="defaults" class="settings-section">
        <Card>
          <CardHeader>
            <CardTitle role="heading" aria-level={2} class="text-base">Scene defaults</CardTitle>
            <CardDescription>Applied to new scenes and renders.</CardDescription>
          </CardHeader>
          <CardContent class="settings-section">
            {#if app}
              <div class="flex flex-wrap items-center gap-3">
                <div class="min-w-[180px] flex-1 text-sm font-medium">Default aspect ratio</div>
                <select
                  aria-label="Default aspect ratio"
                  class="w-48 rounded-md border border-input bg-background px-2 py-1.5 text-sm"
                  disabled={busy === 'app'}
                  value={app.default_aspect_ratio}
                  onchange={(e) =>
                    saveApp({ default_aspect_ratio: (e.currentTarget as HTMLSelectElement).value })}
                >
                  {#each ASPECT_RATIOS as r (r)}
                    <option value={r}>{r}</option>
                  {/each}
                </select>
              </div>

              <div class="flex flex-wrap items-start gap-3">
                <div class="min-w-[180px] flex-1"><label class="settings-label" for="scene-duration">Default scene duration (s)</label><p id="duration-hint" class="settings-help">1–120 whole seconds.</p></div>
                <div><Input id="scene-duration" aria-label="Default scene duration (seconds)" aria-invalid={durationError ? 'true' : undefined} aria-describedby={durationError ? 'duration-hint duration-error' : 'duration-hint'} type="number" min="1" max="120" step="1" class="h-8 w-24 text-sm" disabled={busy === 'app'} value={app.default_scene_duration}
                  onfocus={() => durationError = ''}
                  oninput={() => durationError = ''}
                  onchange={(e) => { const input=e.currentTarget as HTMLInputElement;const n=Number(input.value);if(!input.value || !Number.isInteger(n) || n<1 || n>120) {durationError='Enter a scene duration from 1 to 120 in whole seconds.';return;}durationError='';if(n!==app!.default_scene_duration)void saveApp({default_scene_duration:n}); }} />
                {#if durationError}<p id="duration-error" role="alert" class="mt-2 max-w-xs text-sm text-destructive">{durationError}</p>{/if}</div>
              </div>

              <div class="flex flex-wrap items-center gap-3">
                <div class="min-w-[180px] flex-1 text-sm font-medium">Dialogue language</div>
                <select
                  aria-label="Dialogue language"
                  class="w-48 rounded-md border border-input bg-background px-2 py-1.5 text-sm"
                  disabled={busy === 'app'}
                  value={app.dialogue_language}
                  onchange={(e) =>
                    saveApp({ dialogue_language: (e.currentTarget as HTMLSelectElement).value })}
                >
                  {#if !DIALOGUE_LANGUAGES.some(l => l.value === app!.dialogue_language)}<option value={app.dialogue_language}>{app.dialogue_language}</option>{/if}
                  {#each DIALOGUE_LANGUAGES as l (l.value)}
                    <option value={l.value}>{l.label}</option>
                  {/each}
                </select>
              </div>
            {/if}
          </CardContent>
        </Card>

        <Card class="mt-4">
          <CardHeader>
            <CardTitle role="heading" aria-level={2} class="text-base">Captions</CardTitle>
            <CardDescription>Auto-caption style, language and transcription model.</CardDescription>
          </CardHeader>
          <CardContent class="settings-section">
            {#if app}
              <div class="flex flex-wrap items-center gap-3">
                <div class="min-w-[180px] flex-1 text-sm font-medium">Caption style</div>
                <select
                  aria-label="Caption style"
                  class="w-48 rounded-md border border-input bg-background px-2 py-1.5 text-sm"
                  disabled={busy === 'app'}
                  value={app.caption_style}
                  onchange={(e) =>
                    saveApp({ caption_style: (e.currentTarget as HTMLSelectElement).value })}
                >
                  {#each CAPTION_STYLES as s (s)}
                    <option value={s}>{s}</option>
                  {/each}
                </select>
              </div>

              <div class="flex flex-wrap items-center gap-3">
                <div class="min-w-[180px] flex-1 text-sm font-medium">Caption language</div>
                <select
                  aria-label="Caption language"
                  class="w-48 rounded-md border border-input bg-background px-2 py-1.5 text-sm"
                  disabled={busy === 'app'}
                  value={app.caption_language}
                  onchange={(e) =>
                    saveApp({ caption_language: (e.currentTarget as HTMLSelectElement).value })}
                >
                  {#each CAPTION_LANGUAGES as l (l.value)}
                    <option value={l.value}>{l.label}</option>
                  {/each}
                </select>
              </div>

              <div class="flex flex-wrap items-center gap-3">
                <div class="min-w-[180px] flex-1 text-sm font-medium">OpenRouter transcription model</div>
                <select
                  aria-label="OpenRouter transcription model"
                  class="w-48 rounded-md border border-input bg-background px-2 py-1.5 text-sm"
                  disabled={busy === 'app'}
                  value={app.whisper_model}
                  onchange={(e) =>
                    saveApp({ whisper_model: (e.currentTarget as HTMLSelectElement).value })}
                >
                  {#if !engineCatalogs.transcription.includes(app.whisper_model)}<option value={app.whisper_model}>{app.whisper_model}</option>{/if}
                  {#each engineCatalogs.transcription as m (m)}
                    <option value={m}>{m}</option>
                  {/each}
                </select>
              </div>
            {/if}
          </CardContent>
        </Card>


      </section>
      {/if}

      {#if tab === 'advanced'}<section aria-label="Render preferences">
        <Card class="mt-4">
          <CardHeader>
            <CardTitle role="heading" aria-level={2} class="text-base">Render negative prompts</CardTitle>
            <CardDescription>
              Fragments appended to every render to steer the model away from common artefacts.
            </CardDescription>
          </CardHeader>
          <CardContent class="space-y-3">
            {#if app}
              {#if app.render_negatives.length}
                <div class="flex flex-wrap gap-2">
                  {#each app.render_negatives as neg, i (neg)}
                    <span
                      class="inline-flex items-center gap-1 rounded-md border border-border bg-muted/40 px-2 py-1 text-xs"
                    >
                      {neg}
                      <button
                        type="button"
                        class="text-muted-foreground hover:text-destructive disabled:opacity-40"
                        title="Remove"
                        aria-label="Remove {neg}"
                        disabled={busy === 'app'}
                        onclick={() => removeNegative(i)}
                      >
                        <Trash2 class="size-3" />
                      </button>
                    </span>
                  {/each}
                </div>
              {:else}
                <p class="settings-help">No negative prompts yet.</p>
              {/if}
              <form
                class="flex items-center gap-2"
                onsubmit={(e) => {
                  e.preventDefault();
                  addNegative();
                }}
              >
                <Input
                  class="h-8 max-w-xs text-sm"
                  aria-label="New negative prompt"
                  placeholder="e.g. blurry, extra fingers"
                  bind:value={newNegative}
                  disabled={busy === 'app'}
                />
                <Button type="submit" size="sm" variant="outline" disabled={busy === 'app' || !newNegative.trim()}>
                  <Plus class="size-3.5 mr-1" />Add
                </Button>
              </form>
            {/if}
          </CardContent>
        </Card>
      </section>{/if}

      <!-- ============================================ AGENTS -->
      {#if tab === 'advanced'}
      <section id="agents" class="settings-section">
        <Card>
          <CardHeader>
            <CardTitle role="heading" aria-level={2} class="text-base">Agent routing</CardTitle>
            <CardDescription>
              Choose which provider and model powers each step. Only configured providers are
              selectable; model ids can be free-typed.
            </CardDescription>
          </CardHeader>
          <CardContent>
           <div class="space-y-5">
            <!-- Bulk controls -->
 {#if !app?.all_text_local}
            <div class="flex flex-wrap items-center gap-2 rounded-lg border border-border bg-muted/30 p-3">
              <span class="text-xs font-medium">Set all text agents to</span>
              <select
                aria-label="Bulk provider for text agents"
                class="rounded-md border border-input bg-background px-2 py-1.5 text-sm"
                bind:value={bulkTextProvider}
                disabled={!!busy}
              >
                <option value="">Choose provider…</option>
                {#each configuredProviders as p (p.name)}
                  <option value={p.name}>{providerLabels[p.name] ?? p.name}</option>
                {/each}
              </select>
              <Button
                size="sm"
                variant="secondary"
                disabled={!!busy || !bulkTextProvider}
                onclick={() => setAllText(bulkTextProvider)}
              >
                {busy === 'bulk-text' ? 'Applying…' : 'Apply'}
              </Button>
              <div class="flex-1"></div>
              <Button size="sm" variant="ghost" disabled={!!busy} onclick={() => (confirmReset = true)}>
                <RotateCcw class="size-3.5 mr-1" />
                {busy === 'bulk-reset' ? 'Resetting…' : 'Reset all'}
              </Button>
            </div>

            {#if confirmReset}<div role="group" aria-label="Reset agent routing" class="space-y-2"><p class="text-sm">Reset every agent to its default provider and model? Your custom routing will be replaced.</p><Button variant="destructive" disabled={!!busy} onclick={async () => { await resetAllAgents(); confirmReset = false; }}>Reset routing</Button><Button variant="outline" onclick={() => (confirmReset = false)}>Cancel</Button></div>{/if}
            {/if}
 {#snippet agentRow(a: Agent)}
              <div
                class="flex flex-wrap items-center gap-3 rounded-lg border border-border bg-card p-3"
                data-agent={a.agent}
              >
                <div class="min-w-[160px] flex-1">
                  <div class="flex items-center gap-2">
                    <span class="settings-label" title={agentDesc[a.agent] ?? ''}>{a.label}</span>
                    {#if isDefault(a)}
                      <Badge variant="outline" class="text-xs">default</Badge>
                    {/if}
                  </div>
                  <div class="settings-help">
                    {agentDesc[a.agent] ?? ''}
                    {#if !isDefault(a)}
                      <span class="block">
                        default: {providerLabels[a.default_provider] ?? a.default_provider} / {a.default_model}
                      </span>
                    {/if}
                  </div>
                </div>

                <div class="flex w-full min-w-0 flex-wrap items-center gap-2 lg:w-auto">
                  <select
                    aria-label="{a.label} provider"
                    class="w-full min-w-0 rounded-md border border-input bg-background px-2 py-1.5 text-sm sm:w-auto sm:max-w-full"
                    disabled={busy === `agent-${a.agent}` || !!busy}
                    value={a.provider}
                    onchange={(e) => onAgentProvider(a, (e.currentTarget as HTMLSelectElement).value)}
                  >
                    {#each configuredProviders as p (p.name)}
                      <option value={p.name}>{providerLabels[p.name] ?? p.name}</option>
                    {/each}
                    {#if !byName[a.provider]?.configured}
                      <option value={a.provider}>{providerLabels[a.provider] ?? a.provider} (unconfigured)</option>
                    {/if}
                  </select>

                  {#if agentCustomOpen[a.agent]}
                    <Input
                      class="h-8 w-full min-w-0 text-sm sm:w-48"
                      placeholder="Custom model id"
                      bind:value={agentCustomVal[a.agent]}
                      onkeydown={(e) => e.key === 'Enter' && commitAgentCustom(a)}
                    />
                    <Button size="sm" disabled={busy === `agent-${a.agent}`} onclick={() => commitAgentCustom(a)}>
                      Save
                    </Button>
                    <Button size="sm" variant="ghost" onclick={() => (agentCustomOpen[a.agent] = false)}>
                      Cancel
                    </Button>
                  {:else}
                    <select
                      aria-label="{a.label} model"
                      class="min-w-0 flex-1 rounded-md border border-input bg-background px-2 py-1.5 text-sm sm:w-48 sm:flex-none"
                      disabled={busy === `agent-${a.agent}` || !!busy}
                      value={a.model}
                      onchange={(e) => onAgentModelSelect(a, (e.currentTarget as HTMLSelectElement).value)}
                    >
                      {#if !suggestedFor(a.provider).includes(a.model)}
                        <option value={a.model}>{a.model}</option>
                      {/if}
                      {#each suggestedFor(a.provider) as m (m)}
                        <option value={m}>{m}</option>
                      {/each}
                      <option value={CUSTOM}>Custom…</option>
                    </select>

                    <button
                      type="button"
                      title="Reset to default"
                      aria-label="Reset {a.label} to default"
                      class="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-40"
                      disabled={busy === `agent-${a.agent}` || isDefault(a)}
                      onclick={() => resetAgent(a)}
                    >
                      <RotateCcw class="size-4" />
                    </button>
                  {/if}
                </div>
              </div>
            {/snippet}

            <div>
              <div class="mb-2 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
                Text agents
              </div>
              <div class="space-y-2">
                {#if app?.all_text_local}<p class="text-sm">All planning and writing use the selected local Claude Code/Codex session. <a class="underline" href="/director">Manage engine, sessions and selected references</a></p>{:else}{#each textAgents as a (a.agent)}
                  {@render agentRow(a)}
                {/each}{/if}
              </div>
            </div>

            <div>
              <div class="mb-2 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
                Vision agents
              </div>
              <div class="space-y-2">
                {#each visionAgents as a (a.agent)}
                  {@render agentRow(a)}
                {/each}
              </div>
            </div>
           </div>
          </CardContent>
        </Card>
      </section>
      {/if}

      </div>
    </div>
  {/if}
</div>

{#if tab==='advanced'}<GenerationRecovery/>{/if}
<svelte:window onhashchange={settingsHash} />

<style>
  .settings-page { --settings-label-size: .875rem; --settings-help-size: .75rem; --settings-section-gap: 1rem; }
  .settings-page :global(.settings-label) { font-size:var(--settings-label-size); line-height:1.25rem; font-weight:500; }
  .settings-page :global(.settings-help) { font-size:var(--settings-help-size); line-height:1rem; color:var(--muted-foreground); }
  .settings-page :global(.settings-section > :not(:last-child)) { margin-bottom:var(--settings-section-gap); }

  .settings-page :global([data-slot="card"]) { border-radius: 0; box-shadow: none; border-width: 0 0 1px; border-color: var(--border); background: transparent; padding-bottom: 1.5rem; }
  .settings-page :global([data-slot="card-header"]), .settings-page :global([data-slot="card-content"]) { padding-left: 0; padding-right: 0; }
  .settings-page :global(select) { max-width: 100%; }
  .settings-page :global(summary) { min-height: 2.75rem; align-content: center; }
  @media (max-width: 639px) { .settings-page :global(select) { width: 100%; } }
</style>
