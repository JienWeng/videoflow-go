<script lang="ts">
  import symbol from '../../../static/brand/videoflow-symbol.svg?raw';
  let {class:className='size-10'}:{class?:string}=$props();
  const mark=symbol.replace('fill="#F5EFE7"','fill="currentColor"');
</script>

<span aria-hidden="true" class={`inline-block shrink-0 ${className}`}>{@html mark}</span>
<style>span{color:#19201F}:global(.dark) span{color:#F5EFE7}span :global(svg){display:block;width:100%;height:100%}</style>
