<script lang="ts">
 import {get,post} from '$lib/api';
 import {Button} from '$lib/components/ui/button';
 import Mic from '@lucide/svelte/icons/mic';
 type Item={asset_id:string;start:number;duration:number;text:string;speaker:string;cached:boolean};
 let {clips,disabled=false,hasVoice=false,playhead=0,onbusy,onvoices}:{clips:any[];disabled?:boolean;hasVoice?:boolean;playhead?:number;onbusy:(busy:boolean)=>void;onvoices:(items:Item[],lowerVideoSound:boolean,replace:boolean)=>void|Promise<void>}=$props();
 let typed=$state('');
 const STORE='videoflow.voiceover.narrator';
 let provider=$state(''),connected=$state<string[]|null>(null),voice=$state(''),voices=$state<{id:string;name:string;description?:string}[]>([]),loadingVoices=$state(false),working=$state(false),lower=$state(true),error=$state(''),note=$state('');
 const sceneClips=$derived(clips.filter(c=>String(c.asset_id).startsWith('output-')));
 const LABEL:Record<string,string>={elevenlabs:'ElevenLabs',fish:'Fish Audio'};
 let saved:{provider?:string;voice?:string}={};try{saved=JSON.parse(localStorage.getItem(STORE)||'null')??{}}catch{/* Defaults are fine. */}
 // Only offer providers that can actually speak (have a saved key).
 get('/settings/providers').then((list:any[])=>{connected=['elevenlabs','fish'].filter(n=>list.some(p=>p.name===n&&p.configured));provider=connected.includes(saved.provider??'')?saved.provider!:connected[0]??'';if(provider===saved.provider&&saved.voice)voice=saved.voice}).catch(()=>{connected=['elevenlabs','fish'];provider=saved.provider??'elevenlabs'});
 let sequence=0;
 async function loadVoices(p:string){if(!p)return;const seq=++sequence;loadingVoices=true;error='';try{const r=await get(`/voice/catalog/${p}/voices?language=en`);if(seq!==sequence)return;voices=r.voices??[];if(voice&&!voices.some(v=>v.id===voice))voices=[{id:voice,name:'Saved voice'},...voices];if(!voice&&voices.length)voice=(voices.find(v=>/narrat/i.test(v.name))??voices[0]).id}catch(e){if(seq===sequence){voices=[];error='Could not list '+LABEL[p]+' voices. '+String(e)}}finally{if(seq===sequence)loadingVoices=false}}
 $effect(()=>{void loadVoices(provider)});
 function remember(){try{localStorage.setItem(STORE,JSON.stringify({provider,voice}))}catch{/* Not critical. */}}
 async function generate(){if(working||disabled)return;if(hasVoice&&!confirm('Replace all voice lines on the Audio track, including typed ones? You can undo.'))return;working=true;onbusy(true);error='';note='';remember();
  try{const r=await post('/compositions/voiceover',{clips,narrator:voice?{provider,voice_id:voice}:{}});const items=(r.items??[]) as Item[];const skipped=(r.skipped??[]) as {reason:string}[];
   if(!items.length){note=skipped[0]?.reason??'No script lines found. Add rendered scene videos from Media first.';return}
   await onvoices(items,lower,true);
   note=`Added ${items.length} voice line${items.length===1?'':'s'}`+(r.generated?` (${r.generated} new`+(r.reused?`, ${r.reused} reused)`:')'):' (all reused, no charge)')+'.'+(r.shifted?` Moved ${r.shifted} later so lines don't overlap.`:'')+(r.overrun>0.5?` The voice runs ${r.overrun.toFixed(1)}s past the video: shorten the script, pick a faster voice, or add a closing shot.`:' Drag them on the Audio track to fine-tune.')+(skipped.length?` Skipped ${skipped.length}: ${skipped[0].reason}`:'');
  }catch(e){error=String(e)}finally{working=false;onbusy(false)}}
 // A line that is not in the script: spoken by the narrator voice at the playhead.
 async function speakTyped(){const text=typed.trim();if(!text||working||disabled||!voice)return;working=true;onbusy(true);error='';note='';remember();
  try{const r=await post('/compositions/voiceover',{clips:[],narrator:{provider,voice_id:voice},lines:[{text,start:Math.max(0,Math.round(playhead*1000)/1000)}]});const items=(r.items??[]) as Item[];if(!items.length){note=(r.skipped??[])[0]?.reason??'Nothing was spoken.';return}
   await onvoices(items,lower,false);typed='';note=`Added “${text.length>40?text.slice(0,40)+'…':text}” at ${playhead.toFixed(1)}s`+(r.generated?'':' (reused, no charge)')+'.';
  }catch(e){error=String(e)}finally{working=false;onbusy(false)}}
</script>
<section aria-label="Voice from script" class="space-y-2 rounded-md border border-border p-3">
 <p class="flex items-center gap-2 font-medium"><Mic class="size-4"/>Voice from script</p>
 <p class="text-xs text-muted-foreground">Speaks each scene's script lines at the same timing as captions. Characters with a saved voice in Voices use it; everyone else uses this narrator voice.</p>
 {#if connected&&!connected.length}<div role="note" class="space-y-2 rounded-md bg-muted/60 p-2 text-xs"><p>Connect ElevenLabs or Fish Audio to make voices. It takes one API key.</p><Button size="sm" variant="outline" href="/settings#providers">Connect a voice provider</Button></div>{:else}
 <div class="grid grid-cols-[auto_1fr] gap-2">
  <select aria-label="Speech provider" class="min-h-9 rounded-md border border-input bg-background px-2 text-sm" bind:value={provider} onchange={()=>{voice='';remember()}} disabled={working}>{#each connected??[] as name}<option value={name}>{LABEL[name]}</option>{/each}</select>
  <select aria-label="Narrator voice" class="min-h-9 min-w-0 rounded-md border border-input bg-background px-2 text-sm" bind:value={voice} onchange={remember} disabled={working||loadingVoices}>{#if loadingVoices}<option value={voice}>Loading voices…</option>{:else if !voices.length}<option value="">No voices available</option>{/if}{#each voices as v (v.id)}<option value={v.id}>{v.name||v.id}</option>{/each}</select>
 </div>
 <label class="flex items-start gap-2 text-xs"><input type="checkbox" class="mt-0.5" bind:checked={lower} disabled={working}/>Lower the videos' own sound so voices don't double up (ambience stays)</label>
 <Button class="w-full" disabled={disabled||working||!sceneClips.length} onclick={generate}><Mic class="size-4"/>{working?'Making voices…':hasVoice?'Redo voices from script':'Add voices from script'}</Button>
 <p class="text-xs text-muted-foreground">{sceneClips.length?'Uses speech credits for new lines; unchanged lines are reused free.':'Add a rendered scene video to the timeline to voice its script.'}</p>
 <form class="flex gap-2" onsubmit={e=>{e.preventDefault();void speakTyped()}}><input aria-label="Line to speak" class="min-h-9 min-w-0 flex-1 rounded-md border border-input bg-background px-2 text-sm" placeholder="Or type any line…" maxlength="500" bind:value={typed} disabled={working}/><Button type="submit" size="sm" variant="outline" disabled={disabled||working||!typed.trim()||!voice}>Say at {playhead.toFixed(1)}s</Button></form>
 {/if}
 {#if note}<p role="status" class="text-xs">{note}</p>{/if}
 {#if error}<p role="alert" class="text-xs text-destructive">{error}</p>{/if}
</section>
