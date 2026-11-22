<script lang="ts">
  import { errorMessage } from '$lib/errors';
  // One place to see which AI does each job, whether it is ready, and to change it.
  import { onMount } from 'svelte';
  import { get, post, put } from '$lib/api';
  import { Badge } from '$lib/components/ui/badge';
  import { Button } from '$lib/components/ui/button';
  import { Skeleton } from '$lib/components/ui/skeleton';
  import AgentConnect from '$lib/director/AgentConnect.svelte';
  import { AudioLines, Check, Clapperboard, Eye, Image, MessageSquare, Mic, TriangleAlert } from '@lucide/svelte';

  type Choice = { service: string; model: string; ready: boolean; note: string };
  type Setup = {
    assistant: {
      ready: boolean;
      engines: Record<string, boolean>;
      session_id?: string;
      engine?: string;
      title?: string;
      model?: string;
      effort?: string;
    };
    pictures: Choice;
    vision: Choice;
    listening: Choice;
    video: {
      service: string;
      provider: string;
      model: string;
      ready: boolean;
      routing: string;
      family: string;
      family_label?: string;
      per_second?: number;
      key_rejected?: boolean;
      no_credit?: boolean;
      note?: string;
    };
    voice: {
      provider: string;
      service: string;
      ready: boolean;
      options: { id: string; label: string; ready: boolean }[];
    };
  };
  type Service = {
    provider: string;
    label: string;
    per_second: number;
    usable: boolean;
    in_use: boolean;
    cheapest: boolean;
  };
  type Services = { families: { id: string; label: string }[]; model: string; routing: string; services: Service[] };

  let { onopen }: { onopen?: (section: string) => void } = $props();
  let setup = $state<Setup | null>(null),
    catalog = $state<Record<string, { models: string[]; efforts: string[] }>>({}),
    services = $state<Services | null>(null);
  let error = $state(''),
    busy = $state('');
  // Sign-in state of the local AI tools (Codex, Claude Code) that run the assistant.
  let tools = $state<any>(null),
    connectOpen = $state(false),
    connectChecked = false;
  const connectedTools = $derived((tools?.engines ?? []).filter((e: any) => e.signed_in).length);
  // The paid picture service saved in settings; 'openai' means an OpenAI key decides it.
  let pictureService = $state('');
  const engineName: Record<string, string> = { claude: 'Claude Code', codex: 'Codex' };
  const select = 'h-9 min-w-0 max-w-full rounded-md border border-input bg-background px-2 text-sm';

  async function load() {
    try {
      let app: { default_image_provider?: string } | null;
      [setup, catalog, app, tools] = await Promise.all([
        get('/ai/setup'),
        get('/director/models'),
        get('/settings/app').catch(() => null),
        get('/director/status').catch(() => null)
      ]);
      if (!connectChecked) {
        connectChecked = true;
        connectOpen = !connectedTools;
      }
      pictureService = app?.default_image_provider ?? '';
      services = await get('/video-services' + (setup?.video.family ? '?model=' + setup.video.family : ''));
      error = '';
    } catch (e) {
      error = errorMessage(e);
    }
  }
  async function change(what: string, action: () => Promise<unknown>) {
    busy = what;
    error = '';
    try {
      await action();
      await load();
    } catch (e) {
      error = errorMessage(e);
    }
    busy = '';
  }
  const assistant = (patch: { model?: string; effort?: string }) =>
    change('assistant', () =>
      put(`/director/sessions/${setup!.assistant.session_id}/model`, {
        model: setup!.assistant.model ?? '',
        effort: setup!.assistant.effort ?? '',
        ...patch
      })
    );
  const video = (body: { mode: string; model: string; provider?: string }) =>
    change('video', () => put('/video-services/routing', body));
  // Check again on an empty account: the service is asked when it can be, otherwise the next video decides.
  const checkVideoAgain = () =>
    change('check', async () => {
      const r = await post(`/settings/providers/${setup!.video.provider}/test`);
      if (!r.ok && r.error) throw new Error(r.error);
    });
  const pictures = (provider: string) =>
    change('pictures', () => put('/settings/app', { default_image_provider: provider }));
  const voice = (provider: string) => change('voice', () => put('/voice/preferences', { primary_provider: provider }));
  onMount(() => void load());
  const usable = $derived(services?.services.filter(s => s.usable) ?? []);
