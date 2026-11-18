<script lang="ts">
  import { get, put } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import { protectDraft } from '$lib/protect-draft';

  let { scene, projectStyle, disabled = false, onchanged }: {
    scene: { id: string; updated_at: string; scene_json?: { generation_brief?: { style?: string } } };
    projectStyle?: { name?: string; style_prompt?: string };
    disabled?: boolean;
    onchanged: () => Promise<void>;
  } = $props();

  const current = $derived(scene.scene_json?.generation_brief?.style || projectStyle?.style_prompt || '');
  let preset = $state('');
  let prompt = $state('');
  let catalog = $state<{ id: string; name: string }[] | null>(null);
  let loading = $state(false);
  let saving = $state(false);
  let error = $state('');
  let catalogError = $state('');
  let status = $state('');
  protectDraft(() => !!preset && !saving);

  async function load() {
    if (catalog || loading) return;
    loading = true;
    catalogError = '';
    try {
      catalog = (await get('/style-presets')).presets ?? [];
    } catch {
      catalogError = 'Could not load presets. You can still enter a custom look.';
    } finally {
      loading = false;
    }
  }

  async function save() {
    if (!preset || saving || disabled || (preset === 'custom' && !prompt.trim())) return;
    saving = true;
    error = '';
    status = '';
    try {
      await put(`/scenes/${encodeURIComponent(scene.id)}/style`, {
        preset_id: preset,
        prompt: preset === 'custom' ? prompt.trim() : '',
        expected_updated_at: scene.updated_at,
      });
      preset = '';
      status = 'Scene look saved. Refresh any references that still show the previous look before generating a new storyboard.';
      await onchanged();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      saving = false;
    }
  }
</script>

<details class="my-3 rounded-lg border border-border p-3" ontoggle={(event) => { if (event.currentTarget.open) void load(); }}>
  <summary class="cursor-pointer py-1 text-sm font-medium">Scene look</summary>
  <div class="mt-3 space-y-3">
    {#if current}<p class="whitespace-pre-line break-words text-sm text-muted-foreground">{current}</p>{/if}
    <form class="space-y-3" onsubmit={(event) => { event.preventDefault(); void save(); }}>
      <label class="grid gap-1 text-sm">
        Change look
        <select bind:value={preset} disabled={saving || disabled} class="min-h-11 w-full rounded-md border border-input bg-background px-3 text-sm" onchange={() => { if (preset === 'custom' && !prompt) prompt = current; }}>
          <option value="">Choose a look…</option>
          <option value="custom">Write a custom look</option>
          <option value="project-style">Use current project look</option>
          {#each catalog ?? [] as option (option.id)}
            <option value={option.id}>{option.name}</option>
          {/each}
        </select>
      </label>
      {#if preset === 'custom'}
        <label class="grid gap-1 text-sm">
          Visual direction
          <textarea bind:value={prompt} required maxlength="5000" rows="4" disabled={saving || disabled} class="w-full rounded-md border border-input bg-background p-2" placeholder="Describe the medium, lighting, materials and composition"></textarea>
        </label>
      {/if}
      {#if catalogError}<p role="status" class="text-sm text-muted-foreground">{catalogError} <button type="button" class="underline" onclick={load}>Retry</button></p>{/if}
      {#if preset}
        <p class="text-xs text-muted-foreground">This changes future generation for this scene. Existing shots, references and videos stay saved.</p>
        <Button type="submit" size="sm" disabled={saving || disabled || (preset === 'custom' && !prompt.trim())}>{saving ? 'Saving…' : 'Save scene look'}</Button>
      {/if}
    </form>
    {#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
    {#if status}<p role="status" class="text-sm">{status}</p>{/if}
  </div>
</details>
