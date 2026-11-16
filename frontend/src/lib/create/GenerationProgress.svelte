<script lang="ts">
  import {Check,LoaderCircle,LockKeyhole,TriangleAlert,ArrowRight} from '@lucide/svelte';
  import {creationProgress,creationSteps} from './progress.mjs';
  let {status='running',stages=[],error='',onopen,reviewHref}:{status?:string;stages?:string[];error?:string;onopen?:()=>void;reviewHref?:string}=$props();
  const progress=$derived(creationProgress(status,stages));
  const currentStep=$derived(creationSteps.find(step=>step.key===progress.current));
</script>
<section aria-label="Story creation progress" class="py-2">
  <div class="mb-6" role="status" aria-live="polite" aria-atomic="true">
    <p class="mb-2 flex items-center gap-2 text-xs font-medium text-muted-foreground">{#if progress.stopped}<TriangleAlert class="size-4"/>Needs attention{:else if progress.review}<ArrowRight class="size-4"/>Your turn{:else if status==='completed'}<Check class="size-4"/>Ready{:else}<LoaderCircle class="size-4 motion-safe:animate-spin"/>{error?'Reconnecting…':`Step ${creationSteps.findIndex(step=>step.key===progress.current)+1} of ${creationSteps.length}`}{/if}</p>
    <h2 class="text-xl font-semibold tracking-tight sm:text-2xl">{progress.title}</h2>
    <p class="mt-2 max-w-prose text-sm leading-relaxed text-muted-foreground">{progress.review?'Review your images next. Video generation waits for your approval.':progress.stopped?'Completed work is preserved. Review it before starting another attempt.':status==='completed'?'Open your saved work to review the result.':progress.rendering?'Your submitted clips are being processed. You can check their status in Videos.':currentStep?.detail}</p>
    {#if progress.review&&reviewHref}<a href={reviewHref} class="mt-4 inline-flex min-h-11 items-center gap-2 rounded-lg bg-primary px-4 text-sm font-medium text-primary-foreground hover:opacity-90 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring">Review storyboard<ArrowRight class="size-4"/></a>{/if}
  </div>
  <ol aria-label="Creation steps" class="divide-y divide-border border-y border-border">
    {#each creationSteps as step,i}
      {@const done=progress.completed.has(step.key)}
      {@const active=!done&&progress.current===step.key}
      <li aria-current={active?'step':undefined} data-creation-step={step.key} data-state={done?'done':active?progress.stopped?'stopped':progress.review?'review':'active':'pending'} class="flex gap-3 py-4 {active?'text-foreground':'text-muted-foreground'}">
        <span class="mt-0.5 grid size-7 shrink-0 place-items-center rounded-full border text-xs font-medium {active?'border-primary bg-primary text-primary-foreground':done?'border-foreground/30 text-foreground':'border-border'}" aria-hidden="true">{#if done}<Check class="size-3.5"/>{:else if active&&progress.stopped}<TriangleAlert class="size-3.5"/>{:else if active&&!progress.review&&status!=='completed'}<LoaderCircle class="size-3.5 motion-safe:animate-spin"/>{:else}{i+1}{/if}</span>
        <div class="min-w-0 flex-1"><div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-1"><h3 class="text-sm {active||done?'font-medium text-foreground':''}">{step.label}</h3><span class="flex items-center gap-1 text-xs">{#if done}Done{:else if active}{progress.stopped?'Needs attention':progress.review?'Ready for review':status==='completed'?'Ready':'In progress'}{:else if step.key==='review'&&progress.rendering}Review not recorded{:else if step.key==='render'}<LockKeyhole class="size-3"/>After approval{:else}Upcoming{/if}</span></div>{#if active||step.key==='review'&&!progress.completed.has('review')}<p class="mt-1 text-xs leading-relaxed text-muted-foreground">{step.key==='review'&&progress.rendering?'Approval status is unavailable for this saved run. Review its references in Library.':step.key==='render'&&progress.rendering?'Your submitted clips are processing. Review their results in Videos.':step.detail}</p>{/if}</div>
      </li>
    {/each}
  </ol>
  {#if error}<div role="alert" class="mt-4 rounded-lg border border-destructive p-4 text-sm"><p class="font-medium">{progress.stopped?'Creation paused':'Connection interrupted'}</p><p class="mt-1 break-words text-muted-foreground">{error}</p>{#if progress.stopped&&onopen}<button type="button" class="mt-3 min-h-11 underline underline-offset-4 focus-visible:outline-2 focus-visible:outline-ring" onclick={onopen}>Open existing scenes</button>{/if}</div>{/if}
  {#if !progress.stopped&&status!=='completed'&&!progress.review}<p class="mt-4 text-xs leading-relaxed text-muted-foreground">You can close this window while work continues. Check Activity for the saved operation.</p>{/if}
</section>
