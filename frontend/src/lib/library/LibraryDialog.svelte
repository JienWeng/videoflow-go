<script lang="ts">
 import * as Dialog from '$lib/components/ui/dialog';
 import {Button} from '$lib/components/ui/button';
 import LibraryWorkspace from './LibraryWorkspace.svelte';
 import {library} from './library.svelte';
 let workspace:LibraryWorkspace|undefined=$state();
 function close(){if(workspace?.canClose()!==false){library.open=false;window.dispatchEvent(new Event('videoflow-library-changed'));}}
</script>
<Dialog.Root open={library.open} onOpenChange={value=>{if(value)library.open=true;else close()}}>
 <Dialog.Content class="max-h-[90dvh] overflow-y-auto p-0 sm:max-w-6xl" showCloseButton={false} overlayClass="bg-black/55 supports-backdrop-filter:backdrop-blur-none" onEscapeKeydown={event=>{event.preventDefault();close()}} onInteractOutside={event=>{event.preventDefault();close()}}>
 <Dialog.Header class="flex-row items-center justify-between px-6 pt-5"><div><Dialog.Title>Library</Dialog.Title><Dialog.Description>Characters and reference assets for this project.</Dialog.Description></div><Button variant="ghost" onclick={close}>Close library</Button></Dialog.Header>
 <LibraryWorkspace embedded bind:this={workspace}/>
 </Dialog.Content>
</Dialog.Root>
