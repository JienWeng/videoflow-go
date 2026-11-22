<script lang="ts">
  import * as Dialog from '$lib/components/ui/dialog';
  import { Button } from '$lib/components/ui/button';
  import { del } from '$lib/api';
  import { toast } from 'svelte-sonner';
  let {
    project,
    last,
    onclose,
    ondeleted
  }: {
    project: { id: string; name: string; is_active: boolean };
    last: boolean;
    onclose: () => void;
    ondeleted: () => void | Promise<void>;
  } = $props();
  let busy = $state(false),
    error = $state('');
  async function remove() {
    if (busy) return;
    busy = true;
    error = '';
    try {
      await del('/projects/' + encodeURIComponent(project.id));
    } catch (e) {
      error = String(e);
      busy = false;
      return;
    }
    window.dispatchEvent(new Event('videoflow:projects-changed'));
    toast.success('Project deleted');
    onclose();
    await ondeleted();
  }
</script>
<Dialog.Root open onOpenChange={open=>{if(!open&&!busy)onclose()}}>
 <Dialog.Content showCloseButton={!busy} onEscapeKeydown={e=>{if(busy)e.preventDefault()}} onInteractOutside={e=>{if(busy)e.preventDefault()}}>
  <Dialog.Header><Dialog.Title>Delete “{project.name}”?</Dialog.Title><Dialog.Description>This permanently removes its scenes, characters, asset records and render history. Media files stay on disk. {project.is_active
      ? last
        ? 'A new empty project will open.'
        : 'Your most recently updated remaining project will become active.'
      : ''}</Dialog.Description></Dialog.Header>
  {#if error}<p role="alert" class="text-sm text-destructive">{error}</p>{/if}
  <Dialog.Footer><Button variant="outline" disabled={busy} onclick={onclose}>Cancel</Button><Button
    variant="destructive"
    disabled={busy}
    onclick={remove}>{busy?'Deleting…':'Delete project'}</Button></Dialog.Footer>
 </Dialog.Content>
</Dialog.Root>
