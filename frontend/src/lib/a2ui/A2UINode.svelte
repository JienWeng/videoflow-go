<script lang="ts">
  // One A2UI component, drawn with the app's own UI components: the basic catalog's
  // Text, Image, Row, Column, List, Card, Button, ChoicePicker and Divider, and the
  // app's VideoFlowAction, which the custom snippet draws as the priced cards.
  // Anything else draws nothing.
  import type { Snippet } from 'svelte';
  import { Button } from '$lib/components/ui/button';
  import * as Card from '$lib/components/ui/card';
  import { Separator } from '$lib/components/ui/separator';
  import { childList, resolve } from './a2ui.mjs';
  import type { A2UIComponent, Surface } from './a2ui.d.mts';
  import Self from './A2UINode.svelte';

  let {
    surface,
    id,
    scope = undefined,
    depth = 0,
    inline = false,
    disabled = false,
    onaction,
    custom
  }: {
    surface: Surface;
    id: string;
    scope?: unknown;
    depth?: number;
    /** Inside a button: text is drawn as a span. */
    inline?: boolean;
    disabled?: boolean;
    onaction?: (name: string, sourceComponentId: string, context: Record<string, unknown>) => void;
    custom?: Snippet<[A2UIComponent]>;
  } = $props();

  const MAX_DEPTH = 12;
  const c = $derived(depth > MAX_DEPTH ? undefined : surface.components[id]);
  const value = (v: unknown) => resolve(v, surface.data, scope);
  const text = (v: unknown) => {
    const r = value(v);
    return typeof r === 'string' || typeof r === 'number' ? String(r) : '';
  };
  const headings: Record<string, string> = { h1: 'h3', h2: 'h3', h3: 'h4', h4: 'h5', h5: 'h6' };
  const headingClass: Record<string, string> = { h1: 'text-lg font-semibold', h2: 'text-base font-semibold', h3: 'text-sm font-semibold', h4: 'text-sm font-medium', h5: 'text-xs font-medium' };
  let picked = $state<string | null>(null);

  function press() {
    const event = c?.action?.event;
    if (!event || typeof event.name !== 'string') return;
    const context: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(event.context ?? {})) context[k] = value(v);
    onaction?.(event.name, id, context);
  }
  function pick(option: { label: unknown; value: unknown }) {
    const v = text(option.value);
    if (!v) return;
    picked = v;
    onaction?.('videoflow.reply', id, { reply: v });
  }
</script>

{#if c}
  {#if c.component === 'Text'}
    {@const t = text(c.text)}
    {#if inline}<span>{t}</span>
    {:else if headings[c.variant]}<svelte:element this={headings[c.variant]} class={headingClass[c.variant]}>{t}</svelte:element>
    {:else}<p class={c.variant === 'caption' ? 'text-xs text-muted-foreground' : 'text-sm'}>{t}</p>{/if}
  {:else if c.component === 'Image'}
    {@const src = text(c.url)}
    {#if src}<img {src} alt={text(c.description)} loading="lazy" class="max-h-48 w-full rounded-md border border-border object-cover" />{/if}
  {:else if c.component === 'Row' || c.component === 'Column' || c.component === 'List'}
    <div class={c.component === 'Row' ? 'flex flex-wrap items-start gap-2' : 'flex flex-col gap-2'} role={c.component === 'List' ? 'list' : undefined}>
      {#each childList(c.children, surface.data) as [child, item], i (child + ':' + i)}
        {#if c.component === 'List'}<div role="listitem"><Self {surface} id={child} scope={item} depth={depth + 1} {disabled} {onaction} {custom} /></div>
        {:else}<Self {surface} id={child} scope={item ?? scope} depth={depth + 1} {disabled} {onaction} {custom} />{/if}
      {/each}
    </div>
  {:else if c.component === 'Card'}
    <Card.Root class="gap-2 py-3"><Card.Content class="px-3"><Self {surface} id={c.child} {scope} depth={depth + 1} {disabled} {onaction} {custom} /></Card.Content></Card.Root>
  {:else if c.component === 'Button'}
    <Button size="sm" variant={c.variant === 'primary' ? 'default' : c.variant === 'borderless' ? 'ghost' : 'outline'} class="h-auto min-h-8 whitespace-normal" {disabled} onclick={press}
      ><Self {surface} id={c.child} {scope} depth={depth + 1} inline {onaction} {custom} /></Button>
  {:else if c.component === 'ChoicePicker'}
    <div class="flex flex-wrap gap-1.5" role="group">
      {#each Array.isArray(c.options) ? c.options : [] as option, i (i)}
        <Button
          size="sm"
          variant={picked === text(option.value) ? 'default' : 'secondary'}
          class="h-auto min-h-8 whitespace-normal rounded-full px-3 text-xs font-normal"
          aria-pressed={picked === text(option.value)}
          {disabled}
          onclick={() => pick(option)}>{text(option.label)}</Button>
      {/each}
    </div>
  {:else if c.component === 'Divider'}
    <Separator orientation={c.axis === 'vertical' ? 'vertical' : 'horizontal'} />
  {:else if c.component === 'VideoFlowAction'}
    {@render custom?.(c)}
  {/if}
{/if}
