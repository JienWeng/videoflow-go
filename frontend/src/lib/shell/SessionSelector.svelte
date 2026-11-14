<script lang="ts">
  import { onMount } from 'svelte';
  import { get, post } from '$lib/api';
  import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import { Button } from '$lib/components/ui/button';
  let {compact=false}:{compact?:boolean}=$props();
  let sessions:any[]=$state([]), engines:any[]=$state([]), selected=$state(''), engine=$state('codex'), busy=$state(false), error=$state(''), loaded=$state(false);
  async function refresh(){const [list,status]=await Promise.all([get('/director/sessions'),get('/director/status')]);sessions=list;engines=status.engines??[];selected=list.find((s:any)=>s.selected)?.id??'';}
  async function run(fn:()=>Promise<void>){busy=true;error='';try{await fn();window.dispatchEvent(new Event('videoflow-session-changed'));}catch(e){error=String(e);}finally{busy=false;}}
  onMount(()=>{refresh().catch(e=>error=String(e)).finally(()=>loaded=true)});
  const available=$derived(engines.some(e=>e.engine===engine&&e.installed));
</script>
<div class="space-y-2" data-testid="session-selector">
  <div class="flex flex-wrap items-center gap-2">
    <DropdownMenu.Root>
      <DropdownMenu.Trigger class="flex min-h-10 min-w-0 flex-1 items-center justify-between gap-2 rounded-lg border border-input bg-background px-3 text-left text-sm focus-visible:outline-2 focus-visible:outline-ring disabled:opacity-50" aria-label="AI session" disabled={busy||!loaded}>
        <span class="truncate">{!loaded?'Loading sessions…':sessions.find(s=>s.id===selected)?`${sessions.find(s=>s.id===selected).title||'Untitled'} · ${sessions.find(s=>s.id===selected).engine}`:'Choose AI session'}</span><ChevronDown class="size-4 shrink-0" />
      </DropdownMenu.Trigger>
      <DropdownMenu.Content align="start" class="max-w-[min(420px,90vw)]">
        <DropdownMenu.RadioGroup value={selected} onValueChange={id=>{if(id!==selected)void run(async()=>{await post(`/director/sessions/${id}/select`,{});await refresh();});}}>
          {#each sessions as session}<DropdownMenu.RadioItem value={session.id} closeOnSelect><span class="min-w-0"><span class="block truncate">{session.title||'Untitled'}</span><span class="block text-xs text-muted-foreground">{session.engine} · {session.status}</span></span></DropdownMenu.RadioItem>{/each}
        </DropdownMenu.RadioGroup>
        {#if !sessions.length}<p class="px-2 py-2 text-sm text-muted-foreground">No saved AI sessions</p>{/if}
        <DropdownMenu.Separator /><DropdownMenu.Item>{#snippet child({props})}<a {...props} href="/director">Manage AI sessions</a>{/snippet}</DropdownMenu.Item>
      </DropdownMenu.Content>
    </DropdownMenu.Root>
    {#if !compact}<a href="/director" class="text-xs underline underline-offset-4">Manage</a>{/if}
  </div>
  {#if loaded&&!selected}
    <div class="flex flex-wrap items-center gap-2">
      <select aria-label="New session engine" bind:value={engine} class="min-h-10 rounded-lg border border-input bg-background px-2 text-sm" disabled={busy}><option value="codex">Codex</option><option value="claude">Claude Code</option></select>
      <Button size="sm" disabled={busy||!available} onclick={()=>run(async()=>{await post('/director/sessions',{engine,title:'Story planning'});await refresh()})}>{busy?'Creating…':'Start session'}</Button>
    </div>
    <p class="text-xs text-muted-foreground">{available?'CLI detected. Account authentication is checked on your first message.':'Install and sign in to the selected CLI on the server to start planning.'}</p>
  {/if}
  {#if error}<p role="alert" class="text-xs text-destructive">{error} <button class="underline" onclick={()=>run(refresh)}>Retry</button></p>{/if}
</div>
