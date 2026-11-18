<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { tick, untrack } from 'svelte';
  import { Button } from '$lib/components/ui/button';
  import { guide, TOUR_STEPS } from './guide.svelte';

  let index = $state(0);
  let rect = $state<{ top: number; left: number; width: number; height: number } | null>(null);
  let popover = $state<HTMLElement>();
  let token = 0;

  const step = $derived(TOUR_STEPS[index]);
  const last = $derived(index === TOUR_STEPS.length - 1);

  function visible(el: Element | null): el is HTMLElement {
    if (!(el instanceof HTMLElement)) return false;
    const r = el.getBoundingClientRect();
    return r.width > 0 && r.height > 0;
  }

  async function find(target: string): Promise<HTMLElement | null> {
    for (let i = 0; i < 40; i++) {
      const el = [...document.querySelectorAll(`[data-tour="${target}"]`)].find(visible);
      if (el) return el as HTMLElement;
      await new Promise(r => setTimeout(r, 100));
    }
    return null;
  }

  async function show(i: number, direction = 1) {
    const mine = ++token;
    index = i;
    const s = TOUR_STEPS[i];
    if (s.view) {
      const wanted = s.view === 'story' ? '/?view=story' : '/';
      const onView = page.url.pathname === '/' && (s.view === 'story' ? page.url.searchParams.get('view') === 'story' : !page.url.searchParams.get('view'));
      if (!onView) await goto(wanted);
    }
    if (s.openAssistant) window.dispatchEvent(new CustomEvent('videoflow-ask-assistant', { detail: {} }));
    await tick();
    const el = await find(s.target);
    if (mine !== token) return;
    if (!el) { // Skip steps whose element isn't on screen (e.g. collapsed sidebar on a phone).
      const next = i + direction;
      if (next >= 0 && next < TOUR_STEPS.length) return show(next, direction);
      return finish();
    }
    el.scrollIntoView({ block: 'center', behavior: 'smooth' });
    await new Promise(r => setTimeout(r, 250));
    const r = el.getBoundingClientRect();
    rect = { top: r.top - 6, left: r.left - 6, width: r.width + 12, height: r.height + 12 };
    await tick();
    popover?.focus();
  }

  function finish() { token++; rect = null; guide.endTour(); }

  // Only react to the tour starting; show() reads the URL, which must not restart the tour.
  $effect(() => { if (guide.touring) untrack(() => void show(0)); });

  function onKeydown(e: KeyboardEvent) {
    if (!guide.touring) return;
    if (e.key === 'Escape') finish();
    else if (e.key === 'ArrowRight' && !last) void show(index + 1);
    else if (e.key === 'ArrowLeft' && index > 0) void show(index - 1, -1);
  }
  function reposition() { if (guide.touring) void show(index); }

  // Keep the card on screen: below the target when there is room, otherwise above.
  const card = $derived.by(() => {
    if (!rect) return '';
    const w = Math.min(340, window.innerWidth - 32);
    const left = Math.max(16, Math.min(rect.left, window.innerWidth - w - 16));
    const below = rect.top + rect.height + 12;
    const top = below + 220 < window.innerHeight ? below : Math.max(16, rect.top - 232);
    return `top:${top}px;left:${left}px;width:${w}px`;
  });
</script>

<svelte:window onkeydown={onKeydown} onresize={reposition} />

{#if guide.touring && rect}
  <div class="fixed inset-0 z-[90]" aria-hidden="true" onclick={finish}></div>
  <div class="pointer-events-none fixed z-[91] rounded-xl ring-2 ring-primary transition-all duration-200 motion-reduce:transition-none"
    style={`top:${rect.top}px;left:${rect.left}px;width:${rect.width}px;height:${rect.height}px;box-shadow:0 0 0 9999px rgb(0 0 0 / 0.55)`}></div>
  <div bind:this={popover} tabindex="-1" role="dialog" aria-modal="true" aria-labelledby="tour-title" aria-describedby="tour-body"
    class="fixed z-[92] rounded-xl border border-border bg-popover p-4 text-popover-foreground shadow-xl outline-none" style={card}>
    <p class="text-xs text-muted-foreground">Step {index + 1} of {TOUR_STEPS.length}</p>
    <h2 id="tour-title" class="mt-1 font-semibold">{step.title}</h2>
    <p id="tour-body" class="mt-2 text-sm leading-relaxed text-muted-foreground">{step.body}</p>
    <div class="mt-4 flex items-center gap-2">
      <Button variant="ghost" size="sm" onclick={finish}>Skip tour</Button>
      <div class="ml-auto flex gap-2">
        {#if index > 0}<Button variant="outline" size="sm" onclick={() => show(index - 1, -1)}>Back</Button>{/if}
        <Button size="sm" onclick={() => last ? finish() : show(index + 1)}>{last ? 'Done' : 'Next'}</Button>
      </div>
    </div>
  </div>
{/if}
