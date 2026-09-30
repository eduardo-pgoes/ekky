<script lang="ts">
  import { SCRAMBLE, scramble } from '../lib/fx'

  let { me, machine, finished, running, rev, booted, clock }: {
    me: number
    machine: number
    finished: number
    running: number
    rev: number
    booted: boolean
    clock: string
  } = $props()

  const runningBit = $derived(running ? `${running} running, ` : '')
  const line = $derived(`${me} waiting on you · ${runningBit}${machine} with the machine · ${finished} finished`)
</script>

<header class="top" id="top">
  <div class="wordmark"><span class="wm-serif" id="wm">ekkyklema</span><span class="wm-sub" id="wm-sub"></span></div>
  {#if rev}
    {#key rev}
      <div class="top-mid" id="top-mid" use:scramble={{ text: line, chars: SCRAMBLE, duration: booted ? 0.6 : 0.8, speed: 0.6, delay: booted ? 0 : 2.5 }}><b>{me}</b> waiting on you <em>·</em> <b>{runningBit}{machine}</b> with the machine <em>·</em> <b>{finished}</b> finished</div>
    {/key}
  {:else}
    <div class="top-mid" id="top-mid"></div>
  {/if}
  <div class="top-right">
    <span class="clock" id="clock">{clock}</span>
    <svg class="astrolabe" id="astrolabe" viewBox="0 0 100 100" width="44" height="44" aria-hidden="true">
      <g fill="none" stroke="currentColor" stroke-width="0.8">
        <circle cx="50" cy="50" r="46" />
        <circle cx="50" cy="50" r="30" />
        <circle cx="50" cy="50" r="12" />
        <path d="M50 4 V96 M4 50 H96 M17.5 17.5 L82.5 82.5 M82.5 17.5 L17.5 82.5" />
        <circle cx="50" cy="20" r="2.4" fill="currentColor" stroke="none" />
        <circle cx="80" cy="50" r="1.6" fill="currentColor" stroke="none" />
      </g>
    </svg>
  </div>
</header>

<style>
  .top {
    position: relative; z-index: 2; height: var(--top-h); padding: 0 32px;
    display: grid; grid-template-columns: var(--backlog-w) 1fr auto; align-items: center;
    border-bottom: 1px solid var(--line);
  }
  .wordmark { display: flex; align-items: baseline; gap: 18px; }
  .wm-serif { font-family: var(--serif); font-style: italic; font-size: 30px; line-height: 1; }
  .wm-sub { font-size: 11px; letter-spacing: 0.22em; text-transform: uppercase; color: var(--muted); }
  .top-mid { font-size: 11px; letter-spacing: 0.18em; text-transform: uppercase; color: var(--muted); padding-left: 33px; }
  .top-mid b { color: var(--ink-2); font-weight: 400; }
  .top-mid em { color: var(--accent); font-style: normal; }
  .top-right { display: flex; align-items: center; gap: 22px; }
  .clock { font-size: 11px; letter-spacing: 0.12em; color: var(--muted); font-variant-numeric: tabular-nums; }
  .astrolabe { color: var(--muted); }
</style>
