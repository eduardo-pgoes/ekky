<script lang="ts">
  import { tick } from 'svelte'
  import { OWNERS, type Row as RowData, type Task } from '../lib/api'
  import { finishedIn, marker } from '../lib/fx'
  import Row from './Row.svelte'

  let { rows, selected, finishedOpen, onselect, ontogglefinished, el = $bindable() }: {
    rows: RowData[]
    selected: string | null
    finishedOpen: boolean
    onselect: (id: string) => void
    ontogglefinished: () => void
    el?: HTMLElement
  } = $props()

  type Entry = { row: RowData; heading: string | null }
  type Group = { owner: Task['owner']; entries: Entry[]; collapsed: boolean }

  const groups = $derived<Group[]>(OWNERS.map((owner) => {
    const mine = rows.filter((t) => t.owner === owner)
    let rfc = ''
    const entries = mine.map((row) => {
      const heading = row.rfc === rfc ? null : (rfc = row.rfc)
      return { row, heading }
    })
    return { owner, entries, collapsed: owner === 'finished' && !finishedOpen }
  }))

  async function toggleFinished() {
    ontogglefinished()
    await tick()
    if (finishedOpen && el) finishedIn(el.querySelectorAll('[data-owner="finished"] .row'))
  }
</script>

<aside class="backlog" bind:this={el}>
  <div class="marker" use:marker={{ selected, rows }}></div>
  {#each groups as group (group.owner)}
    <div class="group" class:is-collapsed={group.collapsed} data-owner={group.owner}>
      <div class="group-h" class:is-me={group.owner === 'me'}><span class="gname" data-text={group.owner}>{group.owner}</span><span class="count">{group.entries.length}</span></div>
      <div class="group-body">
        {#if !group.entries.length}<div class="group-none">none</div>{/if}
        {#each group.entries as entry (entry.row.id)}
          {#if entry.heading !== null}
            <div class="rfc-h"><span>{entry.heading === '_' ? '_ · stray' : entry.heading}</span><span class="rule"></span></div>
          {/if}
          <Row t={entry.row} selected={entry.row.id === selected} {onselect} />
        {/each}
      </div>
      {#if group.owner === 'finished'}
        <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
        <div class="finished-toggle" onclick={toggleFinished}>{group.collapsed ? 'show' : 'hide'} finished</div>
      {/if}
    </div>
  {/each}
</aside>

<style>
  .backlog { position: relative; overflow-y: auto; padding: 28px 0 60px; scrollbar-width: none; }
  .backlog::-webkit-scrollbar { display: none; }
  .marker { position: absolute; left: 0; top: 0; width: 2px; height: 0; background: var(--accent); opacity: 0; }

  .group { margin-bottom: 36px; }
  .group-h {
    display: flex; align-items: baseline; justify-content: space-between;
    padding: 0 32px; margin-bottom: 14px;
    font-size: 11px; letter-spacing: 0.24em; text-transform: uppercase; color: var(--muted);
  }
  .group-h .count { font-variant-numeric: tabular-nums; color: var(--dim); }
  .group-h.is-me { color: var(--accent); }
  .group-h.is-me .count { color: var(--accent); }
  .group-none { padding: 0 32px; color: var(--dim); font-style: italic; font-weight: 300; }

  .rfc-h {
    padding: 10px 32px 6px; font-size: 11px; letter-spacing: 0.14em; text-transform: uppercase; color: var(--dim);
    display: flex; gap: 12px; align-items: baseline;
  }
  .rfc-h .rule { flex: 1; height: 1px; background: var(--line); transform-origin: left; }

  .finished-toggle { padding: 0 32px; color: var(--dim); font-size: 11px; letter-spacing: 0.14em; text-transform: uppercase; }
  .finished-toggle:hover { color: var(--muted); }
  .group.is-collapsed .group-body { display: none; }
</style>
