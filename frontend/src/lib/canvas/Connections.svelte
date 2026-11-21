<script lang="ts">
 import {Button} from '$lib/components/ui/button';
 import {connectionRoute,editableConnection,relationshipLabels} from './connections.mjs';
 import {KIND_LABELS} from './transform';
 import type {Node,Edge} from '@xyflow/svelte';
 let {node,nodes,edges,busy,onconnect,ondetach,onselect,onclose}:{node:Node;nodes:Node[];edges:Edge[];busy:boolean;onconnect:(a:string,b:string)=>void;ondetach:(edge:Edge)=>void;onselect:(node:Node)=>void;onclose:()=>void}=$props();
 let target=$state('');
 $effect(()=>{node.id;target='';});
 const links=$derived(edges.filter(e=>e.source===node.id||e.target===node.id));
 const candidates=$derived(nodes.filter(n=>connectionRoute(node,n)&&!links.some(e=>(e.source===n.id||e.target===n.id)&&editableConnection(e,node,n))));
</script>
<section aria-label="Item connections" class="w-80 max-w-[calc(100vw-2rem)] rounded-xl border border-border bg-card p-3 shadow-lg">
 <div class="flex items-center justify-between gap-2"><h3 class="truncate text-sm font-semibold">Connections · {node.data.label}</h3><Button variant="ghost" size="sm" onclick={onclose}>Close</Button></div>
 <p class="mb-3 text-xs text-muted-foreground">Saved links used by the story and assistant. Generated relationships are kept automatically.</p>
 <ul class="max-h-56 space-y-2 overflow-y-auto" aria-label="Connected items">
 {#each links as edge (edge.id)}{@const other=nodes.find(n=>n.id===(edge.source===node.id?edge.target:edge.source))}{#if other}
 <li class="flex items-center gap-2 border-b border-border pb-2"><button class="min-w-0 flex-1 rounded text-left focus-visible:outline-2 focus-visible:outline-ring" onclick={()=>onselect(other)}><span class="block text-xs text-muted-foreground">{relationshipLabels[String(edge.data?.rel)]??edge.data?.rel}</span><span class="block truncate text-sm underline underline-offset-4">{other.data.label}</span></button>{#if editableConnection(edge,node,other)}<Button variant="ghost" size="sm" disabled={busy} onclick={()=>ondetach(edge)} aria-label={`Disconnect ${other.data.label}`}>Disconnect</Button>{:else}<span class="text-xs text-muted-foreground">Automatic</span>{/if}</li>
 {/if}{:else}<li class="py-2 text-sm text-muted-foreground">No connections yet.</li>{/each}
 </ul>
 {#if candidates.length}<label for="connection-target" class="mt-3 block text-xs font-medium">Connect an existing item</label><select id="connection-target" class="mt-1 min-h-11 w-full rounded-md border border-border bg-background px-2 text-sm" bind:value={target} disabled={busy}><option value="">Choose an item…</option>{#each candidates as candidate}<option value={candidate.id}>{KIND_LABELS[String(candidate.data.kind)]} · {candidate.data.label}</option>{/each}</select><Button class="mt-2 w-full" disabled={busy||!candidates.some(n=>n.id===target)} onclick={()=>onconnect(node.id,target)}>{busy?'Saving…':'Connect'}</Button>{:else}<p class="mt-3 text-xs text-muted-foreground">{['scene','character','shot','asset'].includes(String(node.data.kind))?'Add compatible items to connect cast, scene props or locations, and shot assets.':'Open this item’s editor to manage its content.'}</p>{/if}
</section>
