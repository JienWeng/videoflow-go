<script lang="ts">
  import { errorMessage } from '$lib/errors';
  import { onDestroy } from 'svelte';
  import { get, post } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import { Check, Copy, ExternalLink, LoaderCircle } from '@lucide/svelte';

  type Login = { status: string; url?: string; code?: string; needs_code?: boolean; message?: string };
  type Engine = { engine: string; installed: boolean; signed_in?: boolean; method?: string };
  /** An AI tool's sign-in state (GET /director/auth/{engine}). */
  type AuthState = { signed_in?: boolean; login?: Login | null };
  let { engines = [], highlight = '', onchange = () => {} }: { engines?: Engine[]; highlight?: string; onchange?: () => void } = $props();

  const NAMES: Record<string, string> = { codex: 'Codex', claude: 'Claude Code' };
  const ACCOUNTS: Record<string, string> = { codex: 'ChatGPT account', claude: 'Claude account (Pro or Max)' };
  let logins: Record<string, Login | null> = $state({});
  let pasted: Record<string, string> = $state({});
  let busy: Record<string, boolean> = $state({});
  let errors: Record<string, string> = $state({});
  let copied = $state('');
  let timers: Record<string, ReturnType<typeof setInterval>> = {};

  const active = (l: Login | null | undefined) => !!l && ['starting', 'waiting', 'verifying'].includes(l.status);

  function poll(engine: string) {
    clearInterval(timers[engine]);
    timers[engine] = setInterval(async () => {
      try {
        const state = await get<AuthState>(`/director/auth/${engine}`);
        logins[engine] = state.login ?? null;
        if (state.signed_in || !active(state.login)) {
          clearInterval(timers[engine]);
          if (state.signed_in) { logins[engine] = { status: 'complete', message: 'Connected.' }; onchange(); }
        }
      } catch { /* Keep polling; a restart shows up as a failed login. */ }
    }, 2000);
  }
  onDestroy(() => Object.values(timers).forEach(clearInterval));

  async function start(engine: string) {
    busy[engine] = true; errors[engine] = ''; pasted[engine] = '';
    try {
      const login = await post<Login>(`/director/auth/${engine}/start`, {});
      logins[engine] = login;
      if (active(login)) poll(engine);
      else errors[engine] = login.message ?? 'Sign-in could not start.';
    } catch (e) { errors[engine] = errorMessage(e, String(e)); }
    finally { busy[engine] = false; }
  }
  async function submit(engine: string, event: SubmitEvent) {
    event.preventDefault();
    if (!pasted[engine]?.trim()) return;
    busy[engine] = true; errors[engine] = '';
    try { const state = await post<AuthState>(`/director/auth/${engine}/code`, { code: pasted[engine] }); logins[engine] = state.login ?? null; poll(engine); }
    catch (e) { errors[engine] = errorMessage(e, String(e)); }
    finally { busy[engine] = false; }
  }
  async function cancel(engine: string) {
    clearInterval(timers[engine]);
    try { await post(`/director/auth/${engine}/cancel`, {}); } catch { /* The login may already have ended. */ }
    logins[engine] = null;
  }
  async function copy(text: string) {
    try { await navigator.clipboard.writeText(text); copied = text; setTimeout(() => copied = '', 2000); } catch { /* Code stays visible to type by hand. */ }
  }
</script>

