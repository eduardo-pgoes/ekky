<script lang="ts">
  import { parseLog } from '../lib/log'

  let { log }: { log: string } = $props()

  const lines = $derived(parseLog(log))
</script>

{#if !log.trim()}
  <p class="empty">no run.log — nothing has run.</p>
{:else}
  <pre class="log">{#each lines as l, i (i)}{#if i}{'\n'}{/if}{#if l.kind === 'ekky'}<span class="ek">{l.text}</span>{:else if l.kind === 'tool'}<span class="ts">{l.ts}</span>  <span class="tool">{l.tool}:</span>{l.rest}{:else}{l.text}{/if}{/each}</pre>
{/if}

<style>
  pre.log {
    margin: 0; padding: 18px 20px; font-family: inherit; font-size: 12.5px; line-height: 1.6;
    color: var(--ink-2); background: var(--bg-2); border: 1px solid var(--line); overflow-x: auto; white-space: pre-wrap; font-weight: 300;
  }
  pre.log .ek { color: var(--accent); font-weight: 400; }
  pre.log .ts { color: var(--dim); }
  pre.log .tool { color: var(--ink); font-weight: 400; }
</style>
