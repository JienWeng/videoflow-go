<script lang="ts">
 import {nodeTools} from './tools.mjs';
 import {Button} from '$lib/components/ui/button';
 let {kind,id,data={}}:{kind:string;id:string;data?:Record<string,unknown>}=$props();
</script>
<nav aria-label="Editing tools" class="grid gap-2 border-b pb-4">
 {#each nodeTools(kind,id,data) as tool(tool.href)}<Button
   variant="outline"
   class="justify-start whitespace-normal text-left"
   href={tool.href}>{tool.label}</Button>{/each}
</nav>
