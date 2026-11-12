<script lang="ts">
  /**
   * First-run welcome overlay. Points users to the simple Create video flow,
   * then gets out of the way. Dismissal is
   * persisted in localStorage['videoflow.onboarded'] so it shows exactly once.
   *
   * Rendered only on the Studio route (see +page.svelte). Closing — via the
   * button, the backdrop, or Escape — sets the flag; "Show me around" also
   * opens the command palette so the user lands somewhere actionable.
   */
  import { Button } from '$lib/components/ui/button';
  import * as Dialog from '$lib/components/ui/dialog';
  import {
    LayoutGrid,
    PanelLeft,
    Sparkles,
    Command,
  } from '@lucide/svelte';

  const STORAGE_KEY = 'videoflow.onboarded';

  let { onpalette }: { onpalette?: () => void } = $props();

  // Default open=false so SSR/first-paint never flashes the overlay; onMount
  // flips it on only when the flag is absent.
  let open = $state(false);

  $effect(() => {
    try {
      if (!localStorage.getItem(STORAGE_KEY)) open = true;
    } catch {
      /* storage blocked — just skip onboarding */
    }
  });

  function dismiss() {
    open = false;
    try {
      localStorage.setItem(STORAGE_KEY, '1');
    } catch {
      /* ignore */
    }
  }

  function showAround() {
    dismiss();
    onpalette?.();
  }

  function onKeydown(e: KeyboardEvent) {
    if (open && e.key === 'Escape') dismiss();
  }

  const surfaces = [
    {
      icon: LayoutGrid,
      title: 'The canvas',
      body: 'Your project as a living map — characters, scenes, shots and renders. Click a node to work on it.'
    },
    {
      icon: PanelLeft,
      title: 'Guided or hands-on',
      body: 'Use the guided creator for a first story, or work directly in Characters, Assets and Scenes & shots.'
    }
  ];

  const steps = [
    'Set up your AI session and media providers in Settings.',
    'Describe your story and review the scenes and storyboard images.',
    'Approve the references, render each scene, then assemble clips in Videos.'
  ];
</script>

<svelte:window onkeydown={onKeydown} />

<Dialog.Root {open} onOpenChange={(value) => { if (!value) dismiss(); }}>
    <Dialog.Content class="max-h-[calc(100dvh-2rem)] overflow-y-auto p-6 sm:max-w-xl" aria-describedby="onboarding-description">
      <div class="mb-1 flex items-center gap-2">
        <Sparkles class="size-5 text-primary" />
        <Dialog.Title class="text-lg font-semibold">Welcome to VideoFlow</Dialog.Title>
      </div>
      <Dialog.Description id="onboarding-description" class="mb-5 text-sm text-muted-foreground">Build a story, review the references, then choose what to render.</Dialog.Description>

      <div class="mb-5 grid gap-3 sm:grid-cols-2">
        {#each surfaces as s (s.title)}
          <div class="rounded-lg border border-border bg-card/50 p-3">
            <s.icon class="mb-1.5 size-4 text-primary" />
            <p class="text-sm font-medium">{s.title}</p>
            <p class="mt-0.5 text-xs text-muted-foreground">{s.body}</p>
          </div>
        {/each}
      </div>

      <div class="mb-5 rounded-lg border border-border bg-card/50 p-4">
        <p class="mb-2 text-sm font-medium">Your first video</p>
        <ol class="space-y-1.5 text-sm text-muted-foreground">
          {#each steps as step, i (i)}
            <li class="flex gap-2">
              <span class="flex size-5 shrink-0 items-center justify-center rounded-full bg-primary/10 text-[11px] font-semibold text-primary">
                {i + 1}
              </span>
              <span>{step}</span>
            </li>
          {/each}
        </ol>
      </div>

      <div class="flex flex-wrap items-center justify-between gap-3">
        <p class="flex items-center gap-1.5 text-xs text-muted-foreground">
          <Command class="size-3.5" />
          Press
          <kbd class="rounded border border-border bg-muted px-1 py-0.5 text-[10px] font-medium">Ctrl/⌘ K</kbd>
          anytime to search and jump.
        </p>
        <div class="flex gap-2">
          <Button variant="ghost" size="sm" onclick={dismiss}>Got it</Button>
          <Button size="sm" onclick={showAround}>Show me around</Button>
        </div>
      </div>
    </Dialog.Content>
</Dialog.Root>
