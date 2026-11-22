<script lang="ts">
  import { parseDescription } from './description.mjs';
  let { description = '', charactersOnly = false }: { description?: string; charactersOnly?: boolean } = $props();
  const fields = $derived(parseDescription(description));
  const settings = $derived(
    [
      { label: 'Visual style', value: fields.style },
      { label: 'Spoken language', value: fields.language },
      { label: 'Dialogue mode', value: fields.mode },
      { label: 'Direction', value: fields.direction }
    ].filter(f => f.value)
  );
</script>
{#snippet characters()}
 {#if fields.characters.length}<section aria-label="Characters used" class="space-y-2"><h3
   class="text-sm font-medium">Characters used</h3>{#each fields.characters as c}<details
   class="border-t border-border"><summary
   class="min-h-11 cursor-pointer py-3 text-sm font-medium">{c.name}</summary><dl
   class="space-y-3 pb-3 text-sm">{#each [
       { label: 'Description', value: c.description },
       { label: 'Appearance', value: c.appearance },
       { label: 'Personality', value: c.personality },
       { label: 'Visual rules', value: c.visual_rules },
       { label: 'Voice rules', value: c.voice_rules },
       { label: 'Sample dialogue', value: c.sample_dialogue }
     ] as field}{#if field.value}<div><dt
   class="mb-1 text-xs text-muted-foreground">{field.label}</dt><dd
   class="whitespace-pre-wrap break-words leading-relaxed">{field.value}</dd></div>{/if}{/each}</dl>{#if c.id}<a
   class="mb-3 inline-flex min-h-11 items-center text-sm underline underline-offset-4"
   href={`/characters/${encodeURIComponent(c.id)}`}>Open {c.name}</a>{/if}</details>{/each}</section>{/if}
{/snippet}
{#if charactersOnly}{@render characters()}{:else}
 <section aria-label="Asset description" class="space-y-3 text-sm">
 {#if fields.summary}<div><h3 class="mb-2 font-medium">Description</h3><p
   class="whitespace-pre-wrap break-words leading-relaxed text-muted-foreground">{fields.summary}</p></div>{/if}
 {#if settings.length||fields.characters.length}<details><summary
   class="min-h-11 cursor-pointer py-3 font-medium">Generation details</summary><div
   class="space-y-4 pb-3"><dl class="space-y-3">{#each settings as field}<div><dt
   class="mb-1 text-xs text-muted-foreground">{field.label}</dt><dd
   class="whitespace-pre-wrap break-words leading-relaxed">{field.value}</dd></div>{/each}</dl>{@render characters()}</div></details>{/if}
 </section>
{/if}
