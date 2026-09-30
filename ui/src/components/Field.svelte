<script lang="ts">
  import { onMount } from 'svelte'
  import { mountField } from '../field'

  let { onready }: { onready: (field: ReturnType<typeof mountField>) => void } = $props()

  let el: HTMLCanvasElement
  onMount(() => onready(mountField(el)))
</script>

<canvas id="field" bind:this={el}></canvas>
