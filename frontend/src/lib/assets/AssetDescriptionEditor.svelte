<script lang="ts">
  import { parseDescription, formatDescription, type AssetDescription as DescriptionFields } from './description.mjs';
  import AssetDescription from './AssetDescription.svelte';
  import { Textarea } from '$lib/components/ui/textarea';
  import { Input } from '$lib/components/ui/input';
  let {
    value = $bindable(),
    disabled = false,
    onchange
  }: { value?: string; disabled?: boolean; onchange?: () => void } = $props();
  let fields: DescriptionFields = $state(parseDescription(''));
  let seen: string | undefined;
  $effect(() => {
    if (value !== seen) {
      seen = value;
      fields = parseDescription(value);
    }
  });
  function update(key: 'summary' | 'style' | 'language' | 'mode' | 'direction', text: string) {
    fields[key] = text;
    const formatted = formatDescription(fields);
    seen = formatted;
    value = formatted;
    onchange?.();
  }
</script>
<div class="space-y-3">
 <label class="block space-y-1 text-sm">Asset description<Textarea
   rows={4}
   {disabled}
   value={fields.summary}
   oninput={e=>update('summary',e.currentTarget.value)}/></label>
 <details><summary class="min-h-11 cursor-pointer py-3 text-sm font-medium">Generation settings</summary><div class="space-y-3 pb-3">
 <label class="block space-y-1 text-sm">Visual style<Textarea
   rows={2}
   {disabled}
   value={fields.style}
   oninput={e=>update('style',e.currentTarget.value)}/></label>
 <div class="grid gap-3 sm:grid-cols-2"><label class="block space-y-1 text-sm">Spoken language<Input
   {disabled}
   value={fields.language}
   oninput={e=>update('language',e.currentTarget.value)}/></label><label class="block space-y-1 text-sm">Dialogue mode<Input
   {disabled}
   value={fields.mode}
   oninput={e=>update('mode',e.currentTarget.value)}/></label></div>
 <label class="block space-y-1 text-sm">Direction<Textarea
   rows={2}
   {disabled}
   value={fields.direction}
   oninput={e=>update('direction',e.currentTarget.value)}/></label>
 </div></details>
 <AssetDescription description={value} charactersOnly/>
</div>
