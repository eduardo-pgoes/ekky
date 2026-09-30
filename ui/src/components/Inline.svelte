<script lang="ts">
  import { spans } from '../lib/inline'
  let { text }: { text: string } = $props()
</script>
{#each spans(text) as span, i (i)}{#if span.code}<code>{span.text}</code>{:else}{span.text}{/if}{/each}
