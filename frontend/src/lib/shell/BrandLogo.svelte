<script lang="ts">
  let {compact=false, class:className=''}:{compact?:boolean;class?:string}=$props();
</script>

<!-- Custom VideoFlow lettering: shared angular V/F geometry, rounded flowing terminals. -->
{#if compact}
  <svg aria-hidden="true" focusable="false" viewBox="0 0 34 34" class={className} fill="none" stroke="currentColor" stroke-width="2.8" stroke-linecap="round" stroke-linejoin="round">
    <path d="M3 8 11 26 19 8M22 26V8h9M22 16h7" />
  </svg>
{:else}
  <svg aria-hidden="true" focusable="false" viewBox="0 0 164 32" class={className} fill="none" stroke="currentColor" stroke-width="2.8" stroke-linecap="round" stroke-linejoin="round">
    <path d="M3 6 11 26 19 6M26 14v12M44 5v21M44 20a6.5 6.5 0 1 1 0-1M53 19h12c0-9-13-9-13 0 0 7 8 9 12 5M84 20a6.5 6.5 0 1 1-13 0 6.5 6.5 0 1 1 13 0M94 26V6h13M94 15h10M115 5v18q0 3 3 3M135 20a6.5 6.5 0 1 1-13 0 6.5 6.5 0 1 1 13 0M141 14l4 12 6-11 6 11 4-12" />
    <circle cx="26" cy="7.5" r="1.5" fill="currentColor" stroke="none" />
  </svg>
{/if}
