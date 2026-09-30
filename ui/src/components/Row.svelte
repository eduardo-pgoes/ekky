<script lang="ts">
  import type { Row } from '../lib/api'
  import { dur, money } from '../lib/format'
  import { liveLine, rowHover } from '../lib/fx'

  let { t, selected, onselect }: { t: Row; selected: boolean; onselect: (id: string) => void } = $props()

  const live = $derived(t.shown === 'running')
  const meta = $derived.by(() => {
    const bits: { status?: true; text: string }[] = [{ status: true, text: t.shown }, { text: t.task }]
    if (t.shown === 'blocked' && t.upstream) bits.push({ text: `after ${t.upstream}` })
    if (t.result && t.result.cost_usd) bits.push({ text: money(t.result.cost_usd) }, { text: dur(t.result.duration_ms) })
    return bits
  })
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div
  class="row"
  class:is-me={t.owner === 'me'}
  class:is-selected={selected}
  data-id={t.id}
  style="--depth:{t.depth}"
  use:rowHover={() => selected}
  onclick={() => onselect(t.id)}
>
  <div class="title">{t.title || t.task}</div>
  {#if live}
    <div class="live" data-id={t.id} use:liveLine={t.id}>running<span class="sep">·</span>{t.task}</div>
  {:else}
    <div class="meta">{#each meta as bit, i (i)}{#if i}<span class="sep">·</span>{/if}{#if bit.status}<span class="st">{bit.text}</span>{:else}{bit.text}{/if}{/each}</div>
  {/if}
  <span class="hover-rule"></span>
</div>

<style>
  .row {
    position: relative; padding: 9px 32px 9px calc(32px + var(--depth) * 18px);
    display: grid; grid-template-columns: 1fr; gap: 2px;
    transition: background-color 140ms ease-out;
  }
  .row::before {
    content: ''; position: absolute; left: calc(32px + var(--depth) * 18px - 12px); top: 0; bottom: 0; width: 1px;
    background: var(--line); opacity: calc(min(var(--depth), 1));
  }
  .row.is-selected { background: var(--bg-2); }
  .row .title { font-size: 14px; color: var(--ink-2); line-height: 1.35; font-weight: 400; }
  .row.is-selected .title { color: var(--ink); }
  .row .meta { font-size: 11px; color: var(--muted); letter-spacing: 0.02em; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .row .meta .st { color: var(--muted); }
  .row.is-me .meta .st { color: var(--accent); }
  .row .meta .sep { color: var(--dim); margin: 0 6px; }
  .row .live { font-size: 11px; color: var(--ink-2); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; font-weight: 300; }
  .row .live::before { content: '▮ '; color: var(--accent); animation: blink 1.1s steps(2) infinite; }
  @keyframes blink { 50% { opacity: 0; } }
  .row .hover-rule { position: absolute; left: 0; right: 0; bottom: 0; height: 1px; background: var(--dim); transform: scaleX(0); transform-origin: left; }
</style>
