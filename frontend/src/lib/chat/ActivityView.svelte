<script lang="ts">
  import * as Trace from '$lib/components/ui/trace/index.js';
  import * as CodeBlock from '$lib/components/ui/code-block/index.js';
  import * as Marker from '$lib/components/ui/marker/index.js';
  import * as HoverCard from '$lib/components/ui/hover-card/index.js';
  import * as Collapsible from '$lib/components/ui/collapsible/index.js';
  import { Button } from '$lib/components/ui/button/index.js';
  import CircleIcon from '@lucide/svelte/icons/circle';
  import CircleCheckIcon from '@lucide/svelte/icons/circle-check';
  import CircleAlertIcon from '@lucide/svelte/icons/circle-alert';
  import CircleDotIcon from '@lucide/svelte/icons/circle-dot';
  import FileIcon from '@lucide/svelte/icons/file';
  import FolderIcon from '@lucide/svelte/icons/folder';
  import { normalizeActivity, type ActivityEvent, type ActivityStatus, type ActivityFile } from './activity.mjs';
  let { events }: { events: unknown } = $props();
  const normalized = $derived(normalizeActivity(events));
  function traceStatus(status?: ActivityStatus) {
    return status === 'failed' ? 'error' : status === 'running' ? 'running' : status === 'pending' || status === 'skipped' ? 'pending' : undefined;
  }
</script>

{#snippet icon(status?: ActivityStatus)}
  {#if status === 'failed'}<CircleAlertIcon aria-hidden="true" />
  {:else if status === 'done'}<CircleCheckIcon aria-hidden="true" />
  {:else if status === 'running'}<CircleDotIcon aria-hidden="true" />
  {:else}<CircleIcon aria-hidden="true" />{/if}
{/snippet}

{#snippet fileRows(files: ActivityFile[])}
  {#each files as file, index (index)}
    {#if file.type === 'folder'}
      <Collapsible.Root>
        <Collapsible.Trigger class="flex items-center gap-2 py-1 text-sm" title={file.path ?? file.name}>
          <FolderIcon class="size-4" aria-hidden="true" />{file.name}
        </Collapsible.Trigger>
        <Collapsible.Content><div class="ml-4 border-l border-border pl-3">{@render fileRows(file.children)}</div></Collapsible.Content>
      </Collapsible.Root>
    {:else}
      <div class="flex items-center gap-2 py-1 text-sm" title={file.path ?? file.name}><FileIcon class="size-4" aria-hidden="true" />{file.name}</div>
    {/if}
  {/each}
{/snippet}

{#snippet line(event: ActivityEvent)}
  <Trace.Line status={traceStatus(event.status)} title={event.title}>
    <Trace.Icon>{@render icon(event.status)}</Trace.Icon>
    <Trace.Title>{event.title}</Trace.Title>
    {#if event.status}<Trace.Description>{event.status}</Trace.Description>{/if}
  </Trace.Line>
{/snippet}

{#snippet activityRows(rows: ActivityEvent[])}
  <Trace.Root>
    {#each rows as event, index (index)}
      {@const hasDetails = event.detail || event.code || event.items.length || event.sources.length || event.files.length || event.suggestions.length || event.children.length || event.usage}
      {#if hasDetails}
        <Trace.Collapsible>
          <Trace.CollapsibleTrigger>{@render line(event)}</Trace.CollapsibleTrigger>
          <Trace.CollapsibleContent class="flex min-w-0 flex-col gap-3">
            {#if event.detail}<p class="whitespace-pre-wrap break-words text-sm" class:text-destructive={event.status === 'failed'}>{event.detail}</p>{/if}
            {#if event.code}
              <CodeBlock.Root size="sm" variant={event.status === 'failed' ? 'destructive' : 'default'}>
                <CodeBlock.Content wrap class="max-h-64" code={event.code} />
              </CodeBlock.Root>
            {/if}
            {#if event.items.length}
              <Trace.Root>
                {#each event.items as item, itemIndex (itemIndex)}
                  {#if item.detail}
                    <Trace.Collapsible>
                      <Trace.CollapsibleTrigger><Trace.Line status={traceStatus(item.status)} title={item.title}><Trace.Icon>{@render icon(item.status)}</Trace.Icon><Trace.Title>{item.title}</Trace.Title>{#if item.status}<Trace.Description>{item.status}</Trace.Description>{/if}</Trace.Line></Trace.CollapsibleTrigger>
                      <Trace.CollapsibleContent class="whitespace-pre-wrap break-words">{item.detail}</Trace.CollapsibleContent>
                    </Trace.Collapsible>
                  {:else}
                    <Trace.Line status={traceStatus(item.status)} title={item.title}><Trace.Icon>{@render icon(item.status)}</Trace.Icon><Trace.Title>{item.title}</Trace.Title>{#if item.status}<Trace.Description>{item.status}</Trace.Description>{/if}</Trace.Line>
                  {/if}
                {/each}
              </Trace.Root>
            {/if}
            {#if event.sources.length}
              <Trace.Root>
                {#each event.sources as source (source.url)}
                  {#if event.kind === 'inline-citation'}
                    <HoverCard.Root>
                      <HoverCard.Trigger>{#snippet child({ props })}<Button {...props} variant="secondary" size="sm">{source.title}</Button>{/snippet}</HoverCard.Trigger>
                      <HoverCard.Content class="max-w-xs"><p class="font-medium">{source.title}</p>{#if source.description}<p class="text-sm text-muted-foreground">{source.description}</p>{/if}<a href={source.url} target="_blank" rel="noopener noreferrer" class="text-sm underline">Open source<span class="sr-only"> (opens in a new tab)</span></a></HoverCard.Content>
                    </HoverCard.Root>
                  {:else}
                    <a href={source.url} target="_blank" rel="noopener noreferrer"><Trace.Line title={source.title}><Trace.Title>{source.title}<span class="sr-only"> (opens in a new tab)</span></Trace.Title></Trace.Line></a>
                  {/if}
                {/each}
              </Trace.Root>
            {/if}
            {#if event.files.length}<div>{@render fileRows(event.files)}</div>{/if}
            {#if event.usage}
              <Marker.Root><Marker.Content>{event.usage.used.toLocaleString()} / {event.usage.limit.toLocaleString()} tokens ({Math.round(event.usage.used / event.usage.limit * 100)}% context)</Marker.Content></Marker.Root>
              <dl class="grid grid-cols-2 gap-1 text-xs text-muted-foreground">{#each ['input', 'output', 'reasoning'] as field}{@const value = event.usage[field as 'input' | 'output' | 'reasoning']}{#if value !== undefined}<dt class="capitalize">{field}</dt><dd>{value.toLocaleString()}</dd>{/if}{/each}</dl>
            {/if}
            {#if event.suggestions.length}<ul class="list-disc space-y-1 pl-4 text-sm">{#each event.suggestions as suggestion, suggestionIndex (suggestionIndex)}<li>{suggestion}</li>{/each}</ul>{/if}
            {#if event.children.length}{@render activityRows(event.children)}{/if}
          </Trace.CollapsibleContent>
        </Trace.Collapsible>
      {:else}{@render line(event)}{/if}
    {/each}
  </Trace.Root>
{/snippet}

{#if normalized.length}<div class="activity-view min-w-0" aria-label="Application activity">{@render activityRows(normalized)}</div>{/if}

<style>
  @media (prefers-reduced-motion: reduce) {
    .activity-view :global(*) { animation: none !important; transition: none !important; }
  }
</style>
