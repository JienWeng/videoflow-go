<script lang="ts">
 import ReferenceGeneration from "./ReferenceGeneration.svelte";
 import FinishingPanel from "./FinishingPanel.svelte";
 import ChatPanel from "$lib/chat/ChatPanel.svelte";
 import NleTimeline from "./NleTimeline.svelte";
 import {Button} from "$lib/components/ui/button";
 import {Undo2,Redo2,Play,Pause,Scissors,Copy,Trash2,ZoomIn,ZoomOut,Download,Upload,Plus,Image as ImageIcon,Music,Type,Bot,SlidersHorizontal,ArrowLeft,SkipBack} from "@lucide/svelte";
 import {moveClip} from "$lib/composition/proposals.mjs";
 import CompositionPreview from '$lib/composition/CompositionPreview.svelte';
 import {defaults,splitClip,duplicateClip,trimStart,trimEnd,contentEnd} from '$lib/composition/edits.mjs';
 import {protectDraft} from '$lib/protect-draft';
 import {onMount} from 'svelte';
 import {get,post,upload,API_BASE,mediaUrl} from '$lib/api';
 type Clip={id:string;asset_id:string;start:number;trim:number;duration:number;speed:number;[key:string]:any};
 type Cue={id:string;utterance_id:string;start:number;end:number;text:string;source_clip_id?:string;character_id?:string;language?:string};
 type Draft={version:number;width:number;height:number;fps:number;duration:number;caption_style:string;clips:Clip[];audio:Clip[];captions:Cue[];[key:string]:any};
 let draft:Draft={version:1,width:640,height:360,fps:24,duration:2,caption_style:'default',clips:[],audio:[],captions:[]};
 let assets:any[]=[];let sourcesLoaded=false;let runtime:any=null;let error='';let log='';let busy=false;let revision='';let rendered='';let snapshots:string[]=[];
 $: visualSources=assets.filter(a=>a.media_kind==='image'||a.media_kind==='video');
 $: audioSources=assets.filter(a=>a.media_kind==='audio'||a.media_kind==='video');
 let mobilePanel="editor";let draftKey="";let draftReady=false;let dirty=false;
 protectDraft(()=>dirty||advancedDirty,'You have unsaved composition changes. Discard them and leave?');
 let savedEdits:any[]=[];let indexError='';
 async function refreshIndex(){try{savedEdits=await get('/compositions');indexError='';}catch(e){indexError='Could not load saved edits. '+String(e)}}
 let opening=true;let noticeTimer:any;$: if(notice&&!exportOpen){clearTimeout(noticeTimer);noticeTimer=setTimeout(()=>notice='',8000)}
 let autoStarted=false;let playhead=0;let activeClip='';let undoStack:string[]=[];let redoStack:string[]=[];let historyCurrent='';
 function resetHistory(){historyCurrent=JSON.stringify(draft);undoStack=[];redoStack=[]}
 function record(){const now=JSON.stringify(draft);if(historyCurrent&&now!==historyCurrent){undoStack=[...undoStack.slice(-79),historyCurrent];redoStack=[]}historyCurrent=now}
 function travel(redo=false){const from=redo?redoStack:undoStack;if(!from.length)return;const current=JSON.stringify(draft),next=from[from.length-1];if(redo){redoStack=redoStack.slice(0,-1);undoStack=[...undoStack,current]}else{undoStack=undoStack.slice(0,-1);redoStack=[...redoStack,current]}draft=JSON.parse(next);historyCurrent=next;markUnsaved();notice=redo?'Edit redone.':'Edit undone.'}
 function markUnsaved(){dirty=true;if(draftReady&&draftKey)try{sessionStorage.setItem(draftKey,JSON.stringify({draft,playhead,activeClip}))}catch{}revision='';rendered='';snapshots=[];log='';if(typeof window!=='undefined'){const url=new URL(window.location.href);url.searchParams.delete('revision');window.history.replaceState({},'',url)}}
 function editClip(index:number,operation:string){try{const clip=draft.clips[index];if(operation==='split'){if(draft.caption_bindings?.some((b:any)=>b.clip_id===clip.id))throw Error('Remove linked captions before splitting, then reimport approved captions for each new clip.');draft.clips=[...draft.clips.slice(0,index),...splitClip(clip,playhead,uid('clip')),...draft.clips.slice(index+1)]}else{const copy=duplicateClip(clip,uid('clip'));draft.clips=[...draft.clips.slice(0,index+1),copy,...draft.clips.slice(index+1)];draft.duration=Math.max(draft.duration,copy.start+copy.duration)}invalidate();error=''}catch(e){error=String(e)}}
 function adjust(index:number,key:string,value:string){const clip=draft.clips[index];clip.adjustments={...defaults(),...clip.adjustments,[key]:key==='fit'?value:Number(value)};invalidate()}
 let selectedClip='';let selectedAudio='';let advanced='';let advancedDirty=false;let notice='';
 const uid=(prefix:string)=>prefix+'-'+crypto.randomUUID();
 const assetName=(id:string)=>assets.find(a=>a.id===id)?.name??id;
 let historyQueued=false;
 function invalidate(){if(!historyQueued){historyQueued=true;queueMicrotask(()=>{historyQueued=false;record()})}markUnsaved();notice='';}
 async function sourceInfo(id:string){const result=await get('/compositions/sources/'+encodeURIComponent(id));assets=assets.map(a=>a.id===id?{...a,...result.source,...result}:a);return result}
 onMount(async()=>{void refreshIndex();try{let project:any;[assets,runtime,project]=await Promise.all([get('/compositions/sources'),get('/compositions/runtime'),get("/projects/active")]);sourcesLoaded=true;const params=new URL(window.location.href).searchParams;draftKey="videoflow.edit."+project.id+"."+(params.get("scene_id")??params.get("script_id")??params.get("source")??"default");const saved=params.get('revision');if(saved){draft=await get('/compositions/'+encodeURIComponent(saved));revision=draft.revision;notice='Saved edit loaded.';dirty=false;}
 resetHistory();const seed=params.get('source');const scene=params.get('scene_id');const story=params.get('script_id');if(!saved&&story){draft=await get('/scripts/'+encodeURIComponent(story)+'/composition-draft');resetHistory();invalidate();notice='Story scenes assembled in order. Add a continuous soundtrack below.';}else if(!saved&&seed){const item=assets.find(a=>a.id===seed);if(!item)throw Error('This source is unavailable in the active project.');await add(item.media_kind==='audio'?'audio':'clips',seed)}else if(!saved&&scene){draft=await get('/scenes/'+encodeURIComponent(scene)+'/composition-draft');resetHistory();invalidate();notice='Matching clips loaded in shot order with generation padding trimmed. Clip audio is preserved. Add a continuous soundtrack below.';}
 if(!saved){try{const recovered=JSON.parse(sessionStorage.getItem(draftKey)||'null');if(recovered?.draft?.version===1){draft=recovered.draft;playhead=recovered.playhead??0;activeClip=recovered.activeClip??'';dirty=true;notice='Unsaved edit restored.';resetHistory()}}catch{}}// Opening the editor with nothing chosen: continue the latest saved edit, otherwise
 // start from the story's finished scene videos so there is something to work with.
 if(!saved&&!seed&&!scene&&!story&&!draft.clips.length&&!draft.audio.length){await refreshIndex();try{if(savedEdits.length){const latest=[...savedEdits].sort((a,b)=>String(b.created_at).localeCompare(String(a.created_at)))[0];draft=await get('/compositions/'+encodeURIComponent(latest.revision));revision=draft.revision;dirty=false;notice='Your latest saved edit is open.';resetHistory();const url=new URL(window.location.href);url.searchParams.set('revision',revision);window.history.replaceState({},'',url)}else{const scripts=await get('/scripts');if(scripts?.length){draft=await get('/scripts/'+encodeURIComponent(scripts[0].id)+'/composition-draft');resetHistory();dirty=false;autoStarted=true;notice='Your finished scene videos are on the timeline in story order. Add music or captions, then Export.'}}}catch{/* nothing finished yet: start empty */}}
 if(!saved&&!draft.clips.length&&!draft.audio.length&&draft.width===640&&draft.height===360){try{const app=await get('/settings/app');const size=({'9:16':[1080,1920],'16:9':[1920,1080],'1:1':[1080,1080]} as Record<string,number[]>)[app?.default_aspect_ratio];if(size){draft={...draft,width:size[0],height:size[1]};resetHistory()}}catch{/* keep the built-in format */}}
 if(!activeClip)activeClip=draft.clips[0]?.id??'';draftReady=true;
 }catch(e){error=String(e)}finally{opening=false}});
 async function add(track:'clips'|'audio',id:string,at?:number){if(!id||busy)return;busy=true;error='';try{const info=await sourceInfo(id);const start=at!==undefined?Math.max(0,at):track==='clips'?Math.max(0,...draft.clips.map(c=>c.start+c.duration)):0;const duration=info.duration>0?info.duration:3;if(start+duration>600)throw Error('This source would exceed the 600-second composition limit. Trim it in a separate edit first.');draft.duration=!draft.clips.length&&!draft.audio.length?duration:Math.max(draft.duration,start+duration);const clip={id:uid(track==='audio'?'audio':'clip'),asset_id:id,start,trim:0,duration,speed:1,has_audio:info.has_audio,...(track==='audio'?{volume:0.15,fade_in:Math.min(0.4,duration/4),fade_out:Math.min(1,duration/4)}:{})};draft[track]=[...draft[track],clip];activeClip=clip.id;invalidate()}catch(e){error=String(e)}finally{busy=false}}

 async function uploadMusic(event:Event){const input=event.currentTarget as HTMLInputElement;const file=input.files?.[0];if(!file||busy)return;busy=true;error='';let id='';try{const form=new FormData();form.append('file',file);form.append('type','audio_reference');form.append('name',file.name);const asset=await upload('/assets/upload',form);id=asset.id;assets=await get('/compositions/sources');}catch(e){error=String(e)}finally{busy=false;input.value=''}if(id)await add('audio',id)}
 function remove(track:'clips'|'audio',index:number){if(track==='clips')removeLinkedCaptions(draft.clips[index].id);draft[track]=draft[track].filter((_,i)=>i!==index);invalidate()}
 function move(index:number,direction:number){const clips=[...draft.clips];[clips[index],clips[index+direction]]=[clips[index+direction],clips[index]];draft.clips=clips;invalidate()}
 function dimensions(value:string){[draft.width,draft.height]=value.split('x').map(Number);invalidate()}
 function addCaption(){draft.captions=[...draft.captions,{id:uid('cue'),utterance_id:'',start:0,end:Math.min(2,draft.duration),text:''}];invalidate()}
 function applyAdvanced(){try{const parsed=JSON.parse(advanced);if(!parsed||!Array.isArray(parsed.clips)||!Array.isArray(parsed.audio)||!Array.isArray(parsed.captions))throw Error('Include clips, audio and captions arrays.');draft=parsed;advancedDirty=false;error='';invalidate()}catch(e){error='Cannot apply advanced changes: '+String(e)}}
 function removeLinkedCaptions(clipID:string){draft.captions=draft.captions.filter(c=>c.source_clip_id!==clipID);draft.caption_bindings=(draft.caption_bindings??[]).filter((b:any)=>b.clip_id!==clipID);invalidate()}
 async function importCaptions(clip:Clip){busy=true;error='';try{const result=await post('/compositions/captions/import',{clip});draft.captions=[...draft.captions.filter(c=>c.source_clip_id!==clip.id),...result.captions];draft.caption_bindings=[...(draft.caption_bindings??[]).filter((b:any)=>b.clip_id!==clip.id),result.binding];invalidate();notice='Approved captions reused. They stay linked to the script, final audio and this clip timing.';}catch(e){error=String(e)}finally{busy=false}}
 async function save(){busy=true;error='';try{draft=await post('/compositions',draft);revision=draft.revision;historyCurrent=JSON.stringify(draft);dirty=false;advancedDirty=false;if(draftKey)sessionStorage.removeItem(draftKey);void refreshIndex();rendered='';notice='Edit saved. Preview or render when ready.';const url=new URL(window.location.href);url.searchParams.set('revision',revision);window.history.replaceState({},'',url);return revision;}catch(e){error=String(e);return undefined}finally{busy=false}}
 const plainError=(e:any)=>String(e).replace(/^\w*Error:\s*(\d{3}:\s*)?/,'');
 async function run(operation:string,retry=true):Promise<void>{busy=true;error='';try{const result=await post(`/compositions/${revision}/run`,{operation,revision});log=result.log;if(result.asset_url)rendered=API_BASE+result.asset_url;if(result.snapshots)snapshots=result.snapshots.map((p:string)=>API_BASE+p);notice=operation==='render'?'Rendered video added to your project assets.':'Review complete.';}catch(e){
  // Edits saved with an older renderer are refused; save the current edit again and retry once.
  if(retry&&/source changed/.test(String(e))){busy=false;const again=await save();if(again)return await run(operation,false);return}
  error=plainError(e)}finally{busy=false}}

 // ---- Editor shell state (layout, playback, selection) ----
 let playing=false;let zoom=60;let leftTab='media';let rightTab='inspect';let exportOpen=false;let mobileTab='media';
 type Track='clips'|'audio'|'captions';
 $: selTrack=(draft.clips.some(c=>c.id===activeClip)?'clips':draft.audio.some(c=>c.id===activeClip)?'audio':draft.captions.some(c=>c.id===activeClip)?'captions':'') as Track|'';
 $: selIndex=selTrack?draft[selTrack].findIndex((c:any)=>c.id===activeClip):-1;
 $: editTitle=revision?'Saved edit · '+(savedEdits.find(e=>e.revision===revision)?.created_at?new Date(savedEdits.find(e=>e.revision===revision).created_at).toLocaleDateString([], {month:'short',day:'numeric'}):revision.slice(0,8)):'New edit';
 const fmt=(t:number)=>`${Math.floor(t/60)}:${(t%60).toFixed(2).padStart(5,'0')}`;
 function fitLength(){draft.duration=Math.round(contentEnd(draft)*1000)/1000}
 function select(id:string,track:Track){activeClip=id;rightTab='inspect';mobileTab='inspect'}
 function onTimelineMove(track:Track,id:string,start:number){try{if(track==='captions'){const cue=draft.captions.find(c=>c.id===id);if(!cue||cue.source_clip_id)return;const len=cue.end-cue.start;const at=Math.max(0,Math.round(start*draft.fps)/draft.fps);draft.captions=draft.captions.map(c=>c.id===id?{...c,start:at,end:at+len}:c)}else{const clip=draft[track].find((c:any)=>c.id===id);if(clip)draft.duration=Math.max(draft.duration,start+clip.duration);draft=moveClip(draft,track,id,start,true)}fitLength();invalidate()}catch(e){error=String(e)}}
 function onTimelineTrim(track:Track,id:string,edge:'start'|'end',value:number){try{if(track==='captions'){draft.captions=draft.captions.map(c=>c.id!==id||c.source_clip_id?c:edge==='start'?{...c,start:Math.max(0,Math.min(c.end-1/draft.fps,value))}:{...c,end:Math.max(c.start+1/draft.fps,value)})}else{if(draft.caption_bindings?.some((b:any)=>b.clip_id===id))throw Error('Detach linked captions before trimming this clip.');const list=draft[track] as Clip[];const clip=list.find(c=>c.id===id);if(!clip)return;const src=assets.find(a=>a.id===clip.asset_id);const limit=src?.media_kind==='image'?0:src?.duration>0?(src.duration-clip.trim)/(clip.speed||1):0;const next=edge==='start'?trimStart(clip,value,draft.fps):trimEnd(clip,value,draft.fps,limit);draft[track]=list.map(c=>c.id===id?next:c)}fitLength();invalidate()}catch(e){error=String(e)}}
 async function onTimelineDrop(assetId:string,track:Track,start:number){if(track==='captions')return;await add(track,assetId,start);fitLength()}
 function deleteSelected(){if(!selTrack||selIndex<0)return;if(selTrack==='captions'){if(draft.captions[selIndex].source_clip_id){error='Linked captions are edited at their source. Remove them from the clip instead.';return}draft.captions=draft.captions.filter((_,j)=>j!==selIndex);invalidate()}else remove(selTrack,selIndex);activeClip='';fitLength()}
 function splitSelected(){if(selTrack==='clips')editClip(selIndex,'split');else if(selTrack==='audio'){try{const clip=draft.audio[selIndex];draft.audio=[...draft.audio.slice(0,selIndex),...splitClip(clip,playhead,uid('audio')),...draft.audio.slice(selIndex+1)];invalidate()}catch(e){error=String(e)}}}
 function duplicateSelected(){if(selTrack==='clips')editClip(selIndex,'duplicate');fitLength()}
 function addCaptionAt(){const start=Math.min(playhead,Math.max(0,draft.duration-0.5));const cue={id:uid('cue'),utterance_id:'',start,end:Math.min(start+2,Math.max(draft.duration,start+2)),text:'New caption'};draft.captions=[...draft.captions,cue];activeClip=cue.id;rightTab='inspect';fitLength();invalidate()}
 let scriptNote='';
 // Captions from each scene's script, placed at shot timing and shifted to where the clip sits.
 async function captionsFromScript(){if(busy)return;if(draft.captions.length&&!confirm('Replace the current captions with lines from your scripts? You can undo.'))return;busy=true;error='';scriptNote='';try{const r=await post('/compositions/captions/from-script',{clips:draft.clips});const skipped=(r.skipped??[]) as {reason:string}[];if(!r.captions?.length){scriptNote=skipped[0]?.reason??'No rendered scene clips with script lines in this edit. Add scene videos from Media first.';return}draft.captions=r.captions;draft.caption_bindings=[];fitLength();invalidate();scriptNote=`Added ${r.captions.length} caption${r.captions.length===1?'':'s'} from your script. Timing follows each shot; drag them on the Captions track to fine-tune.`+(skipped.length?` ${skipped.length} clip${skipped.length===1?' was':'s were'} skipped: ${skipped[0].reason}`:'')}catch(e){error=String(e)}finally{busy=false}}
 function dragAsset(e:DragEvent,asset:any){e.dataTransfer?.setData('application/x-videoflow-asset',asset.id);e.dataTransfer?.setData('application/x-videoflow-kind-'+asset.media_kind,'1');if(e.dataTransfer)e.dataTransfer.effectAllowed='copy'}
 function keys(e:KeyboardEvent){const t=e.target;if(t instanceof Element&&t.closest('input,textarea,select,[contenteditable="true"],[role="slider"]'))return;const mod=e.metaKey||e.ctrlKey;if(e.key===' '){e.preventDefault();playing=!playing}else if(mod&&e.key.toLowerCase()==='z'){e.preventDefault();travel(e.shiftKey)}else if(mod&&e.key.toLowerCase()==='s'){e.preventDefault();void save()}else if(!mod&&(e.key==='s'||e.key==='S')){splitSelected()}else if(e.key==='Delete'||e.key==='Backspace'){if(activeClip){e.preventDefault();deleteSelected()}}else if(e.key==='Home'){playhead=0}}
