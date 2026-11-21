import { beforeNavigate } from '$app/navigation';
import { onMount } from 'svelte';

/** Guard client navigation and browser unload without persisting private drafts. */
export function protectDraft(isDirty: () => boolean, message = 'You have unsaved changes. Discard them and leave?') {
  beforeNavigate((navigation) => {
    if (!navigation.willUnload && isDirty() && !window.confirm(message)) navigation.cancel();
  });
  onMount(() => {
    const beforeUnload = (event: BeforeUnloadEvent) => {
      if (!isDirty()) return;
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', beforeUnload);
    return () => window.removeEventListener('beforeunload', beforeUnload);
  });
}
