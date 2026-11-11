<script lang="ts">
  import { onMount } from 'svelte';
  import { get, post, put } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import { Card, CardContent, CardHeader, CardTitle } from '$lib/components/ui/card';
  type Session = {id:string; engine:string; native_id?:string; title:string; status:string; selected:boolean};
  type Turn = {id:string;kind:string;status:string;request_json:unknown;result_json:unknown;error?:string;created_at:string};
  let status:any=$state(null), sessions:Session[]=$state([]), turns:Turn[]=$state([]);
  let characters:any[]=$state([]), assets:any[]=$state([]), characterIDs:string[]=$state([]), assetIDs:string[]=$state([]);
  let engine=$state('codex'), title=$state(''), busy=$state(false), loaded=$state(false), error=$state(''), notice=$state('');
  let warnings:string[]=$state([]);
  const selected=$derived(sessions.find(s=>s.selected));
  const json=(value:unknown)=>typeof value==='string'?value:JSON.stringify(value,null,2);
  async function detail(id:string){
    const [history,selection]=await Promise.all([get(`/director/sessions/${id}/turns`),get(`/director/sessions/${id}/selection`)]);
    turns=history??[];characterIDs=selection?.character_ids??[];assetIDs=selection?.asset_ids??[];warnings=[];
  }
  async function refresh(){
    const [health,list,cast,media]=await Promise.all([get('/director/status'),get('/director/sessions'),get('/characters'),get('/assets')]);
    status=health;sessions=list??[];characters=cast??[];assets=media??[];
    const current=sessions.find(s=>s.selected);if(current)await detail(current.id);else{turns=[];characterIDs=[];assetIDs=[];}
  }
  async function run(fn:()=>Promise<void>){busy=true;error='';notice='';try{await fn();}catch(e:any){error=e.message??String(e);}finally{busy=false;}}
  onMount(()=>{void run(refresh).finally(()=>loaded=true);});
  async function create(event:SubmitEvent){event.preventDefault();await run(async()=>{await post('/director/sessions',{engine,title:title.trim()});title='';await refresh();notice='Local session created. Its native conversation starts with the first planning or writing turn.';});}
  async function choose(session:Session){await run(async()=>{await post(`/director/sessions/${session.id}/select`,{});await refresh();});}
  async function saveContext(){if(!selected)return;const id=selected.id;await run(async()=>{const snapshot=await put(`/director/sessions/${id}/selection`,{character_ids:characterIDs,asset_ids:assetIDs});warnings=snapshot.warnings??[];notice='Context selection saved for future turns.';});}
  async function cancel(){if(!selected)return;const id=selected.id;await run(async()=>{await post(`/director/sessions/${id}/cancel`,{});notice='Cancellation requested. Refresh to review the final turn status.';});}
  async function acknowledge(){if(!selected)return;const id=selected.id;await run(async()=>{await post(`/director/sessions/${id}/acknowledge`,{});await refresh();notice='Review acknowledged. This session can continue.';});}
</script>