</script><svelte:head><title>Edit video · VideoFlow</title></svelte:head>
<svelte:window onkeydown={keys} />
<div class="editor flex min-h-full flex-col bg-background text-sm text-foreground lg:h-full lg:min-h-0">
 <!-- Top bar: what you're editing, history and export. -->
 <div class="flex flex-wrap items-center gap-2 border-b border-border px-3 py-2">
  <details class="saved relative">
   <summary class="inline-flex min-h-9 cursor-pointer list-none items-center gap-2 rounded-md px-2 font-medium hover:bg-accent" title="Open another saved edit">{editTitle}<span class="text-xs font-normal text-muted-foreground">{savedEdits.length?`· ${savedEdits.length} saved ▾`:''}</span></summary>
   <div class="absolute left-0 top-full z-30 mt-1 w-72 rounded-lg border border-border bg-popover p-2 shadow-lg">
    {#if indexError}<p role="alert" class="p-2 text-destructive">{indexError}</p><Button size="sm" variant="ghost" onclick={refreshIndex}>Try again</Button>
    {:else if savedEdits.length}<ul class="max-h-72 overflow-y-auto">{#each savedEdits as edit}<li><a class="block rounded-md px-2 py-2 hover:bg-accent" data-sveltekit-reload href={`/?view=edit&revision=${encodeURIComponent(edit.revision)}`}>{new Date(edit.created_at).toLocaleString([], {dateStyle:'medium',timeStyle:'short'})}{edit.revision===revision?' · open now':''}<span class="block text-xs text-muted-foreground">{fmt(edit.duration)} long</span></a></li>{/each}</ul>
    {:else}<p class="p-2 text-muted-foreground">No saved edits yet. Press Ctrl/⌘ S or Export → Save edit to keep this one.</p>{/if}
   </div>
  </details>
  <span class="text-xs text-muted-foreground" role="status">{opening?'Opening your edit…':busy?'Working…':dirty?'Unsaved changes':revision?'All changes saved':''}</span>
  <div class="ml-auto flex items-center gap-1">
   <Button variant="ghost" size="sm" disabled={busy||advancedDirty||(!dirty&&!!revision)} title="Save edit (Ctrl/⌘ S)" onclick={save}>{!dirty&&revision?'Saved':'Save'}</Button>
   <Button variant="ghost" size="icon" aria-label="Undo" title="Undo (Ctrl/⌘ Z)" disabled={!undoStack.length} onclick={()=>travel()}><Undo2 class="size-4"/></Button>
   <Button variant="ghost" size="icon" aria-label="Redo" title="Redo (Shift Ctrl/⌘ Z)" disabled={!redoStack.length} onclick={()=>travel(true)}><Redo2 class="size-4"/></Button>
   {#if runtime && !runtime.ready}<span class="hidden rounded-md border border-amber-500/50 px-2 py-1 text-xs text-amber-700 dark:text-amber-300 sm:inline" title={runtime.message}>Exporter needs setup</span>{/if}
   <Button size="sm" onclick={()=>exportOpen=true}><Download class="size-4"/>Export</Button>
  </div>
 </div>
 {#if notice&&!exportOpen&&!error}<div role="status" class="flex items-start gap-3 border-b border-border bg-muted/40 px-4 py-2 text-xs"><p class="flex-1">{notice}</p><button class="text-muted-foreground underline" onclick={()=>notice=''}>Dismiss</button></div>{/if}
 {#if error}<div role="alert" class="flex items-start gap-3 border-b border-destructive/40 bg-destructive/10 px-4 py-2 text-destructive"><p class="flex-1">{error}</p><button class="underline" onclick={()=>error=''}>Dismiss</button></div>{/if}

 <div class="workspace min-h-0 flex-1">
  <!-- Left: media you can add. Drag onto the timeline or press +. -->
  <aside class="panel left flex flex-col {mobileTab==='media'?'':'max-lg:hidden'}" aria-label="Media">
   <div role="tablist" aria-label="Media type" class="flex border-b border-border">
    {#each [{id:'media',label:'Media',icon:ImageIcon},{id:'audio',label:'Audio',icon:Music},{id:'text',label:'Text',icon:Type}] as tab}<button role="tab" aria-selected={leftTab===tab.id} class="flex min-h-11 flex-1 items-center justify-center gap-1.5 border-b-2 text-xs font-medium {leftTab===tab.id?'border-primary text-foreground':'border-transparent text-muted-foreground hover:text-foreground'}" onclick={()=>leftTab=tab.id}><svelte:component this={tab.icon} class="size-3.5"/>{tab.label}</button>{/each}
   </div>
   <div class="min-h-0 flex-1 space-y-4 overflow-y-auto p-3">
    {#if leftTab==='media'}
     <p class="text-xs text-muted-foreground">Drag onto the Video track, or press + to add at the end.</p>
     {#if !sourcesLoaded}<p class="text-muted-foreground">Loading media…</p>{:else if !visualSources.length}<p class="rounded-md border border-dashed border-border p-4 text-center text-muted-foreground">No images or clips yet. Render a scene in Story, or generate a clip below.</p>{/if}
     <div class="grid grid-cols-2 gap-2" role="list" aria-label="Images and clips">
      {#each visualSources as asset (asset.id)}
       <div role="listitem" aria-label={asset.name} class="group relative overflow-hidden rounded-md border border-border bg-muted/40" draggable="true" ondragstart={e=>dragAsset(e,asset)} title={asset.name}>
        {#if asset.media_kind==='image'}<img src={mediaUrl(asset.url)??''} alt="" class="aspect-video w-full object-cover" draggable="false"/>{:else}<video src={mediaUrl(asset.url)??''} class="aspect-video w-full object-cover" preload="metadata" muted playsinline><track kind="captions"/></video>{/if}
        <p class="truncate px-2 py-1 text-xs">{asset.name}</p>
        <button disabled={busy} class="absolute right-1 top-1 inline-flex size-7 disabled:opacity-40 items-center justify-center rounded-md bg-background/90 shadow opacity-90 hover:opacity-100" aria-label={`Add ${asset.name} to the end of the video`} onclick={async()=>{await add('clips',asset.id);fitLength()}}><Plus class="size-4"/></button>
       </div>
      {/each}
     </div>
     <ReferenceGeneration onready={async id=>{assets=await get("/compositions/sources");await add("clips","output-"+id);fitLength()}}/>
    {:else if leftTab==='audio'}
     <label class="flex min-h-11 cursor-pointer items-center justify-center gap-2 rounded-md border border-dashed border-border px-3 hover:bg-accent"><Upload class="size-4"/>Upload music or sound<input class="sr-only" type="file" accept="audio/*" onchange={uploadMusic}/></label>
     <p class="text-xs text-muted-foreground">Use one soundtrack across the whole video. It starts quiet under dialogue; change the volume in Properties.</p>
     {#if !audioSources.length}<p class="text-muted-foreground">No audio yet.</p>{/if}
     <ul class="space-y-1">{#each audioSources as asset (asset.id)}<li class="flex items-center gap-2 rounded-md border border-border px-2 py-1.5" draggable="true" ondragstart={e=>dragAsset(e,{...asset,media_kind:asset.media_kind==='video'?'audio':asset.media_kind})}><Music class="size-4 shrink-0 text-muted-foreground"/><span class="min-w-0 flex-1 truncate">{asset.name}{#if asset.media_kind==='video'}<span class="text-xs text-muted-foreground"> · sound from video</span>{/if}</span><button disabled={busy} class="inline-flex size-7 items-center justify-center rounded-md hover:bg-accent disabled:opacity-40" aria-label={`Add ${asset.name} as audio`} onclick={async()=>{await add('audio',asset.id,0);fitLength()}}><Plus class="size-4"/></button></li>{/each}</ul>
    {:else}
     <div class="space-y-2 rounded-md border border-border p-3">
      <p class="font-medium">Captions</p>
      <Button class="w-full" disabled={busy||!draft.clips.length} onclick={captionsFromScript}><Type class="size-4"/>Captions from script</Button>
      <p class="text-xs text-muted-foreground">Free. Uses what each scene's script says and when each shot plays. For exact word timing, use “Transcribe audio” below (uses credits).</p>
      {#if scriptNote}<p role="status" class="text-xs">{scriptNote}</p>{/if}
      <Button class="w-full" variant="outline" onclick={addCaptionAt}><Plus class="size-4"/>Add caption at playhead</Button>
     </div>
     <label class="grid gap-1 text-xs font-medium">Caption size<select class="field" bind:value={draft.caption_style} onchange={invalidate}><option value="default">Standard</option><option value="compact">Small</option><option value="large">Large</option></select></label>
     {#if draft.captions.length}<ul class="space-y-1">{#each draft.captions as cue (cue.id)}<li><button class="w-full rounded-md border px-2 py-1.5 text-left {activeClip===cue.id?'border-primary bg-accent':'border-border hover:bg-accent'}" onclick={()=>{select(cue.id,'captions');playhead=cue.start}}><span class="block text-xs tabular-nums text-muted-foreground">{fmt(cue.start)} – {fmt(cue.end)}</span><span class="line-clamp-2">{cue.text||'Empty caption'}</span></button></li>{/each}</ul>{/if}
     <FinishingPanel {draft} {assets} disabled={busy} onbusy={value=>busy=value} onchange={next=>{draft=next;invalidate()}} onexport={async()=>{exportOpen=true;const saved=await save();if(saved)await run('render')}}/>
    {/if}
   </div>
  </aside>

  <!-- Center: preview and transport. -->
  <section class="preview flex min-h-0 flex-col bg-muted/20" aria-label="Preview">
   <div class="min-h-[220px] flex-1 p-3 max-lg:h-[42vh] max-lg:flex-none"><CompositionPreview {draft} {assets} bind:time={playhead} bind:playing controls={false} fill/></div>
   <div class="flex items-center justify-center gap-2 border-t border-border px-3 py-1.5">
    <Button variant="ghost" size="icon" aria-label="Go to start" title="Go to start (Home)" onclick={()=>{playhead=0}}><SkipBack class="size-4"/></Button>
    <Button size="icon" aria-label={playing?'Pause':'Play'} title="Play / pause (Space)" onclick={()=>playing=!playing}>{#if playing}<Pause class="size-4"/>{:else}<Play class="size-4"/>{/if}</Button>
    <span class="min-w-32 text-center text-xs tabular-nums text-muted-foreground"><span class="text-foreground">{fmt(playhead)}</span> / {fmt(draft.duration)}</span>
   </div>
  </section>

  <!-- Right: properties of the selected item, or the assistant. -->
  <aside class="panel right flex flex-col {mobileTab==='media'?'max-lg:hidden':''}" aria-label="Properties and assistant">
   <div role="tablist" aria-label="Side panel" class="flex border-b border-border">
    {#each [{id:'inspect',label:'Properties',icon:SlidersHorizontal},{id:'assistant',label:'Assistant',icon:Bot}] as tab}<button role="tab" aria-selected={rightTab===tab.id} class="flex min-h-11 flex-1 items-center justify-center gap-1.5 border-b-2 text-xs font-medium {rightTab===tab.id?'border-primary text-foreground':'border-transparent text-muted-foreground hover:text-foreground'}" onclick={()=>{rightTab=tab.id;mobileTab=tab.id}}><svelte:component this={tab.icon} class="size-3.5"/>{tab.label}</button>{/each}
   </div>
   {#if rightTab==='assistant'}
    <div class="min-h-0 flex-1 overflow-hidden max-lg:h-[70vh]"><ChatPanel embedded editing={{draft,selected:activeClip,apply:next=>{draft=next;invalidate()}}}/></div>
   {:else}
   <div class="min-h-0 flex-1 space-y-4 overflow-y-auto p-3">
    {#if selTrack==='clips'&&selIndex>=0}
     <div><p class="text-xs text-muted-foreground">Video clip</p><h3 class="truncate font-medium" title={assetName(draft.clips[selIndex].asset_id)}>{assetName(draft.clips[selIndex].asset_id)}</h3></div>
     <div class="grid grid-cols-3 gap-2">
      <label class="lbl">Starts<input class="field" type="number" min="0" step="0.1" bind:value={draft.clips[selIndex].start} oninput={()=>{fitLength();invalidate()}}/></label>
      <label class="lbl">Length<input class="field" type="number" min="0.1" step="0.1" bind:value={draft.clips[selIndex].duration} oninput={()=>{fitLength();invalidate()}}/></label>
      <label class="lbl">Skip<input class="field" type="number" min="0" step="0.1" title="Seconds skipped at the start of the source" bind:value={draft.clips[selIndex].trim} oninput={invalidate}/></label>
     </div>
     <div class="flex flex-wrap gap-1"><Button size="sm" variant="outline" onclick={splitSelected}><Scissors class="size-3.5"/>Split</Button><Button size="sm" variant="outline" onclick={duplicateSelected}><Copy class="size-3.5"/>Duplicate</Button><Button size="sm" variant="outline" disabled={selIndex===0} onclick={()=>move(selIndex,-1)}>Layer down</Button><Button size="sm" variant="outline" disabled={selIndex===draft.clips.length-1} onclick={()=>move(selIndex,1)}>Layer up</Button><Button size="sm" variant="ghost" class="text-destructive" onclick={deleteSelected}><Trash2 class="size-3.5"/>Delete</Button></div>
     <fieldset class="space-y-2"><legend class="text-xs font-medium">Framing</legend>
      <label class="lbl">Fit<select class="field" value={draft.clips[selIndex].adjustments?.fit??'contain'} onchange={e=>adjust(selIndex,'fit',e.currentTarget.value)}><option value="contain">Show whole image</option><option value="cover">Fill frame (crop)</option></select></label>
      {#each [{key:'scale',label:'Zoom',min:1,max:4,step:.05},{key:'x',label:'Left ↔ right',min:0,max:100,step:1},{key:'y',label:'Up ↕ down',min:0,max:100,step:1}] as o}<label class="lbl">{o.label}<input type="range" min={o.min} max={o.max} step={o.step} value={draft.clips[selIndex].adjustments?.[o.key]??(defaults() as Record<string,any>)[o.key]} oninput={e=>adjust(selIndex,o.key,e.currentTarget.value)}/></label>{/each}
     </fieldset>
     <fieldset class="space-y-2"><legend class="text-xs font-medium">Color</legend>
      {#each [{key:'brightness',label:'Brightness',min:0,max:2,step:.05},{key:'contrast',label:'Contrast',min:0,max:2,step:.05},{key:'saturation',label:'Saturation',min:0,max:2,step:.05},{key:'opacity',label:'Opacity',min:0,max:1,step:.05}] as o}<label class="lbl">{o.label}<input type="range" min={o.min} max={o.max} step={o.step} value={draft.clips[selIndex].adjustments?.[o.key]??(defaults() as Record<string,any>)[o.key]} oninput={e=>adjust(selIndex,o.key,e.currentTarget.value)}/></label>{/each}
      <div class="flex flex-wrap items-center gap-3"><label class="flex items-center gap-2 text-xs"><input type="checkbox" bind:checked={draft.clips[selIndex].muted} onchange={invalidate}/>Mute clip sound</label><Button size="sm" variant="ghost" onclick={()=>{draft.clips[selIndex].adjustments=defaults();invalidate()}}>Reset look</Button></div>
     </fieldset>
     {#if draft.clips[selIndex].asset_id.startsWith('output-')}<div class="space-y-2 border-t border-border pt-3"><p class="text-xs font-medium">Captions from this scene</p><div class="flex flex-wrap gap-2"><Button size="sm" variant="outline" onclick={()=>importCaptions(draft.clips[selIndex])}>Reuse its captions</Button>{#if draft.caption_bindings?.some((b:any)=>b.clip_id===draft.clips[selIndex].id)}<Button size="sm" variant="ghost" onclick={()=>removeLinkedCaptions(draft.clips[selIndex].id)}>Remove linked captions</Button>{/if}</div></div>{/if}
     <p class="text-xs text-muted-foreground">Changes only affect this edit; your Library media stays the same.</p>
    {:else if selTrack==='audio'&&selIndex>=0}
     <div><p class="text-xs text-muted-foreground">Audio</p><h3 class="truncate font-medium">{assetName(draft.audio[selIndex].asset_id)}</h3></div>
     <label class="lbl">Volume · {Math.round((draft.audio[selIndex].volume??1)*100)}%<input type="range" min="0" max="100" step="1" value={(draft.audio[selIndex].volume??1)*100} oninput={e=>{draft.audio[selIndex].volume=Number(e.currentTarget.value)/100;invalidate()}}/></label>
     <Button size="sm" variant="outline" onclick={()=>{draft.audio[selIndex].volume=0.15;invalidate()}}>Quiet background level (15%)</Button>
     <div class="grid grid-cols-2 gap-2"><label class="lbl">Fade in (s)<input class="field" type="number" min="0" step="0.1" bind:value={draft.audio[selIndex].fade_in} oninput={invalidate}/></label><label class="lbl">Fade out (s)<input class="field" type="number" min="0" step="0.1" bind:value={draft.audio[selIndex].fade_out} oninput={invalidate}/></label></div>
     <div class="grid grid-cols-3 gap-2"><label class="lbl">Starts<input class="field" type="number" min="0" step="0.1" bind:value={draft.audio[selIndex].start} oninput={()=>{fitLength();invalidate()}}/></label><label class="lbl">Length<input class="field" type="number" min="0.1" step="0.1" bind:value={draft.audio[selIndex].duration} oninput={()=>{fitLength();invalidate()}}/></label><label class="lbl">Skip<input class="field" type="number" min="0" step="0.1" bind:value={draft.audio[selIndex].trim} oninput={invalidate}/></label></div>
     <div class="flex gap-1"><Button size="sm" variant="outline" onclick={splitSelected}><Scissors class="size-3.5"/>Split</Button><Button size="sm" variant="ghost" class="text-destructive" onclick={deleteSelected}><Trash2 class="size-3.5"/>Delete</Button></div>
    {:else if selTrack==='captions'&&selIndex>=0}
     {@const linked=!!draft.captions[selIndex].source_clip_id}
     <div><p class="text-xs text-muted-foreground">Caption</p>{#if linked}<p class="text-xs text-muted-foreground">Linked to a scene's approved captions. Edit the wording at its source.</p>{/if}</div>
     <label class="lbl">Text<textarea class="field" rows="3" readonly={linked} bind:value={draft.captions[selIndex].text} oninput={invalidate}></textarea></label>
     <div class="grid grid-cols-2 gap-2"><label class="lbl">Starts (s)<input class="field" type="number" min="0" step="0.05" readonly={linked} bind:value={draft.captions[selIndex].start} oninput={()=>{fitLength();invalidate()}}/></label><label class="lbl">Ends (s)<input class="field" type="number" min="0.05" step="0.05" readonly={linked} bind:value={draft.captions[selIndex].end} oninput={()=>{fitLength();invalidate()}}/></label></div>
     <Button size="sm" variant="ghost" class="text-destructive" disabled={linked} onclick={deleteSelected}><Trash2 class="size-3.5"/>Delete caption</Button>
    {:else}
     <div><p class="text-xs text-muted-foreground">Nothing selected</p><h3 class="font-medium">Video settings</h3></div>
     <label class="lbl">Format<select class="field" value={`${draft.width}x${draft.height}`} onchange={e=>dimensions(e.currentTarget.value)}><option value="1080x1920">Vertical 9:16 · 1080 × 1920</option><option value="1920x1080">Landscape 16:9 · 1920 × 1080</option><option value="1080x1080">Square 1:1 · 1080 × 1080</option><option value="640x360">Small landscape · 640 × 360</option>{#if !['640x360','1920x1080','1080x1920','1080x1080'].includes(`${draft.width}x${draft.height}`)}<option value={`${draft.width}x${draft.height}`}>Custom · {draft.width} × {draft.height}</option>{/if}</select></label>
     <div class="grid grid-cols-2 gap-2"><label class="lbl">Frame rate<select class="field" bind:value={draft.fps} onchange={invalidate}><option value={24}>24 fps</option><option value={30}>30 fps</option><option value={60}>60 fps</option></select></label><label class="lbl">Length (s)<input class="field" type="number" min="0.1" max="600" step="0.1" bind:value={draft.duration} oninput={invalidate}/></label></div>
     <Button size="sm" variant="outline" onclick={()=>{fitLength();invalidate()}}>Fit length to content</Button>
     <div class="space-y-2 rounded-md bg-muted/40 p-3 text-xs text-muted-foreground"><p class="font-medium text-foreground">How to edit</p><ul class="list-disc space-y-1 pl-4"><li>Drag media from the left onto the timeline.</li><li>Click a clip to change it here; drag it to move, drag its edges to trim.</li><li>Move the red playhead, then <kbd>S</kbd> to split.</li><li><kbd>Space</kbd> play · <kbd>Del</kbd> delete · <kbd>Ctrl/⌘ Z</kbd> undo</li><li>Ask the Assistant: “shorten the opening” or “add quiet music”.</li></ul></div>
     <details class="text-xs" ontoggle={e=>{if(e.currentTarget.open&&!advancedDirty)advanced=JSON.stringify(draft,null,2)}}><summary class="cursor-pointer py-2 text-muted-foreground">Advanced: edit data</summary><textarea class="field json" rows="10" bind:value={advanced} oninput={()=>{advancedDirty=true;invalidate()}} spellcheck="false" aria-label="Composition JSON"></textarea><Button size="sm" variant="outline" disabled={!advancedDirty} onclick={applyAdvanced}>Apply</Button></details>
    {/if}
   </div>
   {/if}
  </aside>
 </div>

 <!-- Mobile: choose which side panel shows under the preview. -->
 <nav class="mnav flex border-t border-border lg:hidden" aria-label="Editor panels">{#each [{id:'media',label:'Media'},{id:'inspect',label:'Properties'},{id:'assistant',label:'Assistant'}] as tab}<button class="min-h-11 flex-1 text-xs {mobileTab===tab.id?'bg-accent font-semibold':''}" aria-pressed={mobileTab===tab.id} onclick={()=>{mobileTab=tab.id;if(tab.id!=='media')rightTab=tab.id}}>{tab.label}</button>{/each}</nav>

 <!-- Bottom: timeline toolbar and tracks. -->
 <div class="timeline flex flex-col border-t border-border bg-card">
  <div class="flex flex-wrap items-center gap-1 border-b border-border px-2 py-1">
   <Button variant="ghost" size="sm" title="Split at playhead (S)" disabled={selTrack!=='clips'&&selTrack!=='audio'} onclick={splitSelected}><Scissors class="size-4"/>Split</Button>
   <Button variant="ghost" size="sm" title="Duplicate" disabled={selTrack!=='clips'} onclick={duplicateSelected}><Copy class="size-4"/>Duplicate</Button>
   <Button variant="ghost" size="sm" title="Delete (Del)" disabled={!selTrack} onclick={deleteSelected}><Trash2 class="size-4"/>Delete</Button>
   <span class="mx-2 hidden text-xs text-muted-foreground sm:inline">{selTrack?'Drag to move · drag edges to trim':'Click a clip to select it'}</span>
   <div class="ml-auto flex items-center gap-1"><Button variant="ghost" size="icon" aria-label="Zoom out timeline" onclick={()=>zoom=Math.max(10,zoom/1.5)}><ZoomOut class="size-4"/></Button><input type="range" min="10" max="200" step="1" bind:value={zoom} aria-label="Timeline zoom" class="w-24"/><Button variant="ghost" size="icon" aria-label="Zoom in timeline" onclick={()=>zoom=Math.min(200,zoom*1.5)}><ZoomIn class="size-4"/></Button></div>
  </div>
  <div class="h-[200px]"><NleTimeline {draft} {assets} selected={activeClip} bind:time={playhead} bind:zoom onselect={select} onmove={onTimelineMove} ontrim={onTimelineTrim} ondrop={onTimelineDrop}/></div>
 </div>
</div>

<!-- Export: save, check and render with the local HyperFrames renderer. -->
{#if exportOpen}
 <div class="fixed inset-0 z-40 bg-black/40" aria-hidden="true" onclick={()=>exportOpen=false}></div>
 <div class="fixed inset-y-0 right-0 z-50 flex w-full max-w-md flex-col border-l border-border bg-background shadow-xl" role="dialog" aria-modal="true" aria-labelledby="export-title">
  <div class="flex items-center justify-between border-b border-border px-4 py-3"><h2 id="export-title" class="text-base font-semibold">Export video</h2><Button variant="ghost" size="sm" onclick={()=>exportOpen=false}>Close</Button></div>
  <div class="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
   <p class="text-muted-foreground">{draft.width} × {draft.height} · {draft.fps} fps · {fmt(draft.duration)} · {draft.clips.length} clip{draft.clips.length===1?'':'s'}, {draft.audio.length} audio, {draft.captions.length} caption{draft.captions.length===1?'':'s'}</p>
   <ol class="space-y-3">
    <li class="space-y-1"><p class="font-medium">1. Save this edit</p><Button variant={revision&&!dirty?'outline':'default'} disabled={busy||advancedDirty} onclick={save}>{revision&&!dirty?'Saved ✓':'Save edit'}</Button></li>
    <li class="space-y-1"><p class="font-medium">2. Make the video file</p><Button disabled={busy||!draft.clips.length||!runtime?.ready} onclick={async()=>{const saved=dirty||!revision?await save():revision;if(saved)await run('render')}}><Download class="size-4"/>{busy?'Rendering…':'Render video'}</Button><p class="text-xs text-muted-foreground">Free. Rendering runs on this server and adds the finished video to Videos → Edited videos and your Library.</p></li>
   </ol>
   <details class="text-xs"><summary class="cursor-pointer text-muted-foreground">Check before rendering (optional)</summary><div class="mt-2 flex flex-wrap gap-2">{#each [{id:'snapshot',name:'Preview frames'},{id:'check',name:'Test playback'},{id:'lint',name:'Check structure'}] as op}<Button size="sm" variant="outline" disabled={busy||!revision||dirty||!runtime?.ready} onclick={()=>run(op.id)}>{op.name}</Button>{/each}</div><p class="mt-1 text-muted-foreground">Save first. These don't change your edit.</p></details>
   <p role="status" class="text-sm">{notice}</p>
   {#if runtime && !runtime.ready}<div class="rounded-md border border-amber-500/50 p-3 text-xs"><p class="font-medium">Exporter needs setup</p><p class="text-muted-foreground">{runtime.message}</p></div>{/if}
   {#if rendered}<div class="space-y-2"><h3 class="font-medium">Your video</h3><video controls src={rendered} class="mx-auto max-h-[55vh] max-w-full rounded-md bg-black"><track kind="captions"/></video><a class="inline-flex items-center gap-2 underline underline-offset-4" href={rendered} download>Download video</a></div>{/if}
   {#if snapshots.length}<div class="space-y-2"><h3 class="font-medium">Preview frames</h3><div class="grid grid-cols-3 gap-2">{#each snapshots as snapshot}<img src={snapshot} alt="Frame from your edit" class="rounded"/>{/each}</div></div>{/if}
   {#if log}<details><summary class="cursor-pointer text-xs text-muted-foreground">Technical details</summary><pre class="mt-2 max-h-60 overflow-auto whitespace-pre-wrap rounded bg-muted p-2 text-xs">{log}</pre></details>{/if}
  </div>
 </div>
{/if}
<style>
 .workspace{display:grid;grid-template-columns:minmax(0,1fr)}
 @media(min-width:1024px){.workspace{grid-template-columns:280px minmax(0,1fr) 320px}}
 .panel{min-height:0;background:var(--card)}
 @media(min-width:1024px){.left{border-right:1px solid var(--border)}.right{border-left:1px solid var(--border)}}
 /* Phones: preview, timeline, panel switcher, then the chosen panel. */
 @media(max-width:1023px){.workspace{display:contents}.preview{order:1}.timeline{order:2}.mnav{order:3}.panel{order:4}}
 .lbl{display:grid;gap:4px;font-size:12px;font-weight:500}
 .field{width:100%;min-width:0;min-height:36px;padding:6px 8px;border:1px solid var(--border);border-radius:6px;background:var(--background);color:var(--foreground);font:inherit;font-variant-numeric:tabular-nums}
 .json{font:12px monospace;margin:8px 0}
 input[type=range]{width:100%;accent-color:var(--primary)}
 kbd{border:1px solid var(--border);border-radius:4px;padding:0 4px;font-size:11px}
 .saved summary::-webkit-details-marker{display:none}
 .saved:not([open])>div{display:none}
</style>
