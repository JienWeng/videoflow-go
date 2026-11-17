<script lang="ts">
  import {get} from '$lib/api';
  import {Button} from '$lib/components/ui/button';
  import * as Dialog from '$lib/components/ui/dialog';
  import History from '@lucide/svelte/icons/history';
  let {disabled=false,oncontinue}:{disabled?:boolean;oncontinue:(id:string)=>Promise<void>}=$props();
  let open=$state(false),loading=$state(false),error=$state(''),sessions:any[]=$state([]),chosen=$state(''),turns:any[]=$state([]),switching=$state(false);
  let generation=0;
  async function loadTurns(id:string){const current=++generation;chosen=id;turns=[];error='';loading=true;try{const result=await get(`/director/sessions/${encodeURIComponent(id)}/turns`);if(current===generation)turns=result.filter((t:any)=>t.kind==='chat'||t.kind==='specialist_review');}catch(e){if(current===generation)error=String(e)}finally{if(current===generation)loading=false}}
  async function show(){open=true;error='';turns=[];sessions=[];loading=true;const current=++generation;try{const list=await get('/director/sessions');if(current!==generation)return;sessions=list;const id=list.find((s:any)=>s.selected)?.id??list[0]?.id;chosen=id??'';if(id)await loadTurns(id);}catch(e){if(current===generation)error=String(e)}finally{if(current===generation)loading=false}}
  function changeOpen(value:boolean){if(switching)return;open=value;if(!value){generation++;loading=false}}
  async function resume(){switching=true;error='';try{await oncontinue(chosen);open=false;generation++;}catch(e){error=String(e)}finally{switching=false}}
</script>
<Button variant="ghost" size="icon" class="size-9 shrink-0" aria-label="Conversation history" title="Conversation history" {disabled} onclick={show}><History class="size-4" /></Button>
<Dialog.Root {open} onOpenChange={changeOpen}>
  <Dialog.Content class="flex max-h-[85dvh] flex-col overflow-hidden sm:max-w-2xl">
    <Dialog.Header><Dialog.Title>Conversation history</Dialog.Title><Dialog.Description>Browse saved conversations in this project. Opening a conversation does not rerun its actions.</Dialog.Description></Dialog.Header>
    {#if sessions.length}<label class="space-y-2 text-sm">Conversation<select aria-label="Saved conversation" class="w-full rounded-md border border-input bg-background p-2" value={chosen} disabled={switching} onchange={e=>loadTurns(e.currentTarget.value)}>{#each sessions as session}<option value={session.id}>{session.title||'Untitled'} · {session.engine}{session.selected?' · Current':''}</option>{/each}</select></label>{/if}
    <div class="min-h-0 flex-1 overflow-y-auto space-y-4 py-2" aria-label="Saved messages" aria-busy={loading}>
      {#if loading}<p role="status" class="text-sm text-muted-foreground">Loading conversation…</p>
      {:else if error}<div role="alert" class="space-y-2"><p class="text-sm">Could not load conversation. {error}</p><Button variant="outline" size="sm" onclick={()=>chosen?loadTurns(chosen):show()}>Retry history</Button></div>
      {:else if !sessions.length}<p class="text-sm text-muted-foreground">No saved AI conversations in this project yet.</p>
      {:else if !turns.length}<p class="text-sm text-muted-foreground">No saved chat messages in this conversation yet.</p>
      {:else}{#each turns as turn}<article class="space-y-3 border-b border-border pb-4 text-sm">{#if turn.request_json?.user_message}<div><p class="mb-1 font-semibold">You</p><p class="whitespace-pre-wrap break-words">{turn.request_json.user_message}</p></div>{/if}{#if turn.result_json?.reply||turn.error}<div><p class="mb-1 font-semibold">Story assistant</p><p class="whitespace-pre-wrap break-words">{turn.result_json?.reply||turn.error}</p></div>{:else}<p class="text-muted-foreground">{turn.status==='running'?'Response in progress…':'No saved response.'}</p>{/if}</article>{/each}{/if}
    </div>
    {#if sessions.length}<Button disabled={loading||switching||!!error||!chosen} onclick={resume}>{switching?'Opening…':'Open in chat'}</Button>{/if}
  </Dialog.Content>
</Dialog.Root>