<div class="grid gap-3 sm:grid-cols-2">
  {#each engines as item (item.engine)}
    {@const login = logins[item.engine]}
    <section
      class="rounded-lg border p-4 {highlight === item.engine && !item.signed_in ? 'border-primary' : 'border-border'}"
      aria-label={`${NAMES[item.engine]} connection`}>
      <div class="flex items-start justify-between gap-3">
        <div class="min-w-0">
          <h3 class="font-medium">{NAMES[item.engine] ?? item.engine}</h3>
          <p class="mt-0.5 text-xs text-muted-foreground">Uses your {ACCOUNTS[item.engine]}</p>
        </div>
        {#if item.signed_in}<span
          class="inline-flex shrink-0 items-center gap-1 rounded-full bg-emerald-500/15 px-2 py-0.5 text-xs font-medium text-emerald-700 dark:text-emerald-300"><Check
          class="size-3" />Connected</span>
        {:else if item.installed}<span class="shrink-0 rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground">Not connected</span>{/if}
      </div>

      {#if !item.installed}
        <p class="mt-3 text-sm text-muted-foreground">Not available on this server. The Docker version includes it; for a manual install, add the {NAMES[item.engine]} command-line tool.</p>
      {:else if active(login)}
        <ol class="mt-4 space-y-4 text-sm">
          <li>
            <p class="font-medium">1. Open the sign-in page</p>
            <Button class="mt-2 min-h-11" href={login!.url} target="_blank" rel="noopener noreferrer">Open sign-in page<ExternalLink class="size-4" /></Button>
          </li>
          {#if login!.code}
            <li>
              <p class="font-medium">2. Enter this code on that page</p>
              <div class="mt-2 flex items-center gap-2">
                <code class="rounded-md border border-border bg-muted/40 px-3 py-2 font-mono text-lg tracking-widest">{login!.code}</code>
                <Button
                  variant="ghost"
                  size="sm"
                  class="min-h-11"
                  onclick={() => copy(login!.code!)}
                  aria-label="Copy code">{#if copied === login!.code}<Check class="size-4" />Copied{:else}<Copy class="size-4" />Copy{/if}</Button>
              </div>
            </li>
            <li class="flex items-center gap-2 text-muted-foreground"><LoaderCircle
              class="size-4 animate-spin" />Waiting for you to approve… this page updates by itself.</li>
          {:else if login!.needs_code}
            <li>
              <p class="font-medium">2. Paste the code shown after you sign in</p>
              <form class="mt-2 flex flex-wrap gap-2" onsubmit={e => submit(item.engine, e)}>
                <input
                  class="min-h-11 min-w-0 flex-1 rounded-md border border-input bg-background px-3 font-mono text-sm"
                  aria-label={`${NAMES[item.engine]} sign-in code`}
                  placeholder="Paste code"
                  autocomplete="off"
                  spellcheck="false"
                  bind:value={pasted[item.engine]}
                  disabled={login!.status === 'verifying' || busy[item.engine]} />
                <Button
                  type="submit"
                  class="min-h-11"
                  disabled={!pasted[item.engine]?.trim() || login!.status === 'verifying' || busy[item.engine]}>{login!.status === 'verifying' ? 'Checking…' : 'Connect'}</Button>
              </form>
            </li>
          {/if}
        </ol>
        <Button variant="ghost" size="sm" class="mt-3" onclick={() => cancel(item.engine)}>Cancel</Button>
      {:else}
        {#if item.signed_in}
          <p class="mt-3 text-sm text-muted-foreground">{item.method ? `Signed in with ${item.method}. ` : ''}Ready for story planning.</p>
        {:else}
          <p class="mt-3 text-sm text-muted-foreground">Sign in once and VideoFlow remembers it, even after a restart. It takes about a minute.</p>
        {/if}
        {#if login?.status === 'complete' && item.signed_in}<p role="status" class="mt-2 text-sm">Connected. You can start planning.</p>{/if}
        {#if login?.status === 'failed'}<p role="alert" class="mt-2 text-sm text-destructive">{login.message}</p>{/if}
        <Button
          class="mt-3 min-h-11"
          variant={item.signed_in ? 'outline' : 'default'}
          disabled={busy[item.engine]}
          onclick={() => {
            if (
              item.signed_in &&
              !window.confirm(
                `Switching signs ${NAMES[item.engine]} out of the current account right away, even if you cancel. Continue?`
              )
            )
              return;
            start(item.engine);
          }}>
          {busy[item.engine] ? 'Starting…' : item.signed_in ? 'Switch account' : `Connect ${NAMES[item.engine]}`}
        </Button>
      {/if}
      {#if errors[item.engine]}<p role="alert" class="mt-2 text-sm text-destructive">{errors[item.engine]}</p>{/if}
    </section>
  {/each}
</div>
