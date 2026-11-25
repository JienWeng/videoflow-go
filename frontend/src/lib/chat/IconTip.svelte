<script lang="ts">
  // Icon-only button with a tooltip; the label doubles as the accessible name.
  import type { Component } from 'svelte';
  import { mergeProps } from 'bits-ui';
  import * as Tooltip from '$lib/components/ui/tooltip';
  import { Button, type ButtonProps } from '$lib/components/ui/button';
  let {
    label,
    tip,
    icon: Icon,
    iconClass = 'size-3.5',
    variant = 'ghost',
    size = 'icon-sm',
    class: className = 'text-muted-foreground',
    ...rest
  }: ButtonProps & { label: string; tip?: string; icon: Component<{ class?: string }>; iconClass?: string } = $props();
</script>

<Tooltip.Provider delayDuration={300}>
  <Tooltip.Root>
    <Tooltip.Trigger>
      {#snippet child({ props })}
        <Button {...mergeProps(props, rest)} {variant} {size} class={className} aria-label={label}><Icon class={iconClass} /></Button>
      {/snippet}
    </Tooltip.Trigger>
    <Tooltip.Content side="top">{tip ?? label}</Tooltip.Content>
  </Tooltip.Root>
</Tooltip.Provider>
