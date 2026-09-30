<script lang="ts">
  import { parseRecord } from '../lib/record'
  import Inline from './Inline.svelte'

  let { md }: { md: string } = $props()

  const parsed = $derived(parseRecord(md))
</script>

{#if !md.trim()}
  <p class="empty">no task.md</p>
{:else}
  {#if parsed.frontmatter.length}
    <div class="fm">{#each parsed.frontmatter as [k, v], i (i)}<span class="k">{k}</span><span class="v">{v}</span>{/each}</div>
  {/if}
  <div class="rec">
    {#each parsed.blocks as b, i (i)}
      {#if b.kind === 'list'}<ul>{#each b.items as item, j (j)}<li><Inline text={item} /></li>{/each}</ul>
      {:else if b.kind === 'heading'}<h3>{b.text}</h3>
      {:else}<p><Inline text={b.text} /></p>{/if}
    {/each}
  </div>
{/if}

<style>
  .rec h3 { font-size: 11px; letter-spacing: 0.22em; text-transform: uppercase; color: var(--muted); margin: 30px 0 10px; font-weight: 400; }
  .rec h3:first-child { margin-top: 0; }
  .rec p, .rec li { font-size: 14.5px; line-height: 1.65; color: var(--ink); font-weight: 300; }
  .rec ul { padding-left: 18px; margin: 0; }
  .rec li { margin-bottom: 6px; }
  .rec li::marker { color: var(--dim); }
  .rec :global(code) { font-family: inherit; color: var(--accent-2); font-weight: 400; }
  .fm { display: grid; grid-template-columns: 84px 1fr; column-gap: 18px; row-gap: 4px; margin-bottom: 34px; }
  .fm .k { font-size: 11px; letter-spacing: 0.2em; text-transform: uppercase; color: var(--muted); padding-top: 2px; }
  .fm .v { font-size: 12.5px; color: var(--ink-2); word-break: break-all; }
</style>
