<script lang="ts">
 import { BookOpen, Clapperboard, Hand, Layers, PenTool, Sparkles, Camera, Drama, Palette, Check } from '@lucide/svelte';
 type Preset={id:string;name:string;description?:string;style_prompt?:string;fingerprint?:string};
 let {presets,selected='',disabled=false,onselect}:{presets:Preset[];selected?:string;disabled?:boolean;onselect?:(preset:Preset)=>void}=$props();
 // A quick visual hint of each built-in look; community and project styles get a neutral swatch.
 const LOOKS:Record<string,{bg:string;icon:typeof Palette}>={
  '2d-picture-book':{bg:'linear-gradient(135deg,#fde68a,#fca5a5 55%,#93c5fd)',icon:BookOpen},
  'cinematic':{bg:'linear-gradient(135deg,#0f172a,#1e3a8a 50%,#f59e0b)',icon:Clapperboard},
  'clay-stop-motion':{bg:'linear-gradient(135deg,#fdba74,#d97706 50%,#84cc16)',icon:Hand},
  'layered-paper-cut':{bg:'repeating-linear-gradient(160deg,#fef3c7 0 14px,#fcd34d 14px 26px,#f97316 26px 36px)',icon:Layers},
  'graphic-novel':{bg:'linear-gradient(135deg,#111827 0 45%,#ef4444 45% 60%,#facc15 60%)',icon:PenTool},
  'soft-3d-animation':{bg:'radial-gradient(circle at 30% 30%,#fbcfe8,#a5b4fc 55%,#67e8f9)',icon:Sparkles},
  'live-action-documentary':{bg:'linear-gradient(135deg,#365314,#a3a3a3 55%,#e7e5e4)',icon:Camera},
  'live-action-drama':{bg:'linear-gradient(135deg,#1c1917,#7f1d1d 55%,#d6d3d1)',icon:Drama}
 };
 const look=(id:string)=>LOOKS[id]??{bg:'linear-gradient(135deg,#334155,#64748b)',icon:Palette};
</script>
<fieldset
  disabled={disabled}
  class="@container min-w-0 space-y-2"
  aria-label="Visual style choices"><legend class="mb-2 text-sm font-medium">Choose a look</legend><div
    class="grid min-w-0 grid-cols-2 gap-2 @lg:grid-cols-3 @3xl:grid-cols-4">
 {#each presets as preset(preset.id)}{@const l=look(preset.id)}{@const on=selected===preset.id}
  <button
    type="button"
    aria-pressed={on}
    class={`group relative min-w-0 overflow-hidden rounded-lg border text-left transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-60 ${on?'border-primary ring-2 ring-primary/60':'border-border hover:border-foreground/40'}`}
    onclick={()=>onselect?.(preset)}>
   <span class="flex h-16 items-center justify-center" style={`background:${l.bg}`} aria-hidden="true"><l.icon class="size-6 text-white drop-shadow" /></span>
   {#if on}<span
     class="absolute right-2 top-2 inline-flex size-6 items-center justify-center rounded-full bg-primary text-primary-foreground shadow"
     aria-hidden="true"><Check class="size-4" /></span>{/if}
   <span class="block p-2.5">
    <span class="block break-words text-sm font-medium">{preset.name||'Project style'}</span>
    <span class="mt-0.5 block text-xs leading-snug text-muted-foreground line-clamp-2">{preset.description||preset.style_prompt}</span>
    {#if preset.id.startsWith('community:')||preset.id==='project-style'}<span
      class="mt-1 block text-[11px] text-muted-foreground">{preset.id==='project-style'?'Project default':'Community'}</span>{/if}
   </span>
  </button>
 {/each}
 </div></fieldset>
