<script lang="ts">
  import { onMount } from 'svelte'
  import { gsap } from 'gsap'
  import { registerCursor } from '../lib/fx'

  let el: HTMLElement
  onMount(() => {
    const unregister = registerCursor(el)
    const qx = gsap.quickTo(el, 'x', { duration: 0.18, ease: 'power3.out' })
    const qy = gsap.quickTo(el, 'y', { duration: 0.18, ease: 'power3.out' })
    const move = (e: PointerEvent) => { qx(e.clientX); qy(e.clientY) }
    const down = () => gsap.fromTo(el, { scale: 0.6 }, { scale: 1, duration: 0.4, ease: 'expo.out' })
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerdown', down)
    return () => {
      window.removeEventListener('pointermove', move)
      window.removeEventListener('pointerdown', down)
      unregister()
    }
  })
</script>

<div class="cursor" aria-hidden="true" bind:this={el}><i></i><i></i></div>
