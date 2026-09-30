// Every GSAP tween the components raise lives here, as an action that owns its tween and
// kills it on destroy. Nothing else in the app touches gsap except App.svelte's boot timeline.
import { gsap } from 'gsap'
import { ScrambleTextPlugin } from 'gsap/ScrambleTextPlugin'
import { SplitText } from 'gsap/SplitText'
import { fetchDetail } from './api'

gsap.registerPlugin(SplitText, ScrambleTextPlugin)

export const SCRAMBLE = '·∙:+×┼01'
const LIVE_POLL_MS = 1500

let cursorEl: HTMLElement | null = null

export function registerCursor(el: HTMLElement) {
  cursorEl = el
  return () => { if (cursorEl === el) cursorEl = null }
}

function cursorHot(vars: gsap.TweenVars) {
  if (!cursorEl) return
  cursorEl.classList.add('is-hot')
  gsap.to(cursorEl, vars)
}

function cursorCool(vars: gsap.TweenVars) {
  if (!cursorEl) return
  cursorEl.classList.remove('is-hot')
  gsap.to(cursorEl, vars)
}

export function rowHover(node: HTMLElement, isSelected: () => boolean) {
  const rule = node.querySelector<HTMLElement>('.hover-rule')!
  const title = node.querySelector<HTMLElement>('.title')!
  const enter = () => {
    cursorHot({ scale: 1.7, rotate: 45, duration: 0.25, ease: 'expo.out' })
    gsap.to(rule, { scaleX: 1, duration: 0.5, ease: 'expo.out' })
    gsap.to(title, { x: 6, color: '#e8e2d4', duration: 0.3, ease: 'expo.out' })
  }
  const leave = () => {
    cursorCool({ scale: 1, rotate: 0, duration: 0.35, ease: 'expo.out' })
    gsap.to(rule, { scaleX: 0, duration: 0.4, ease: 'expo.inOut' })
    gsap.to(title, { x: 0, color: isSelected() ? '#e8e2d4' : '#b9b2a3', duration: 0.3 })
  }
  node.addEventListener('mouseenter', enter)
  node.addEventListener('mouseleave', leave)
  return {
    destroy() {
      node.removeEventListener('mouseenter', enter)
      node.removeEventListener('mouseleave', leave)
      gsap.killTweensOf([rule, title])
    },
  }
}

export function tabHover(node: HTMLElement) {
  const enter = () => cursorHot({ scale: 1.4, duration: 0.25 })
  const leave = () => cursorCool({ scale: 1, duration: 0.25 })
  node.addEventListener('mouseenter', enter)
  node.addEventListener('mouseleave', leave)
  return {
    destroy() {
      node.removeEventListener('mouseenter', enter)
      node.removeEventListener('mouseleave', leave)
    },
  }
}

// The marker rides the selected row. It reads geometry from the row it finds, so it has to
// re-run when the row list changes and not only when the selection does.
export function marker(node: HTMLElement, at: { selected: string | null; rows: unknown }) {
  const move = (selected: string | null) => {
    const row = node.parentElement?.querySelector<HTMLElement>(`.row[data-id="${CSS.escape(selected ?? '')}"]`)
    if (!row) return gsap.to(node, { opacity: 0, duration: 0.2 })
    gsap.to(node, { top: row.offsetTop, height: row.offsetHeight, opacity: 1, duration: 0.55, ease: 'expo.out' })
  }
  move(at.selected)
  return {
    update(next: { selected: string | null; rows: unknown }) { move(next.selected) },
    destroy() { gsap.killTweensOf(node) },
  }
}

// A running row shows the last line of its own run.log. The interval belongs to the row, so
// it stops when the row stops running or leaves the backlog, and a refresh that keeps the row
// keeps the tween running through it.
export function liveLine(node: HTMLElement, id: string) {
  let last = ''
  const tick = async () => {
    try {
      const d = await fetchDetail(id)
      const lines = d.log.split('\n').filter((l) => l.trim() && !l.startsWith('ekky:'))
      const text = (lines.at(-1) ?? '').replace(/^\d\d:\d\d:\d\d\s+/, '')
      if (text && text !== last) {
        last = text
        gsap.to(node, { duration: 0.5, scrambleText: { text, chars: SCRAMBLE, speed: 0.7 } })
      }
    } catch { /* the row will go away on the next refresh */ }
  }
  const timer = setInterval(tick, LIVE_POLL_MS)
  return {
    destroy() {
      clearInterval(timer)
      gsap.killTweensOf(node)
    },
  }
}

