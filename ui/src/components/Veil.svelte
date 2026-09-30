<script lang="ts">
  let { error = null }: { error?: string | null } = $props()
</script>

<div class="veil" id="veil">
  {#if error}
    <div class="err">{error}</div>
  {:else}
    <div class="veil-word" id="veil-word"></div>
    <div class="veil-sub" id="veil-sub"></div>
  {/if}
</div>

<style>
  .veil {
    position: fixed; inset: 0; z-index: 50; background: var(--bg);
    display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 14px;
  }
  .veil-word { font-family: var(--serif); font-style: italic; font-size: 96px; line-height: 1; letter-spacing: -0.01em; min-height: 1em; }
  .veil-sub { font-size: 12px; letter-spacing: 0.24em; text-transform: uppercase; color: var(--muted); min-height: 1.5em; }
</style>
