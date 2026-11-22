<script lang="ts">
  import * as Dialog from '$lib/components/ui/dialog';
  import {Button} from '$lib/components/ui/button';
  import StoryCreator from './StoryCreator.svelte';
  import {storyCreation} from './story-creation.svelte';
  let creator:StoryCreator|undefined=$state();
  function close(){if(creator?.canClose()!==false)storyCreation.open=false;}
</script>
<Dialog.Root open={storyCreation.open} onOpenChange={value=>{if(value)storyCreation.open=true;else close()}}>
  <Dialog.Content
    class="flex max-h-[90dvh] flex-col overflow-hidden sm:max-w-3xl"
    showCloseButton={false}
    overlayClass="bg-black/55 supports-backdrop-filter:backdrop-blur-none"
    onEscapeKeydown={event=>{event.preventDefault();close()}}
    onInteractOutside={event=>{event.preventDefault();close()}}>
    <Dialog.Header><Dialog.Title>Create story</Dialog.Title><Dialog.Description>Create your story, references and storyboard together.</Dialog.Description></Dialog.Header>
    <div class="min-h-0 flex-1 overflow-y-auto pr-1"><StoryCreator embedded bind:this={creator}/></div>
    <div class="flex shrink-0 justify-end border-t border-border pt-4"><Button variant="outline" class="min-h-11" onclick={close}>Close</Button></div>
  </Dialog.Content>
</Dialog.Root>
