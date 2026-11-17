<script lang="ts">
 type Preset={id:string;name:string;description?:string;style_prompt?:string;fingerprint?:string};
 let {presets,selected='',disabled=false,onselect}:{presets:Preset[];selected?:string;disabled?:boolean;onselect?:(preset:Preset)=>void}=$props();
</script>
<fieldset disabled={disabled} class="min-w-0 space-y-2" aria-label="Visual style choices"><legend class="mb-2 text-sm font-medium">Visual style</legend><div class="grid min-w-0 grid-cols-2 gap-2">
 {#each presets as preset(preset.id)}<button type="button" aria-pressed={selected===preset.id} class={`min-h-24 min-w-0 rounded-lg border p-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${selected===preset.id?'border-foreground bg-accent':'border-border bg-background hover:bg-accent/50'}`} onclick={()=>onselect?.(preset)}><span class="block break-words text-sm font-medium">{preset.name||'Project style'}</span><span class="mt-1 block text-xs leading-relaxed text-muted-foreground line-clamp-3">{preset.description||preset.style_prompt}</span><span class="mt-2 block text-xs">{selected===preset.id?'Selected':preset.id.startsWith('community:')?'Community preset':preset.id==='project-style'?'Project default':'Choose style'}</span></button>{/each}
 </div></fieldset>