export function finishedIn(rows: ArrayLike<Element>) {
  gsap.from(rows, { x: -14, opacity: 0, filter: 'blur(4px)', stagger: 0.04, duration: 0.5, ease: 'expo.out' })
}

export function countsFlash(counts: ArrayLike<Element>) {
  gsap.fromTo(counts, { color: '#e39a3b' }, { color: '', duration: 1.2, ease: 'power2.out' })
}

// ScrambleText owns the node for the length of the tween: it blanks the text, tweens plain
// characters into place and hands the original markup back on completion. Destroy kills the
// tween, so a `{#key}` remount mid-intro drops it instead of racing the new one.
export type ScrambleOpts = { text: string; chars: string; duration: number; speed: number; delay: number; tweenLength?: boolean }

export function scramble(node: HTMLElement, o: ScrambleOpts) {
  const markup = node.innerHTML
  node.textContent = ''
  const tween = gsap.to(node, {
    duration: o.duration,
    delay: o.delay,
    scrambleText: { text: o.text, chars: o.chars, speed: o.speed, tweenLength: o.tweenLength ?? true },
    onComplete: () => { node.innerHTML = markup },
  })
  return { destroy() { tween.kill() } }
}

export function tabsIn(node: HTMLElement) {
  const tween = gsap.from(node.querySelectorAll('.tab'), { y: 8, opacity: 0, stagger: 0.06, duration: 0.5, ease: 'expo.out', delay: 0.3 })
  return { destroy() { tween.kill() } }
}

export function tabIndicator(node: HTMLElement, _tab: unknown) {
  const place = (animate: boolean) => {
    const active = node.parentElement?.querySelector<HTMLElement>('.tab.is-active')
    if (!active) return
    gsap.to(node, { x: active.offsetLeft, width: active.offsetWidth, duration: animate ? 0.45 : 0, ease: 'expo.out' })
  }
  place(false)
  return {
    update() { place(true) },
    destroy() { gsap.killTweensOf(node) },
  }
}

export function fadeOut(node: HTMLElement, done: () => void) {
  gsap.to(node, { opacity: 0, y: -8, duration: 0.18, ease: 'power2.in', onComplete: done })
}

// The view's own entrance: blocks rise, forks slide in behind them, and prose marked `.split`
// comes in line by line. SplitText rewrites the paragraph it splits, so every split reverts —
// on completion in the normal case, on destroy when the view is swapped out under it.
export function viewIn(node: HTMLElement, delay: number) {
  const tl = gsap.timeline({ delay })
  const blocks = node.querySelectorAll('.block, .fm, .rec > *, pre.log')
  tl.fromTo(blocks, { y: 18, opacity: 0, filter: 'blur(6px)' }, { y: 0, opacity: 1, filter: 'blur(0px)', duration: 0.7, stagger: 0.06, ease: 'expo.out' }, 0)
  const forks = node.querySelectorAll('.fork')
  if (forks.length) tl.fromTo(forks, { x: -10, opacity: 0 }, { x: 0, opacity: 1, duration: 0.6, stagger: 0.05, ease: 'expo.out' }, 0.15)
  const splits: SplitText[] = []
  node.querySelectorAll<HTMLElement>('.split').forEach((el) => {
    const split = SplitText.create(el.matches('p') ? el : el.querySelectorAll('p'), { type: 'lines' })
    splits.push(split)
    tl.from(split.lines, { y: 12, opacity: 0, duration: 0.6, stagger: 0.05, ease: 'expo.out', onComplete: () => split.revert() }, 0.1)
  })
  return {
    destroy() {
      tl.kill()
      splits.forEach((s) => s.revert())
    },
  }
}
