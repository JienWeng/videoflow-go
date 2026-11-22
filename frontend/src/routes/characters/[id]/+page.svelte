<script lang="ts">
  import {page} from '$app/state';
  import CharacterDetail from '$lib/library/CharacterDetail.svelte';
</script>
{#key page.params.id}<CharacterDetail characterId={page.params.id ?? ''}/>{/key}
