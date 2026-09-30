<script lang="ts">
  import { untrack } from 'svelte'
  import { TABS, type Detail, type Tab } from '../lib/api'
  import { dur, money } from '../lib/format'
  import { SCRAMBLE, fadeOut, scramble, tabHover, tabIndicator, tabsIn, viewIn } from '../lib/fx'
  import LogView from './LogView.svelte'
  import RecordView from './RecordView.svelte'
  import ResultView from './ResultView.svelte'

  let { detail, tab, ontab, error = null, innerEl = $bindable(), scrollEl = $bindable() }: {
    detail: Detail | null
    tab: Tab
    ontab: (tab: Tab) => void
    error?: string | null
    innerEl?: HTMLElement
    scrollEl?: HTMLElement
  } = $props()

  let viewEl = $state<HTMLElement>()
  let viewTab = $state<Tab>(untrack(() => tab))
  let viewDelay = $state(0.4)
  let shownId = ''

  const task = $derived(detail?.task)
  const crumbText = $derived(
    !detail || !task ? ''
      : task.rfc === '_' ? 'stray'
      : `RFC ${task.rfc}${detail.rfcTitle ? ` — ${detail.rfcTitle}` : ''}`,
  )
  const titleText = $derived(task ? task.title || task.task : '')
  const headBits = $derived.by(() => {
    if (!task) return []
    const bits: { status?: true; text: string }[] = [
      { status: true, text: task.shown },
      { text: task.pr || task.branch || '—' },
      { text: task.model || '—' },
    ]
    if (task.result && task.result.cost_usd) {
      bits.push({ text: money(task.result.cost_usd) }, { text: dur(task.result.duration_ms) }, { text: `${task.result.turns} turns` })
    }
    return bits
  })
  const headText = $derived(headBits.map((b) => b.text).join('·'))

  // The view lags the tab by the length of its fade-out, and snaps to it when the task changes:
  // a new selection is already fading the whole pane, so the view must not fade a second time.
  $effect(() => {
    const id = detail?.task.id ?? ''
    const next = tab
    untrack(() => {
      if (id !== shownId) {
        shownId = id
        viewDelay = 0.4
        viewTab = next
        return
      }
      if (next === viewTab || !viewEl) return
      fadeOut(viewEl, () => {
        viewDelay = 0
        viewTab = next
        if (scrollEl) scrollEl.scrollTop = 0
      })
    })
  })
</script>

<section class="pane">
  <span class="corner tl"></span><span class="corner tr"></span><span class="corner bl"></span><span class="corner br"></span>
  <div class="pane-scroll" bind:this={scrollEl}>
    <div class="pane-inner" bind:this={innerEl}>
      {#if error}
        <div class="err">{error}</div>
      {:else if detail && task}
        {#key task.id}
          <div class="crumb" use:scramble={{ text: crumbText, chars: SCRAMBLE, duration: 0.6, speed: 0.6, delay: 0 }}>{#if task.rfc === '_'}stray{:else}RFC <b>{task.rfc}</b>{#if detail.rfcTitle}{` — ${detail.rfcTitle}`}{/if}{/if}</div>
          <h1 use:scramble={{ text: titleText, chars: 'abcdefghijklmnopqrstuvwxyz·', duration: 0.9, speed: 0.4, delay: 0.05, tweenLength: false }}>{titleText}</h1>
          <div class="headline" use:scramble={{ text: headText, chars: SCRAMBLE, duration: 0.7, speed: 0.5, delay: 0.2 }}>{#each headBits as bit, i (i)}{#if i}<span class="sep">·</span>{/if}{#if bit.status}<span class="st" class:is-me={task.owner === 'me'}>{bit.text}</span>{:else}{bit.text}{/if}{/each}</div>
          <div class="tabs" use:tabsIn>{#each TABS as k (k)}<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions --><span class="tab" class:is-active={k === tab} use:tabHover onclick={() => ontab(k)}>{k}</span>{/each}<span class="tab-ind" use:tabIndicator={tab}></span></div>
          {#key viewTab}
            <div class="view" bind:this={viewEl} use:viewIn={viewDelay}>
              {#if viewTab === 'result'}
                <ResultView {task} />
              {:else if viewTab === 'record'}
                <RecordView md={detail.record} />
              {:else}
                <LogView log={detail.log} />
              {/if}
            </div>
          {/key}
        {/key}
      {/if}
    </div>
  </div>
</section>

<style>
  .pane { position: relative; overflow: hidden; }
  .pane-scroll { height: 100%; overflow-y: auto; scrollbar-width: none; }
  .pane-scroll::-webkit-scrollbar { display: none; }
  .corner { position: absolute; width: 14px; height: 14px; border-color: var(--dim); border-style: solid; border-width: 0; pointer-events: none; }
  .corner.tl { top: 18px; left: 18px; border-top-width: 1px; border-left-width: 1px; }
  .corner.tr { top: 18px; right: 18px; border-top-width: 1px; border-right-width: 1px; }
  .corner.bl { bottom: 18px; left: 18px; border-bottom-width: 1px; border-left-width: 1px; }
  .corner.br { bottom: 18px; right: 18px; border-bottom-width: 1px; border-right-width: 1px; }
  .pane-inner { max-width: 760px; padding: 44px 64px 120px 56px; }

  .crumb { font-size: 11px; letter-spacing: 0.18em; text-transform: uppercase; color: var(--muted); margin-bottom: 18px; min-height: 1.5em; }
  .crumb b { font-weight: 400; color: var(--ink-2); }
  .pane h1 { font-family: var(--serif); font-weight: 400; font-size: 40px; line-height: 1.08; margin: 0 0 16px; letter-spacing: -0.005em; }
  .pane h1 :global(em) { font-style: italic; color: var(--accent-2); }
  .headline { font-size: 12px; color: var(--muted); margin-bottom: 30px; min-height: 1.5em; }
  .headline .st { color: var(--ink-2); }
  .headline .st.is-me { color: var(--accent); }
  .headline .sep { color: var(--dim); margin: 0 8px; }

  .tabs { position: relative; display: flex; gap: 28px; border-bottom: 1px solid var(--line); margin-bottom: 36px; }
  .tab { padding: 0 0 10px; font-size: 11px; letter-spacing: 0.22em; text-transform: uppercase; color: var(--muted); }
  .tab.is-active { color: var(--ink); }
  .tab-ind { position: absolute; left: 0; bottom: -1px; height: 1px; width: 0; background: var(--accent); }

  .view { will-change: transform, opacity; }
</style>
