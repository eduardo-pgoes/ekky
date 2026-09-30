<script lang="ts">
  import type { Task } from '../lib/api'
  import { roman } from '../lib/format'
  import { paragraphs } from '../lib/inline'
  import Inline from './Inline.svelte'

  let { task }: { task: Task } = $props()

  const r = $derived(task.result)
  const forks = $derived(r?.decisions ?? [])
</script>

{#if !r}
  <div class="block"><p class="empty">no result card. {#if task.shown === 'failed'}the reason is the last <code>ekky:</code> line but one of the log.{:else if task.shown === 'running'}the attempt is still in flight — watch the log.{:else}nothing has run yet.{/if}</p></div>
{:else}
  {#if r.question}
    <div class="block"><div class="label is-accent">question</div><p class="question split"><Inline text={r.question} /></p></div>
  {/if}
  {#if r.summary}
    <div class="block"><div class="label">summary</div><div class="prose split"><p><Inline text={r.summary} /></p></div></div>
  {/if}
  {#if forks.length}
    <div class="block"><div class="label">decisions <span class="sep">·</span> {forks.length} fork{forks.length > 1 ? 's' : ''}</div>
      {#each forks as d, i (i)}
        <div class="fork"><div class="q"><span class="fork-n">{roman(i + 1)}</span><Inline text={d.fork} /></div><span class="k chose">chose</span><span class="v chose"><Inline text={d.chosen} /></span><span class="k">rejected</span><span class="v"><Inline text={d.rejected} /></span><span class="k">why</span><span class="v"><Inline text={d.why} /></span></div>
      {/each}
    </div>
  {/if}
  {#if r.deviations}
    <div class="block"><div class="label">deviations</div><div class="prose">{#each paragraphs(r.deviations) as p, i (i)}<p><Inline text={p} /></p>{/each}</div></div>
  {/if}
  {#if r.verify}
    <div class="block"><div class="label">verify</div><pre class="verify">{r.verify.trim()}</pre></div>
  {/if}
{/if}

<style>
  .block { margin-bottom: 34px; }
  .label { font-size: 11px; letter-spacing: 0.22em; text-transform: uppercase; color: var(--muted); margin-bottom: 10px; }
  .label.is-accent { color: var(--accent); }
  .prose { font-size: 14.5px; line-height: 1.65; color: var(--ink); font-weight: 300; }
  .prose p { margin: 0 0 12px; }
  .prose :global(code), .fork :global(code) { font-family: inherit; color: var(--accent-2); font-weight: 400; }
  .question { font-family: var(--serif); font-size: 24px; line-height: 1.3; color: var(--ink); margin: 0 0 8px; }

  .fork { display: grid; grid-template-columns: 84px 1fr; column-gap: 18px; row-gap: 6px; margin-bottom: 30px; }
  .fork .q { grid-column: 1 / -1; font-size: 15px; line-height: 1.45; color: var(--ink); margin-bottom: 4px; font-weight: 400; }
  .fork .k { font-size: 11px; letter-spacing: 0.2em; text-transform: uppercase; color: var(--muted); padding-top: 3px; }
  .fork .k.chose { color: var(--accent); }
  .fork .v { font-size: 13.5px; line-height: 1.6; color: var(--ink-2); font-weight: 300; }
  .fork .v.chose { color: var(--ink); }
  .fork-n { font-family: var(--serif); font-style: italic; color: var(--accent); font-size: 19px; margin-right: 12px; }

  pre.verify {
    margin: 0; padding: 18px 20px; font-family: inherit; font-size: 12.5px; line-height: 1.6;
    color: var(--ink-2); background: var(--bg-2); border: 1px solid var(--line); overflow-x: auto; white-space: pre-wrap; font-weight: 300;
  }
</style>
