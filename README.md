# ekky

A personal AI task daemon. *Ekkyklema*: the wheeled platform Greek theatre used to roll offstage action into view. The machine works offstage; `promote` is the roll-out.

Generation is not the bottleneck; attention is. `ekky` takes a backlog of task records, runs a headless [Claude Code](https://docs.claude.com/en/docs/claude-code) agent on each one in its own git worktree, and leaves a local branch plus a result card behind. Nothing pings you, nothing waits on you, and nothing leaves the machine until you say so. The target felt state is "I have five PRs to review."

> A showcase of a tool I built for my own workflow, not a supported product. It assumes Linux, systemd, the Claude Code CLI and a particular way of working. The full design is in [`docs/design.md`](docs/design.md).

## The loop

```
decided direction ──ekky:cut──▶ ## Decisions + ## Tasks ──ekky:decompose──▶ task records
                                                                              │
                                                                       ekky tackle
                                                                              ▼
       ekky:promote ◀── accept ◀──ekky:review◀── result card + ekky/<id> branch ◀── drain (N slots, one worktree each)
            │                          │
   push, PRs, tickets         rework note ──▶ fresh attempt
```

- **In-session verbs** (`skills/`, a Claude Code plugin): `cut` turns a decided RFC or design doc into a decisions section and one-PR tasks with `after:` dependencies; `decompose` writes the task records; `load` makes one from a stray ticket or conversation; `review` reads the result card and diff, then accepts, sends back or answers a parked question; `promote` renames and pushes branches, opens the PRs and files the tickets.
- **The dispatcher** (`cmd/`, `internal/`): Go, standard library only, one static binary. `tackle` fetches and queues, `drain` runs every ready task `SLOTS` at a time, and `backlog` and `show` read the state. `serve` hosts the web view.
- **The web view** (`ui/`): Svelte 5, read-only, loopback only. The backlog grouped by who each task waits on, plus result, record and live log panes.

## Design choices worth reading

- **Files are the database.** State is a plain directory: one `task.md` per task (human-edited only), a one-word `status` file written `tmp + mv`, and `result.json`, `run.log` and `events.jsonl`. Every verb rebuilds its view from disk on every call. `internal/model` reads the tree and never writes, and everything else is a function over what it returns.
- **The agent owns nothing but commits.** It returns one JSON object validated against [`agent/result.schema.json`](agent/result.schema.json). The dispatcher interprets only `status` and owns every state transition.
- **Forks over questions.** The agent doesn't stop to ask. It picks a branch, commits to it, and records what it rejected in a decisions ledger. `needs-input` is reserved for an ambiguity that would invalidate more than half the work.
- **No resume, ever.** Every attempt is a cold run over the whole record. Rework, answers and the previous failure's log tail get appended to `task.md`, so the record is the entire context.
- **Fenced by construction.** No MCP servers, no user settings or hooks, no `gh` credentials, a `pushurl` of `/nonexistent` injected through `GIT_CONFIG_*`, a pre-push hook, a dollar budget, and a wall-clock timeout that kills the whole process group. The drain does no network I/O. `tackle` fetches in your session, where your SSH key lives.
- **Dependencies are chains, not DAGs.** `after:` names one sibling. A dependent is cut from its upstream's branch, and reworking an upstream re-runs the chain cold from the new tip instead of rebasing.
- **Crash-honest.** A single-instance lock file, an orphan sweep on startup (a leftover `running` lands as `failed`), and SIGTERM propagated to every live attempt's process group.

## Build

```sh
go build -o bin/ekky ./cmd/ekky      # Go 1.26+, no dependencies
go test ./...                        # drives a fake agent through real git repos and worktrees
cd ui && pnpm install && pnpm build  # the web view; `ekky serve` serves ui/dist
```

Installing it as a live Claude Code plugin with systemd units is covered in [`docs/design.md`](docs/design.md#install).

## Layout

```
cmd/ekky/         CLI: tackle | drain | backlog | show | cancel | serve
internal/         model (read-only view of state), attempt (one agent run), sched (drain, lock, cancel),
                  render, serve, store, harness (test fixtures and a fake agent)
agent/            the system prompt and result schema the headless agent runs with
skills/           the in-session verbs, one SKILL.md each
ui/               the web view
systemd/          user units for the drain and the web view
```

Copyright © Eduardo Goes. All rights reserved. The source is published for reading, and no license is granted.
