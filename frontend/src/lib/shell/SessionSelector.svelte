<script lang="ts">
  import { onMount } from 'svelte';
  import { get, post } from '$lib/api';
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
    <label for="workspace-session" class={compact?'sr-only':'text-xs font-medium text-muted-foreground'}>AI session</label>
    <select id="workspace-session" aria-label="AI session" class="min-h-10 max-w-full flex-1 rounded-lg border border-input bg-background px-2 text-sm" value={selected} disabled={busy||!loaded} onchange={e=>run(async()=>{await post(`/director/sessions/${e.currentTarget.value}/select`,{});await refresh()})}>
      <option value="" disabled>{loaded?'Choose a session':'Loading sessions…'}</option>
      {#each sessions as s}<option value={s.id}>{s.title||'Untitled'} · {s.engine} · {s.status}</option>{/each}
    </select>
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
