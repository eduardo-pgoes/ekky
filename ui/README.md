# ekky ui

A read-only view over `agent-tasks/`. It renders what `ekky backlog --json` and `ekky show --json` return, plus `task.md`, `run.log` and the RFC title, and writes nothing.

The server is the dispatcher: `ekky serve`, kept on by `ekky-ui.service`, owns `/api/backlog` and `/api/task/<rfc>/<slug>` (the model in-process, no subprocess) and serves `dist/` off disk at `http://127.0.0.1:5178`, loopback only. `pnpm build` is the deploy; nothing at runtime is node.

```sh
cd ~/ekky/ui
pnpm install
pnpm build          # writes dist/, which ekky serve serves — the deploy
pnpm dev            # http://localhost:5177, hot reload; /api proxied to ekky serve on 5178
pnpm check          # svelte-check
```

Vite + Svelte 5 (runes) + TypeScript + GSAP (SplitText, ScrambleText). `pnpm check` runs `svelte-check` and is the type gate. `vite.config.ts` carries no API of its own — dev proxies `/api` to the running service, so the endpoints have one implementation, in Go. The backlog is re-read every 4s; a `running` row shows the last line of its `run.log`, re-read every 1.5s.

Keys: `j`/`k` move the selection, `1`/`2`/`3` switch result / record / log.
