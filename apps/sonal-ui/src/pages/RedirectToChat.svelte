<script>
  import { onMount } from 'svelte'
  import { replace } from 'svelte-spa-router'

  onMount(() => {
    replace('/chat')
  })
</script>

<p class="muted" role="status">Opening chat…</p>

<style>
  .muted {
    margin: 0;
    color: var(--text);
    font-size: var(--font-size-body);
    line-height: 1.5;
  }
</style>
