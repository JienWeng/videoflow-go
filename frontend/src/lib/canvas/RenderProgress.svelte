<script lang="ts">
 import {untrack} from 'svelte';
 import {get,mediaUrl} from '$lib/api';import {subscribeJobs} from '$lib/sse';
 import {Film,Check,TriangleAlert} from '@lucide/svelte';import {Button} from '$lib/components/ui/button';
 let {id,initialStatus='pending'}:{id:string;initialStatus?:string}=$props();
 type Detail={job:{scene_id?:string;request_json?:{audio_mode?:string};status:string;stage?:string;progress?:string;error?:string};outputs:{id:string;video_path?:string;captioned_path?:string}[]};
 let detail=$state<Detail|null>(null),error=$state(''),loading=$state(true);let retry=$state(0);
 const status=$derived(detail?.job.status??initialStatus);
 const active=$derived(!error&&['pending','running','queued','submitted','processing'].includes(status));
 const done=$derived(status==='succeeded');
 const failed=$derived(status==='failed');
 const title=$derived(error?'Render details unavailable':done?'Your video is ready':failed?'Render could not finish':status==='reconciliation_required'?'Render needs attention':status==='pending'||status==='queued'?'Waiting to start':detail?.job.stage==='downloading'?'Saving your video':active?'Creating your video':'Render paused');
 const output=$derived(done?detail?.outputs.find(o=>o.captioned_path||o.video_path):undefined);
 $effect(()=>{
  const jobId=id;const current=initialStatus;retry;
  untrack(()=>{detail=null;error='';loading=true});let live=true,inFlight=false;
  async function refresh(){if(inFlight)return;inFlight=true;try{const response=await get(`/render-jobs/${encodeURIComponent(jobId)}`);if(!response?.job?.status)throw Error('Render details were not returned');if(live){detail=response;error=''}}catch(e){if(live)error=String(e)}finally{inFlight=false;if(live)loading=false}}
  void refresh();const unsubscribe=subscribeJobs(event=>{if(event.job_id===jobId)void refresh()},()=>void refresh());
  return()=>{live=false;unsubscribe()};
 });
</script>
<section aria-label="Render progress" aria-busy={active||loading} class="space-y-4">
 {#if output}
 <!-- svelte-ignore a11y_media_has_caption (Silent generated previews can receive captions in the editor.) -->
 <video class="result aspect-video w-full rounded-lg bg-background object-contain" controls preload="metadata" src={mediaUrl(output.captioned_path||output.video_path)??''} aria-label="Generated video preview"></video>
 {:else}
 <div class="preview relative flex aspect-video items-center justify-center overflow-hidden rounded-lg border border-border bg-background" class:working={active} aria-hidden="true">
  {#if done}<Check class="size-9 text-muted-foreground"/>{:else if failed||error||!active}<TriangleAlert class="size-8 text-muted-foreground"/>{:else}<div class="placeholder-frame flex h-24 w-36 items-center justify-center rounded-md border border-border bg-muted/40"><Film class="size-8 text-muted-foreground"/></div>{/if}
  {#if active}<div class="preview-light absolute inset-0 bg-muted/40"></div>{/if}
 </div>
 {/if}
 <div role="status" aria-live="polite" aria-atomic="true" class="space-y-2">
  <div class="flex items-center gap-2"><span aria-hidden="true" class="size-2 shrink-0 rounded-full bg-muted-foreground" class:live-dot={active}></span><h3 class="font-semibold">{title}</h3></div>
  <p class="text-sm leading-relaxed text-muted-foreground">{error?'The last known status is '+status+'. Loading details does not restart generation.':failed?(detail?.job.error??'Check the existing render before trying again.'):status==='reconciliation_required'?(detail?.job.error??'Check the existing provider request before submitting another render.'):done?(output?'Preview the clip, then continue editing or add speaker audio.':'The render completed. Open renders & outputs to inspect the result.'):active?(detail?.job.progress??(status==='pending'?'Your clip is in the queue. You can keep working while it starts.':'Your clip will appear here when it is ready. You can keep working.')):'Open renders & outputs to inspect this job.'}</p>
 </div>
 {#if active}<div aria-hidden="true" class="space-y-2"><div class="h-2 w-3/4 rounded bg-muted"></div><div class="h-2 w-1/2 rounded bg-muted"></div></div><p class="text-xs text-muted-foreground">Updates automatically · Timing depends on the video provider</p>{/if}
 {#if error}<Button variant="outline" size="sm" onclick={()=>retry++}>Reload render details</Button>{/if}
 {#if detail?.job.request_json?.audio_mode}<p class="text-xs text-muted-foreground">Audio: {detail.job.request_json.audio_mode==='native'?'native dialogue, ambience and effects':detail.job.request_json.audio_mode==='saved'?'silent visuals for saved-voice speech':'silent'}</p>{/if}
 {#if output}<Button variant="outline" size="sm" href={`/editor/${output.id}`}>Open video editor</Button>{#if detail?.job.scene_id}<Button variant="outline" size="sm" href={`/compositions?scene_id=${encodeURIComponent(detail.job.scene_id)}`}>Assemble clips & add music</Button>{/if}{/if}
</section>
<style>
 .preview-light{animation:render-breathe 2.8s ease-in-out infinite;pointer-events:none}
 .live-dot{background:var(--foreground)}
 .result{animation:render-reveal 160ms ease-out both}
 @keyframes render-breathe{0%,100%{opacity:.12}50%{opacity:.7}}
 @keyframes render-reveal{from{opacity:.6}to{opacity:1}}
 @media(prefers-reduced-motion:reduce){.preview-light,.result{animation:none}.preview-light{opacity:.25}}
</style>
