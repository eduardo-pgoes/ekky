// A glyph field: a slow noise flow rendered as sparse monospace marks behind the UI.
// Driven by gsap.ticker so it shares a clock with every other motion on the page.
import { gsap } from 'gsap'

const GLYPHS = ['·', '·', '·', '∙', ':', '⁘', '⁙', '+', '×', '┼']

function hash(x: number, y: number): number {
  let h = Math.imul(x | 0, 374761393) + Math.imul(y | 0, 668265263)
  h = Math.imul(h ^ (h >>> 13), 1274126177)
  return ((h ^ (h >>> 16)) >>> 0) / 4294967295
}
const smooth = (t: number) => t * t * (3 - 2 * t)
const lerp = (a: number, b: number, t: number) => a + (b - a) * t
function noise(x: number, y: number): number {
  const xi = Math.floor(x), yi = Math.floor(y)
  const u = smooth(x - xi), v = smooth(y - yi)
  return lerp(lerp(hash(xi, yi), hash(xi + 1, yi), u), lerp(hash(xi, yi + 1), hash(xi + 1, yi + 1), u), v)
}

export function mountField(canvas: HTMLCanvasElement) {
  const ctx = canvas.getContext('2d')!
  const cell = 18
  let w = 0, h = 0, dpr = 1
  const mouse = { x: -9999, y: -9999 }
  const state = { intensity: 0, pulse: 0 } // tweened from outside
  // Rects the field goes quiet under: the columns that carry text. Read each frame; two calls, cheap.
  let quiet: (() => DOMRect)[] = []

  function resize() {
    dpr = Math.min(window.devicePixelRatio || 1, 2)
    w = window.innerWidth; h = window.innerHeight
    canvas.width = w * dpr; canvas.height = h * dpr
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
    ctx.font = `12px "IBM Plex Mono", ui-monospace, monospace`
    ctx.textBaseline = 'middle'
    ctx.textAlign = 'center'
  }
  resize()
  window.addEventListener('resize', resize)
  window.addEventListener('pointermove', (e) => { mouse.x = e.clientX; mouse.y = e.clientY })

  function draw(time: number) {
    const t = time * 0.12
    ctx.clearRect(0, 0, w, h)
    const cols = Math.ceil(w / cell), rows = Math.ceil(h / cell)
    const qr = quiet.map((f) => f())
    for (let j = 0; j < rows; j++) {
      for (let i = 0; i < cols; i++) {
        const x = i * cell + cell / 2, y = j * cell + cell / 2
        const n1 = noise(i * 0.055 + t * 0.35, j * 0.075 - t * 0.2)
        const n2 = noise(i * 0.2 - t * 0.6, j * 0.2 + t * 0.4)
        let n = n1 * 0.72 + n2 * 0.28
        const dx = x - mouse.x, dy = y - mouse.y
        const d = Math.sqrt(dx * dx + dy * dy)
        let near = Math.max(0, 1 - d / 220)
        let under = false
        for (const r of qr) if (x >= r.left && x <= r.right && y >= r.top && y <= r.bottom) { under = true; break }
        if (under) near *= 0.25
        n += near * near * 0.35 + state.pulse * 0.25 * Math.max(0, 1 - Math.abs(d - state.pulse * 900) / 120)
        const thr = 0.6
        if (n < thr) continue
        let a = Math.min(1, (n - thr) * 2.6) * (0.36 + 0.6 * state.intensity)
        if (under) a *= 0.16
        const g = GLYPHS[Math.min(GLYPHS.length - 1, Math.floor((n - thr) * 22))]
        ctx.fillStyle = near > 0.15 ? `rgba(227,154,59,${a})` : `rgba(232,226,212,${a * 0.55})`
        ctx.fillText(g, x, y)
      }
    }
  }

  gsap.ticker.add((time) => draw(time))
  return {
    state,
    quietUnder(els: HTMLElement[]) { quiet = els.map((el) => () => el.getBoundingClientRect()) },
    wake() { gsap.to(state, { intensity: 1, duration: 2.4, ease: 'power2.out' }) },
    pulse() { gsap.fromTo(state, { pulse: 0 }, { pulse: 1, duration: 1.4, ease: 'power1.out' }) },
  }
}
