<script lang="ts">
  // Switch the service that makes the videos with one click. Free, but it changes the
  // price per second, so it always waits for the owner, even in Full auto.
  import { put } from '$lib/api';
  import { Button } from '$lib/components/ui/button';

  type Choice = { mode: 'manual' | 'cheapest'; model: string; provider: string; label: string; per_second: number };
  let { choice, onswitched }: { choice: Choice; onswitched: (text: string) => void } = $props();
  let busy = $state(false), error = $state('');
  const usd = (v: number) => '$' + v.toFixed(3).replace(/0$/, '');
  async function use() {
    busy = true; error = '';
    try {
      const r = await put('/video-services/routing', { mode: choice.mode, model: choice.model, provider: choice.provider });
      const row = (r.services ?? []).find((s: any) => s.in_use);
      const price = row ? ` at ${usd(row.per_second)} per second` : '';
      onswitched(`${r.current?.label ?? choice.label} makes your videos now${price}. Say “make the videos” when you are ready.`);
    } catch (e) { error = (e as Error).message.replace(/^\d{3}: /, ''); } finally { busy = false; }
  }
</script>

<div class="mt-2 grid gap-2 rounded-md border border-border p-3 text-sm">
  <div class="flex flex-wrap items-center gap-2">
    <Button size="sm" class="rounded-full px-4" disabled={busy} onclick={use}>
      {choice.mode === 'cheapest' ? 'Always use the cheapest service' : `Use ${choice.label} for videos`} · {usd(choice.per_second)} per second
    </Button>
    <span class="text-xs text-muted-foreground">Free to switch. Nothing is made yet.{choice.mode === 'cheapest' ? ` Right now that is ${choice.label}.` : ''}</span>
  </div>
  {#if error}<p role="alert" class="text-xs text-destructive">{error}</p>{/if}
</div>