</script>

{#snippet row(Icon: typeof Image, job: string, what: string, ready: boolean, problem: string)}
  <div class="grid gap-1">
    <div class="flex items-center gap-2">
      <Icon class="size-4 shrink-0 text-muted-foreground" />
      <h3 class="text-sm font-medium">{job}</h3>
      {#if ready}<Badge variant="outline" class="gap-1"><Check class="size-3" />Ready</Badge>{:else}<Badge
        variant="destructive" class="gap-1"><TriangleAlert class="size-3" />{problem}</Badge>{/if}
    </div>
    <p class="text-xs text-muted-foreground">{what}</p>
  </div>
{/snippet}

<section class="grid gap-4" aria-labelledby="ai-setup">
  <div class="grid gap-1">
    <h2 id="ai-setup" class="text-base font-semibold">AI setup</h2>
    <p class="text-sm text-muted-foreground">Which AI does each job right now. Changes here save straight away and become the default for new work.</p>
  </div>
  {#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}

  {#if !setup}
    <div
      role="status"
      aria-busy="true"
      aria-label="Loading AI setup"
      class="grid gap-3">{#each Array(5) as _, i (i)}<Skeleton class="h-20 w-full" />{/each}</div>
  {:else}
    <ul class="grid divide-y divide-border rounded-lg border border-border">
      <li class="grid gap-3 p-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)] lg:items-center">
        {@render row(MessageSquare, 'Writing and planning', 'The assistant you chat with. It writes stories, plans shots and answers questions.', setup.assistant.ready, setup.assistant.engine ? 'Signed out' : 'No conversation')}
        <div class="grid gap-2">
          {#if setup.assistant.engine}
            <p class="text-sm"><span
              class="font-medium">{engineName[setup.assistant.engine] ?? setup.assistant.engine}</span> <span
              class="text-muted-foreground">· conversation "{setup.assistant.title}"</span></p>
            <div class="flex flex-wrap gap-2">
              <label class="grid gap-1 text-xs text-muted-foreground">Model
                <select
                  class={select}
                  disabled={busy === 'assistant'}
                  value={setup.assistant.model ?? ''}
                  onchange={(e) => assistant({ model: e.currentTarget.value })}>
                  <option value="">Default for {engineName[setup.assistant.engine] ?? 'this AI'}</option>
                  {#each catalog[setup.assistant.engine]?.models ?? [] as m (m)}<option value={m}>{m}</option>{/each}
                </select>
              </label>
              <label class="grid gap-1 text-xs text-muted-foreground">Thinking effort
                <select
                  class={select}
                  disabled={busy === 'assistant'}
                  value={setup.assistant.effort ?? ''}
                  onchange={(e) => assistant({ effort: e.currentTarget.value })}>
                  <option value="">Default</option>
                  {#each catalog[setup.assistant.engine]?.efforts ?? [] as f (f)}<option value={f}>{f}</option>{/each}
                </select>
              </label>
            </div>
            <p class="text-xs text-muted-foreground">Applies to the open conversation. To use {setup.assistant.engine === 'claude' ? 'Codex' : 'Claude Code'} instead, start a new conversation from the picker above the chat. Its characters and reference pictures are chosen there too.</p>
          {:else}
            <p class="text-sm text-muted-foreground">Open the chat and start a conversation to choose its AI.</p>
          {/if}
        </div>
        <details id="connect" bind:open={connectOpen} class="rounded-md border border-border px-3 lg:col-span-2">
          <summary class="min-h-11 cursor-pointer py-3 text-sm font-medium">Connect Codex or Claude Code <span
            class="font-normal text-muted-foreground">· {(tools?.engines ?? [])
                .map(
                  (e: any) =>
                    `${engineName[e.engine] ?? e.engine}: ${e.signed_in ? 'connected' : e.installed ? 'not connected' : 'unavailable'}`
                )
                .join(' · ') || 'status unavailable'}</span></summary>
          <div class="grid gap-4 pb-4 text-sm">
            <p class="max-w-prose text-muted-foreground">The assistant runs on Codex or Claude Code with your own account. Connect at least one; you only need to do this once.</p>
            {#if tools?.all_text_local === false}<p role="alert" class="text-destructive">Local planning is disabled in the backend configuration.</p>{/if}
            <AgentConnect engines={tools?.engines ?? []} highlight={setup.assistant.engine ?? ''} onchange={() => void load()} />
            <details>
              <summary class="cursor-pointer py-2 text-muted-foreground">Prefer the terminal?</summary>
              <p class="mt-2 max-w-prose text-muted-foreground">On the computer hosting VideoFlow, run one of these from the project folder:</p>
              <pre class="mt-3 overflow-x-auto rounded-md border border-border bg-muted/30 p-3 text-xs leading-relaxed"><code>docker compose exec backend codex login --device-auth
docker compose exec backend claude auth login</code></pre>
              <p class="mt-3 max-w-prose text-muted-foreground">Sign-in is stored in the backend, not your browser, so a login on your own computer doesn't count.</p>
              {#if tools?.host_requirement}<p class="mt-3 max-w-prose text-muted-foreground">{tools.host_requirement}</p>{/if}
              {#each tools?.engines ?? [] as item (item.engine)}{#if item.reason}<p
                class="mt-2 break-words text-muted-foreground">{engineName[item.engine] ?? item.engine}: {item.reason}</p>{/if}{/each}
            </details>
          </div>
        </details>
      </li>

      <li class="grid gap-3 p-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)] lg:items-center">
        {@render row(Image, 'Pictures', 'Character, place and storyboard pictures.', setup.pictures.ready, 'Needs a key')}
        <div class="grid gap-1">
          <p class="text-sm"><span class="font-medium">{setup.pictures.service}</span> <span class="text-muted-foreground">· {setup.pictures.model}</span></p>
          {#if setup.pictures.note}<p class="text-xs text-muted-foreground">{setup.pictures.note}</p>{/if}
          {#if pictureService && pictureService !== 'openai'}
            <label class="grid gap-1 text-xs text-muted-foreground">{setup.pictures.service === 'Codex' ? 'Service when Codex is not used' : 'Service'}
              <select class={select + ' w-64'} disabled={busy === 'pictures'} value={pictureService} onchange={(e) => pictures(e.currentTarget.value)}>
                <option value="fal">fal.ai</option><option value="openrouter">OpenRouter</option>
              </select>
            </label>
          {/if}
          <p class="text-xs text-muted-foreground">Codex makes the pictures when the conversation runs on Codex, and OpenAI whenever its key is saved. Otherwise the service chosen here is used.</p>
        </div>
      </li>

      <li class="grid gap-3 p-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)] lg:items-center">
        {@render row(Clapperboard, 'Video', 'Turns each scene into a moving clip. This is where most of the money goes.', setup.video.ready, setup.video.key_rejected ? 'Key rejected' : setup.video.no_credit ? 'No balance' : 'Needs a key')}
        <div class="grid gap-2">
          {#if setup.video.key_rejected && setup.video.note}<p class="text-xs text-destructive" role="status">{setup.video.note} <button
            type="button"
            class="underline"
            onclick={() => onopen?.('connections')}>Open AI connections</button></p>{/if}
          {#if setup.video.no_credit && setup.video.note}<p class="text-xs text-destructive" role="status">{setup.video.note} <button
            type="button"
            class="underline"
            disabled={busy === 'check'}
            onclick={checkVideoAgain}>{busy === 'check' ? 'Checking…' : 'Check again'}</button></p>{/if}
          <p class="text-sm"><span class="font-medium">{setup.video.service}</span> <span
            class="text-muted-foreground">· {setup.video.family_label ?? setup.video.model}{setup.video.per_second ? ` · $${setup.video.per_second.toFixed(3)} per second at 720p` : ''}</span></p>
          {#if services}
            <div class="flex flex-wrap items-end gap-2">
              <label class="grid gap-1 text-xs text-muted-foreground">Model
                <select
                  class={select}
                  disabled={busy === 'video'}
                  value={services.model}
                  onchange={(e) => video({ mode: services!.routing === 'cheapest' ? 'cheapest' : 'manual', model: e.currentTarget.value })}>
                  {#each services.families as f (f.id)}<option value={f.id}>{f.label}</option>{/each}
                </select>
              </label>
              <label class="grid gap-1 text-xs text-muted-foreground">Service
                <select
                  class={select}
                  disabled={busy === 'video'}
                  value={services.routing === 'cheapest' ? 'cheapest' : (usable.find((s) => s.in_use)?.provider ?? '')}
                  onchange={e =>
                    e.currentTarget.value === 'cheapest'
                      ? video({ mode: 'cheapest', model: services!.model })
                      : video({ mode: 'manual', model: services!.model, provider: e.currentTarget.value })}>
                  <option value="cheapest">Cheapest connected (automatic)</option>
                  {#if services.routing !== 'cheapest' && !usable.some((s) => s.in_use)}<option value="">{setup.video.service} (current)</option>{/if}
                  {#each usable as s (s.provider)}<option value={s.provider}>{s.label} · ${s.per_second.toFixed(3)}/s</option>{/each}
                </select>
              </label>
            </div>
          {/if}
          <p class="text-xs text-muted-foreground">Only services with a saved key are listed. <a
            class="underline" href="/costs#services">Compare all prices</a></p>
        </div>
      </li>

      <li class="grid gap-3 p-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)] lg:items-center">
        {@render row(AudioLines, 'Voice', 'Speaks narration and character lines.', setup.voice.ready, 'Needs a key')}
        <div class="grid gap-2">
          <label class="grid gap-1 text-xs text-muted-foreground">Service
            <select class={select + ' w-64'} disabled={busy === 'voice'} value={setup.voice.provider} onchange={(e) => voice(e.currentTarget.value)}>
              {#each setup.voice.options as o (o.id)}<option value={o.id} disabled={!o.ready}>{o.label}{o.ready ? '' : ' (no key saved)'}</option>{/each}
            </select>
          </label>
          <p class="text-xs text-muted-foreground">Each character's own voice is chosen in <a class="underline" href="/voices">Voices</a>.</p>
        </div>
      </li>

      <li class="grid gap-3 p-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)] lg:items-center">
        {@render row(Eye, 'Looking at pictures', 'Checks pictures for mistakes and reads a look from a reference you upload.', setup.vision.ready, 'Needs a key')}
        <div class="grid gap-1">
          <p class="text-sm"><span class="font-medium">{setup.vision.service}</span> <span class="text-muted-foreground">· {setup.vision.model}</span></p>
          {#if setup.vision.note}<p class="text-xs text-muted-foreground">{setup.vision.note}</p>{/if}
        </div>
      </li>

      <li class="grid gap-3 p-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)] lg:items-center">
        {@render row(Mic, 'Listening to audio', 'Turns speech into text for subtitles.', setup.listening.ready, 'Needs a key')}
        <div class="grid gap-1">
          <p class="text-sm"><span class="font-medium">{setup.listening.service}</span> <span class="text-muted-foreground">· {setup.listening.model}</span></p>
          {#if setup.listening.note}<p class="text-xs text-muted-foreground">{setup.listening.note}</p>{/if}
        </div>
      </li>
    </ul>
    <div class="flex flex-wrap gap-2">
      <Button variant="outline" size="sm" onclick={() => onopen?.('connections')}>Add or change keys</Button>
      <Button variant="ghost" size="sm" onclick={() => onopen?.('advanced')}>Exact model names</Button>
    </div>
  {/if}
</section>
