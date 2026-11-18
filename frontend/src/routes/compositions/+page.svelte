<script lang="ts">
 // The editor lives in the Studio. Old links (/compositions?scene_id=…) keep working.
 import { goto } from '$app/navigation';
 import { page } from '$app/state';
 import { onMount } from 'svelte';
 onMount(() => { const q = new URLSearchParams(page.url.search); q.set('view', 'edit'); void goto('/?' + q.toString(), { replaceState: true }); });
</script>
<p class="p-6 text-sm text-muted-foreground">Opening the editor…</p>
