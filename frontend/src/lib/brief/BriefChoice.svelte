<script lang="ts">
  import { tick } from 'svelte';

  let { label, value, options, maxlength, onchange }: {
    label: string;
    value: string;
    options: string[];
    maxlength: number;
    onchange: (value: string) => void;
  } = $props();

  const controlClass = 'min-h-11 w-full rounded-md border border-input bg-background px-3 py-2 text-sm';
  let typing = $state(false);
  let input = $state<HTMLInputElement>();
  const custom = $derived(typing || (!!value && !options.includes(value)));

  async function choose(choice: string) {
    typing = choice === '__custom';
    onchange(typing ? '' : choice);
    if (typing) {
      await tick();
      input?.focus();
    }
  }
</script>

<div class="grid gap-2">
  <label class="grid gap-1 text-sm">
    {label}
    <select aria-label={label} class={controlClass} value={custom ? '__custom' : value} onchange={(event) => choose(event.currentTarget.value)}>
      <option value="">Choose…</option>
      {#each options as option}
        <option value={option}>{option}</option>
      {/each}
      <option value="__custom">Custom…</option>
    </select>
  </label>
  {#if custom}
    <input bind:this={input} class={controlClass} aria-label={'Custom ' + label.toLowerCase()}
      placeholder={'Type your ' + label.toLowerCase()} {value} {maxlength}
      oninput={(event) => onchange(event.currentTarget.value)} />
  {/if}
</div>