<div class="mx-auto w-full max-w-6xl space-y-5 p-4 sm:p-6">
  <div class="flex flex-wrap items-start justify-between gap-3">
    <div><h1 class="text-lg font-semibold">Local sessions</h1><p class="text-sm text-muted-foreground">Plan and write through Claude Code or Codex on your backend host.</p></div>
    <Button variant="outline" disabled={busy} onclick={()=>run(refresh)}>Refresh sessions</Button>
  </div>
  {#if error}<p role="alert" class="rounded-lg border border-destructive p-3 text-sm text-destructive">{error}</p>{/if}
  {#if notice}<p role="status" class="rounded-lg border p-3 text-sm">{notice}</p>{/if}
  {#if !loaded}<p class="text-sm text-muted-foreground">Loading local sessions…</p>{/if}
  {#if status}
    <Card><CardHeader><CardTitle>Backend host setup</CardTitle></CardHeader><CardContent class="space-y-3 text-sm">
      <p>{status.host_requirement}</p>
      <p>Install the official Claude Code or Codex CLI on that host, then sign in from its terminal using your own account. VideoFlow does not install software or sign in for you. In a container, configure access to the host CLI and its credentials explicitly.</p>
      <p>CLI detection checks the executable only. It does not verify account authentication, subscription access, or model availability.</p>
      <div class="grid gap-2 sm:grid-cols-2">{#each status.engines??[] as item(item.engine)}<div class="rounded-lg border p-3"><strong>{item.engine==='claude'?'Claude Code':'Codex'}</strong>: {item.installed?'CLI detected':'CLI unavailable'}{#if item.reason}<p class="mt-1 break-words text-muted-foreground">{item.reason}</p>{/if}</div>{/each}</div>
      <p>Text routing: {status.all_text_local?'local sessions enabled':'local sessions disabled; check backend configuration'}. Voice default: saved character voice.</p>
    </CardContent></Card>
  {/if}
  <div class="grid gap-5 lg:grid-cols-[minmax(240px,1fr)_minmax(0,2fr)]">
    <div class="space-y-4">
      <Card><CardHeader><CardTitle>Create local session</CardTitle></CardHeader><CardContent>
        <form class="space-y-3" onsubmit={create}>
          <label class="block text-sm" for="session-engine">Engine</label><select id="session-engine" class="w-full rounded-md border bg-background p-2" bind:value={engine} disabled={busy}><option value="codex">Codex</option><option value="claude">Claude Code</option></select>
          <label class="block text-sm" for="session-title">Session title</label><input id="session-title" class="w-full rounded-md border bg-background p-2" bind:value={title} placeholder="Story planning" maxlength="120" disabled={busy}/>
          <Button type="submit" disabled={busy}>Create session</Button>
          <p class="text-xs text-muted-foreground">Creates a project session and selects it. A native conversation ID is assigned after its first CLI turn.</p>
        </form>
      </CardContent></Card>
      <section aria-label="Project sessions" class="space-y-2">
        <h2 class="text-sm font-semibold">Project sessions</h2>
        {#if loaded&&!sessions.length}<p class="text-sm text-muted-foreground">No local sessions yet.</p>{/if}
        {#each sessions as session(session.id)}<button type="button" class="w-full rounded-lg border p-3 text-left hover:bg-accent disabled:opacity-50" aria-pressed={session.selected} disabled={busy} onclick={()=>choose(session)}><span class="block break-words font-medium">{session.title||'Untitled session'}{session.selected?' · Selected':''}</span><span class="text-xs text-muted-foreground">{session.engine} · {session.status}</span></button>{/each}
      </section>
    </div>
    {#if selected}
      <div class="min-w-0 space-y-4">
        <Card><CardHeader><CardTitle>{selected.title||'Selected session'}</CardTitle></CardHeader><CardContent class="space-y-3 text-sm">
          <p>Status: <strong>{selected.status}</strong></p>
          <p class="break-all">Native conversation: {selected.native_id||'Not started — assigned after the first CLI turn.'}</p>
          {#if selected.status==='running'}<Button variant="destructive" disabled={busy} onclick={cancel}>Cancel active turn</Button>{/if}
          {#if selected.status==='review_required'}<div class="space-y-2 rounded-lg border p-3"><p>Review the latest request, result and error below before continuing. Cancellation or interruption may leave partial work in the native conversation.</p><Button disabled={busy} onclick={acknowledge}>I reviewed the turn — allow continuation</Button></div>{/if}
          <p>Start planning or writing from Studio or Scenes. History below is read-only and cannot replay old actions.</p>
        </CardContent></Card>
        <Card><CardHeader><CardTitle>Context for future turns</CardTitle></CardHeader><CardContent class="space-y-3">
          <p class="text-sm text-muted-foreground">Choose project characters and assets, then save. Existing turns keep their recorded context.</p>
          <fieldset class="space-y-2" disabled={busy||selected.status==='running'}><legend class="mb-2 text-sm font-medium">Characters</legend>{#if !characters.length}<p class="text-sm text-muted-foreground">No characters in this project.</p>{/if}{#each characters as character(character.id)}<label class="flex items-start gap-2 text-sm"><input type="checkbox" class="mt-1" bind:group={characterIDs} value={character.id}/><span class="break-words">{character.name||character.id}</span></label>{/each}</fieldset>
          <fieldset class="space-y-2" disabled={busy||selected.status==='running'}><legend class="mb-2 text-sm font-medium">Assets</legend>{#if !assets.length}<p class="text-sm text-muted-foreground">No assets in this project.</p>{/if}{#each assets as asset(asset.id)}<label class="flex items-start gap-2 text-sm"><input type="checkbox" class="mt-1" bind:group={assetIDs} value={asset.id}/><span class="break-words">{asset.name||asset.id} <span class="text-muted-foreground">({asset.type})</span></span></label>{/each}</fieldset>
          <Button disabled={busy||selected.status==='running'} onclick={saveContext}>Save context selection</Button>
          {#each warnings as warning}<p class="text-sm text-amber-700">{warning}</p>{/each}
        </CardContent></Card>
        <section aria-label="Native turn history" class="space-y-2"><h2 class="text-sm font-semibold">Native turn history</h2>{#if !turns.length}<p class="text-sm text-muted-foreground">No turns yet. Planning and writing will appear here.</p>{/if}
          {#each turns as turn(turn.id)}<details class="rounded-lg border p-3"><summary class="cursor-pointer break-words text-sm font-medium">{turn.kind} · {turn.status} · {turn.created_at}</summary><div class="mt-3 space-y-3 text-sm">{#if turn.error}<p role="alert" class="break-words text-destructive">{turn.error}</p>{/if}<h3 class="font-medium">Recorded request and context</h3><pre class="max-h-96 overflow-auto whitespace-pre-wrap break-words rounded bg-muted p-3 text-xs">{json(turn.request_json)}</pre><h3 class="font-medium">Recorded result</h3><pre class="max-h-96 overflow-auto whitespace-pre-wrap break-words rounded bg-muted p-3 text-xs">{json(turn.result_json)}</pre></div></details>{/each}
        </section>
      </div>
    {:else if loaded}<p class="text-sm text-muted-foreground">Create or select a session to review its history and context.</p>{/if}
  </div>
</div>
