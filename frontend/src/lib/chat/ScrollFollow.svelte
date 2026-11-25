<script lang="ts">
  import { tick } from 'svelte';
  import { useMessageScroller } from '$lib/components/ui/message-scroller';

  /** Each new value of `key` (a sent message, a new reply, another chat) brings
   * the conversation down to its latest message. Must sit inside the Provider. */
  let { key }: { key: string } = $props();
  const { scrollToEnd } = useMessageScroller();
  $effect(() => {
    void key;
    tick().then(() => requestAnimationFrame(() => scrollToEnd({ behavior: 'smooth' })));
  });
</script>
