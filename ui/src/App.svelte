<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { gsap } from 'gsap'
  import { OWNERS, fetchBacklog, fetchDetail, type Detail, type Tab, type Task } from './lib/api'
  import { SCRAMBLE, countsFlash } from './lib/fx'
  import type { mountField } from './field'
  import { orderTree } from './lib/tree'
  import Backlog from './components/Backlog.svelte'
  import Cursor from './components/Cursor.svelte'
  import Field from './components/Field.svelte'
  import Pane from './components/Pane.svelte'
  import Top from './components/Top.svelte'
  import Veil from './components/Veil.svelte'

  let tasks = $state<Task[]>([])
  let selected = $state<string | null>(null)
  let tab = $state<Tab>('result')
  let finishedOpen = $state(false)
  let current = $state<Detail | null>(null)
  let paneError = $state<string | null>(null)
  let bootError = $state<string | null>(null)
  let booted = $state(false)
  let clock = $state('')
  let rev = $state(0)

  let lastBacklog = ''
  let field: ReturnType<typeof mountField> | null = null
  let backlogEl = $state<HTMLElement>()
  let paneInnerEl = $state<HTMLElement>()
  let paneScrollEl = $state<HTMLElement>()
  const timers: ReturnType<typeof setInterval>[] = []

  const rows = $derived(orderTree(tasks))
  const count = $derived((owner: Task['owner']) => tasks.filter((t) => t.owner === owner).length)
  const running = $derived(tasks.filter((t) => t.shown === 'running').length)
  // The keys walk what the eye walks: group order, row order, and nothing inside a collapsed group.
  const visible = $derived(
    OWNERS.filter((owner) => owner !== 'finished' || finishedOpen)
      .flatMap((owner) => rows.filter((t) => t.owner === owner).map((t) => t.id)),
  )

  async function select(id: string) {
    selected = id
    field?.pulse()
    const [d] = await Promise.all([
      fetchDetail(id).then((detail) => ({ ok: true as const, detail })).catch((e) => ({ ok: false as const, error: String(e) })),
      gsap.to(paneInnerEl!, { opacity: 0, y: -10, filter: 'blur(4px)', duration: 0.22, ease: 'power2.in' }).then(),
    ])
    if (id !== selected) return
    if (!d.ok) {
      paneError = d.error
      await tick()
      gsap.to(paneInnerEl!, { opacity: 1, y: 0, filter: 'blur(0px)', duration: 0.3 })
      return
    }
    paneError = null
    current = d.detail
    tab = d.detail.task.shown === 'failed' || d.detail.task.shown === 'running' ? 'log' : 'result'
    await tick()
    if (paneScrollEl) paneScrollEl.scrollTop = 0
    gsap.set(paneInnerEl!, { opacity: 1, y: 0, filter: 'blur(0px)' })
  }

  function switchTab(next: Tab) {
    if (!current || next === tab) return
    tab = next
  }

  async function refresh(first = false) {
    const next = await fetchBacklog()
    const key = JSON.stringify(next)
    if (key === lastBacklog) return
    lastBacklog = key
    tasks = next
    rev += 1
    if (first) return
    await tick()
    if (backlogEl) countsFlash(backlogEl.querySelectorAll('.group-h .count'))
    if (selected) {
      const t = next.find((x) => x.id === selected)
      if (t && current && t.shown !== current.task.shown) select(t.id)
    }
  }

  function tickClock() {
    clock = new Date().toTimeString().slice(0, 8)
  }

  function onkeydown(e: KeyboardEvent) {
    if (!booted) return
    const i = visible.indexOf(selected ?? '')
    if (e.key === 'j' || e.key === 'ArrowDown') { const id = visible[Math.min(visible.length - 1, i + 1)]; if (id && id !== selected) select(id) }
    if (e.key === 'k' || e.key === 'ArrowUp') { const id = visible[Math.max(0, i - 1)]; if (id && id !== selected) select(id) }
    if (e.key === '1') switchTab('result')
    if (e.key === '2') switchTab('record')
    if (e.key === '3') switchTab('log')
  }

  async function boot() {
    await document.fonts.ready
    await refresh(true)
    await tick()
    const first = rows.find((t) => t.owner === 'me') ?? rows[0]

    const tl = gsap.timeline({ defaults: { ease: 'expo.out' } })
    tl.to('#veil-word', { duration: 1.3, scrambleText: { text: 'ekkyklema', chars: 'εκκυκλημα·', speed: 0.3, tweenLength: false } })
    tl.to('#veil-sub', { duration: 0.9, scrambleText: { text: 'the machine works offstage', chars: SCRAMBLE, speed: 0.5 } }, 0.5)
    tl.call(() => field?.wake(), [], 1.0)
    tl.to('#veil', { clipPath: 'inset(0 0 100% 0)', duration: 1.0, ease: 'expo.inOut' }, 1.9)
    tl.from('#wm', { y: 10, opacity: 0, duration: 0.8 }, 2.2)
    tl.from('#astrolabe', { scale: 0, rotate: -90, opacity: 0, duration: 1.2 }, 2.3)
    tl.from('#clock', { opacity: 0, duration: 0.6 }, 2.6)
    tl.from('#divider', { scaleY: 0, duration: 1.1, ease: 'expo.inOut' }, 2.2)
    tl.from('.corner', { opacity: 0, scale: 0.4, stagger: 0.06, duration: 0.6 }, 2.6)
    backlogEl?.querySelectorAll<HTMLElement>('.group-h .gname').forEach((g, i) => {
      tl.to(g, { duration: 0.6, scrambleText: { text: g.dataset.text ?? '', chars: SCRAMBLE, speed: 0.5 } }, 2.4 + i * 0.25)
    })
    tl.from('.group-h .count, .group-none, .finished-toggle', { opacity: 0, duration: 0.5, stagger: 0.05 }, 2.6)
    tl.from('.rfc-h', { opacity: 0, duration: 0.5, stagger: 0.08 }, 2.5)
    tl.from('.rfc-h .rule', { scaleX: 0, duration: 0.9, stagger: 0.08, ease: 'expo.inOut' }, 2.5)
    tl.from('.row', { x: -18, opacity: 0, filter: 'blur(6px)', duration: 0.7, stagger: 0.045 }, 2.55)
    tl.call(() => { if (first) select(first.id) }, [], 2.9)
    tl.call(() => { booted = true }, [], 3.4)

    gsap.to('#astrolabe', { rotate: 360, duration: 140, repeat: -1, ease: 'none' })
    tickClock()
    timers.push(
      setInterval(tickClock, 1000),
      setInterval(() => refresh().catch(() => {}), 4000),
    )
  }

  onMount(() => {
    if (field && backlogEl && paneInnerEl) field.quietUnder([backlogEl, paneInnerEl])
    boot().catch((e) => { bootError = String(e) })
    return () => timers.forEach(clearInterval)
  })
</script>

<svelte:window on:keydown={onkeydown} />

<Field onready={(f) => { field = f }} />
<div class="grain" aria-hidden="true"></div>
<Cursor />

<Veil error={bootError} />

<Top me={count('me')} machine={count('machine')} finished={count('finished')} {running} {rev} {booted} {clock} />

<main class="stage">
  <Backlog
    {rows}
    {selected}
    {finishedOpen}
    onselect={select}
    ontogglefinished={() => { finishedOpen = !finishedOpen }}
    bind:el={backlogEl}
  />
  <div class="divider" id="divider"></div>
  <Pane detail={current} {tab} ontab={switchTab} error={paneError} bind:innerEl={paneInnerEl} bind:scrollEl={paneScrollEl} />
</main>

<style>
  .stage {
    position: relative; z-index: 1;
    display: grid; grid-template-columns: var(--backlog-w) 1px 1fr;
    height: calc(100vh - var(--top-h));
  }
  .divider { background: var(--line); transform-origin: top; }
</style>
