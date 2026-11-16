<script lang="ts">
  import * as Dialog from '$lib/components/ui/dialog';
  import {Button} from '$lib/components/ui/button';
  import StoryCreator from './StoryCreator.svelte';
  import {storyCreation} from './story-creation.svelte';
  let creator:StoryCreator|undefined=$state();
  function close(){if(creator?.canClose()!==false)storyCreation.open=false;}
</script>
<Dialog.Root open={storyCreation.open} onOpenChange={value=>{if(value)storyCreation.open=true;else close()}}>
  <Dialog.Content class="max-h-[90dvh] overflow-y-auto sm:max-w-3xl" showCloseButton={false} overlayClass="bg-black/55 supports-backdrop-filter:backdrop-blur-none" onEscapeKeydown={event=>{event.preventDefault();close()}} onInteractOutside={event=>{event.preventDefault();close()}}>
    <Dialog.Header><Dialog.Title>Create story</Dialog.Title><Dialog.Description>Plan your story and storyboard. Review references before rendering.</Dialog.Description></Dialog.Header>
    <StoryCreator embedded bind:this={creator}/>
    <Button variant="outline" onclick={close}>Close</Button>
  </Dialog.Content>
</Dialog.Root>
