# ekky — design

A private AI task daemon. *Ekkyklema*: the wheeled platform Greek theatre used to roll offstage action into view. The machine works offstage; `promote` is the roll-out.

Generation is not the bottleneck; attention is. `ekky` takes a backlog of task records, runs a headless agent on each one in its own worktree, and leaves a local branch plus a result card behind. Nothing pings you, nothing waits on you, and colleagues never see unclaimed output: no pushed branches, no draft PRs, no Linear writes until you `promote`. The target felt state is "I have five PRs to review."

## Shape

One upstream document (an internal RFC, or any decided direction), four in-session verbs, one later verb, and a small Go dispatcher in the middle.

| verb | where | what |
|---|---|---|
| `ekky:cut` | session | a decided direction → the `## Decisions` and `## Tasks` sections appended to it, with you present |
| `ekky:decompose` | session | a document carrying those two sections → one task record per entry in its `## Tasks` |
| `ekky:load` | session | a stray task (spec, ticket, vault note, conversation) → one task record, thirty seconds of your attention |
| `ekky tackle <id>` | shell | mark a task `ready` and start the drain |
| `ekky drain` | shell / systemd | run every `ready` task, `SLOTS` at a time, until none is left |
| `ekky backlog` | shell | the table, grouped by who each task waits on: you, the machine, nobody |
| `ekky show <id>` | shell | one task: record, status, result card, tail of the log |
| `ekky cancel <id>` | shell | stop a running attempt; it lands `failed` / cancelled |
| `ekky serve` | systemd | the web view: backlog and task panes at `http://127.0.0.1:5178`, loopback only |
| `ekky:review` | session | next `done` task → result card → diff → accept or `rework`; also answers `needs-input` |
| `ekky:promote` | session | per RFC batch: create tickets, rename and push branches, open PRs, publish the RFC; or one task at a time against an epic's existing tickets: review, match the ticket, rename, push, open the PR |

The dispatcher owns all state. The agent owns nothing but commits on a local branch: it returns structured JSON and the dispatcher writes `result.json`, renders `result.md`, and flips `status`. `task.md` is human-edited only; the daemon never rewrites it. Every verb rebuilds its view of the world from the files on each call; nothing holds state in memory, and `--json` on the read verbs is that view for a UI.

## Layout

```
~/ekky/                     # also the plugin root: ~/.claude/skills/ekky → here
  .claude-plugin/plugin.json
  bin/ekky                  # the built dispatcher: tackle | drain | backlog | show | cancel — gitignored
  cmd/ekky/  internal/      # its source: a read-only model of agent-tasks/, the attempt runner, the scheduler, the harness
  agent/                    # execute.md  result.schema.json — what runs in the worktree
  skills/                   # cut/ decompose/ load/ review/ promote/ — each <verb>/SKILL.md → ekky:<verb>
  ui/                       # the web view: svelte source; `pnpm build` writes dist/, `ekky serve` serves it
  systemd/                  # ekky-drain.service  ekky-ui.service → symlinked into ~/.config/systemd/user/
  llms/                     # specs and handoffs — gitignored, local working notes
  agent-tasks/              # state — gitignored, plain directory, never a git repo
    .drain.lock             # held by the drain for its whole life
    <rfc-id>/
      rfc.md
      promote.md            # promote's ledger, once it has run
      tasks/<nn-slug>/
        task.md             # the record — see below
        status              # one word
        result.json         # the agent's final result event, verbatim
        result.md           # rendered from result.json for reading
        run.log             # the attempt as it happened: the dispatcher's lines, the agent's tool calls, its stderr
        events.jsonl        # the agent's stream-json events, verbatim, one per line
        pgid                # the attempt's process group; present only while it is live
        cancel              # marker written by `ekky cancel`, consumed by the drain
        spec.md             # the spec the agent wrote for itself before executing
        wt/                 # worktree; removed on done, kept on failed / needs-input
    _/tasks/<nn-slug>/      # the null RFC: where `load` puts stray tasks
```

`skills/` runs in your session, `agent/` runs in the worktree, `bin/` dispatches between them. Code is under git; `agent-tasks/` is state and stays out. The dispatcher is Go 1.26, standard library only, one static binary; `internal/model` reads the tree and never writes, and everything else is a function over what it returns.

A task id is `<rfc-id>/<nn-slug>`; the null RFC is `_`, so a stray task is `_/03-fix-cors`. The branch the agent commits to is `ekky/<rfc-id>-<nn-slug>` in the task's repo; the `ekky/` prefix never reaches a remote, `promote` renames it.

## Install

```sh
git clone <this repo> ~/ekky
go build -o ~/ekky/bin/ekky ./cmd/ekky   # from ~/ekky; Go 1.26, no dependencies; rebuild after a pull
ln -s ~/ekky ~/.claude/skills/ekky      # auto-discovered as ekky@skills-dir; edits are live next session
ln -s ~/ekky ~/.claude-work/skills/ekky  # once per extra login: the skills dir is per CLAUDE_CONFIG_DIR
claude plugin details ekky              # should report Source: ekky@skills-dir, under each CLAUDE_CONFIG_DIR
ln -s ~/ekky/bin/ekky ~/.local/bin/ekky
cd ~/ekky/ui && pnpm install && pnpm build   # writes ui/dist, which `ekky serve` serves off disk
systemctl --user link ~/ekky/systemd/ekky-drain.service ~/ekky/systemd/ekky-ui.service && systemctl --user daemon-reload
systemctl --user enable --now ekky-ui.service   # the web view, on from here without a terminal
```

The plugin is loaded live from the working tree on purpose. Marketplace installs snapshot into `~/.claude/plugins/cache` and drift from the checkout; the symlink name is the plugin name.

Root resolution: `EKKY_ROOT` if set, else `~/ekky`. Everything the dispatcher touches derives from it. The services hardcode `%h/ekky`; if you keep the checkout elsewhere, add a drop-in that sets `EKKY_ROOT` and `ExecStart`.

Then, once per repository the daemon will work in, install the push guard:

```sh
cat > .git/hooks/pre-push <<'EOF'
#!/bin/sh
if [ -n "$EKKY_TASK" ]; then
  echo "pre-push: refused — this checkout is driven by ekky task $EKKY_TASK" >&2
  exit 1
fi
EOF
chmod +x .git/hooks/pre-push
```

Worktrees share the main checkout's hooks, so one hook covers every task in that repo. `git push --no-verify` skips pre-push hooks, which is why the dispatcher also sets `remote.origin.pushurl=/nonexistent` in the agent's environment (`GIT_CONFIG_*`); that one is not skippable from the command line. The hook is the belt, the environment is the braces. The agent has no reason to reach the network at all: the drain fetches nothing, and the worktree is cut from a ref your session already fetched, or from the local base in a repo with no origin.

## Running

```sh
ekky tackle _/00-smoke          # fetch origin/<base>, mark ready, start the drain
ekky tackle <rfc-id>            # every new task in that rfc
ekky tackle all                 # every new task everywhere
ekky backlog                    # the table: me, machine, finished
ekky backlog --needs-me         # only what waits on you
ekky backlog --json             # the model, one object per task; --needs-me filters it the same way
ekky show _/00-smoke            # one task: record, status, result card, log tail; --json for the object
ekky cancel _/00-smoke          # stop a running attempt
journalctl --user -u ekky-drain -f
tail -f ~/ekky/agent-tasks/_/tasks/00-smoke/run.log
xdg-open http://127.0.0.1:5178          # the same backlog in a browser; ekky-ui.service keeps it on
```

`ekky serve` is the web view's server: `/api/backlog` and `/api/task/<id>` are the model in-process — the same objects `--json` prints — and `ui/dist` is served off disk, bound to loopback only because `run.log` and `task.md` are not for the network. `ekky-ui.service` keeps it on across boots; the vite dev server proxies `/api` to it, so the API has exactly one implementation. `pnpm build` in `ui/` is the deploy, and a running serve picks it up on the next request — no restart, nothing at runtime is node.

There is no timer. `tackle` runs in your session, where your ssh key works: before it writes anything it runs `git fetch origin <base>` once per distinct repo and base among the tasks it is about to ready. A fetch that fails is allowed only out loud: with a local `origin/<base>` it logs `using origin/<base> as of <age>` and proceeds; with none it refuses that task, and its dependents, and writes nothing for them. A repo with no `origin` remote at all is legitimately local-only: there is nothing to fetch and no staleness to warn about, so `tackle` skips the fetch out loud (`no origin in <repo>; using local <base>`) and the cut comes from the local `<base>`, which must exist or the task is refused the same way. Dependents are never fetched for; they cut from the upstream's branch. Then it writes `ready` down the cascade and starts `ekky-drain.service`, a oneshot that loops until nothing is `ready`, running up to `SLOTS` attempts at once. Without the service (or with `EKKY_NO_SYSTEMD=1`) `tackle` drains in the foreground.

The drain holds `agent-tasks/.drain.lock` for its whole life; a second drain is refused with a message, so single-instance no longer depends on systemd. Before scheduling anything it reconciles: every task still at `running` was left by a drain that died, and lands `failed` with `orphaned: no drain was running` in its log. It rescans after each attempt finishes, so anything tackled mid-drain is picked up; when a scan finds nothing to start and nothing is live it releases the lock, looks once more, and takes the lock back if a `tackle` landed in between, so a task readied during the drain's exit is never stranded. `tackle` itself never waits on the lock: if a drain holds it, that drain will see the task. On `systemctl stop` (TERM) or Ctrl-C the drain signals every live attempt's process group, waits for them, lands each one `failed` with `drain stopped` in its log, and exits.

Per attempt the dispatcher: writes `running`; truncates `run.log` and `events.jsonl`; deletes any stale `wt/`; cuts `wt/` on a fresh `ekky/<id>` branch (`-B`, so a retry or rework starts over from the base; `--no-track`, and one cut at a time, so two slots never race the repository's lock) from the local `origin/<base>` (the local `<base>` itself in a repo with no origin), or, when the record has `after:`, from the upstream's local `ekky/<rfc>-<after>`; the drain does no network I/O, ever, and a repository with an origin but no local `origin/<base>` lands `failed` with that reason. It runs the agent in its own process group with the record on stdin, `agent/execute.md` as the system prompt, `--output-format stream-json`, and `CLAUDE_CONFIG_DIR` set from `account:` (refusing up front if that directory has no `.credentials.json`); writes the group id to `pgid`; appends every event to `events.jsonl` as it arrives and renders it into `run.log` — the agent's text, each tool call as its name and the command or path it was given, the result envelope — so `tail -f run.log` shows what the agent is doing within seconds of it doing it. When the agent returns, `result.json` is its final result event verbatim, `result.md` is rendered from it, and the final status is the result's `status`. Non-zero exit, the wall clock (`EKKY_TIMEOUT`, default `90m`: TERM to the whole group, KILL ten seconds later, so the agent's own subprocesses die with it), budget (`EKKY_BUDGET`, default `15` USD), or unparseable output all land as `failed`, with the reason at the end of `run.log`. On `done` the worktree is removed; on anything else it stays for inspection. `pgid` goes whatever the outcome.

`ekky cancel <id>` stops a running attempt: it writes the `cancel` marker, signals the group from `pgid`, and the drain lands the task `failed` with `attempt cancelled` in its log, worktree kept. If no drain holds the lock, `cancel` lands it itself.

A task with `after: <nn-slug>` is a dependent. The drain skips it while its upstream's `status` is anything but `done`; it stays `ready` and `backlog` shows it as `blocked`, with the upstream's slug in the `upstream` column, so a typo'd `after:` is visible rather than blocking forever in silence. Once the upstream lands, the dependent is cut from the upstream's branch, so a chain 01 → 02 → 03 runs in order and `ekky/<rfc>-03` carries all three tasks' commits. At cut time the dispatcher checks that the upstream's `repo` matches and that its branch exists; either failing lands the attempt as `failed` with the reason in `run.log`.

Re-tackling a `failed` task appends the tail of its `run.log` to `task.md` under `## Previous attempt` first, so the retry can see what broke. Same model; escalation is a `model:` edit from `review`.

`tackle` on a task with descendants re-readies all of them, transitively, through the same path: `accepted` cleared, `ready` written, a `failed` one getting its `## Previous attempt`. A reworked upstream therefore re-runs its whole chain cold from the new tip; there is no rebase and no conflict resolution, the fresh-attempt design pays for the cascade. It refuses, before writing anything, if the task or any descendant is `running`. A tweak committed directly on an upstream's branch from `review` does not cascade: descendants keep their fork point, their PR diff stays their own commits, and the merge carries the tweak.

## The task record: `task.md`

Frontmatter is flat `key: value` lines between the first `---` and the next; the first match of a key wins. There is no `yq` on this machine and none is needed.

```markdown
---
repo: /home/you/dev/some-repo      # path to the main checkout; worktrees are cut from here
base: main                         # branch the task merges into; cut from the fetched origin/<base>, or the local <base> when the repo has no origin
model: opus                        # claude --model value; the knob you turn from review
account: work                      # which Claude login bills the run: perso → ~/.claude, <name> → ~/.claude-<name>
after: 01-add-migration            # optional: a sibling task in the same RFC; cut from its ekky/ branch once it is done
source: ENG-1234                   # optional: spec path, ticket id, vault note, or "conversation"
---
# <title, one line>

## Done looks like
<what the reviewer will see when this is finished: behaviour, not files>

## Decisions made
- <the two or three calls already taken; the agent does not reopen them>

## Do not
- <the one thing the agent must not do, if there is one>
```

Everything above the sections is written once, by `load` or `decompose`, with you present. Three sections may be appended later; the agent reads all of them on every attempt, in order, and the newest overrides the oldest:

- `## Answers` — appended by `review` when you answer a `needs-input` question. One sentence is enough.
- `## Rework` — appended by `review` when you send a `done` task back. The note says what to change; the retry starts cold with the whole record.
- `## Previous attempt` — appended by the dispatcher on retry after `failed`: the tail of `run.log`, so the agent can see what broke.

There is no `--resume`. Every attempt is a fresh run over the whole record; the record is the entire context.

Required fields are `repo`, `base`, `model`. `account` is optional: absent, the dispatcher uses whatever `CLAUDE_CONFIG_DIR` it inherited, else `~/.claude`; under systemd that means perso, so `load` and `decompose` always write it, inferring it from `agent-tasks/accounts`, a local file of `<path-prefix> <account>` lines (e.g. `~/dev/work/ work`), and `perso` when no prefix matches. `source` is free text for `backlog`. `after` is optional, names exactly one sibling (a chain or a tree, never a DAG), and is written only by `decompose` from the RFC's `` `after: N` `` token; a stray task has no siblings. `base` keeps its meaning on a dependent: it is the PR target once the upstream has merged, and the dispatcher reads `after`, not `base`, to decide where to cut from.

## Status

`status` is a one-word file, written `tmp + mv` by the dispatcher only.

| word | meaning |
|---|---|
| *(absent)* | loaded, not yet tackled; `backlog` shows it as `new` |
| `ready` | queued; the next `drain` picks it up |
| `running` | an attempt is in flight; set before the worktree is cut |
| `done` | every unit committed on `ekky/<id>`; worktree removed; waiting for `review` |
| `failed` | the run broke (timeout, budget, hook refusal, non-zero exit, unparseable output); worktree kept; `run.log` has the story |
| `needs-input` | the agent found a core ambiguity and parked; worktree kept; `result.json` carries the question |

Three more words appear in `backlog` and `show` but are never written to `status`, in this precedence: `promoted`, a task whose batch's `promote.md` carries a `branch <nn-slug>:` line (the branch column then shows the renamed branch, and the JSON carries the PR URL from the `pr` line); `accepted`, a `done` task with `<taskdir>/accepted` from `review`; `blocked`, a `ready` task whose `after:` upstream is not `done`. `ekky tackle` on a blocked task logs "already ready" and does nothing.

Every task also has an owner, which is how `backlog` groups its table and what `--needs-me` means:

| owner | shown status | meaning |
|---|---|---|
| `me` | `new`, `done` (not accepted), `needs-input`, `failed` | waiting on you: tackle, review, answer, or triage |
| `machine` | `ready`, `blocked`, `running` | the daemon's; nothing to do |
| `finished` | `accepted`, `promoted` | closed |

Transitions: `ready → running → done | failed | needs-input`. `review` and `rework` write `ready` again; a `failed` task goes back to `ready` by hand, after reading the log. `running` is written exactly once per attempt, and a `running` left behind by a drain that died is landed `failed` (`orphaned`) by the next drain before it schedules anything. The reason a `failed` task failed is the last `ekky:` line but one of its `run.log`: a precondition, the exit code, `deadline`, `cancelled`, `drain stopped`, `orphaned`, or `no parseable result`.

## The result contract: `result.json`

The agent's only output is one JSON object validated against `agent/result.schema.json`:

```json
{ "status": "done | failed | needs-input",
  "summary": "one line",
  "decisions": [{ "fork": "", "chosen": "", "rejected": "", "why": "" }],
  "deviations": "", "verify": "how to check",
  "question": "only on needs-input, empty otherwise" }
```

`status` is the only field the dispatcher interprets. `decisions` is the fork ledger: the agent picks and flags, it does not stop to ask. The bar for `needs-input` is one sentence: would the answer invalidate more than half the units? If not, it is a fork, and the agent commits to the branch it would bet on and records the other one under `rejected`.

`result.md` is rendered from this for reading during `review`; edit neither by hand.

## Fences

The agent runs with no MCP servers (`--strict-mcp-config` with no `--mcp-config`, which on 2.1.250 also drops plugin-contributed servers), no user-level settings, hooks or plugins (`--setting-sources project`, so the repo's `CLAUDE.md` still loads; never `--bare`, which kills `CLAUDE.md` discovery), no `gh` credentials (`GH_CONFIG_DIR=/nonexistent`), a dollar budget and a wall-clock timeout, the `pre-push` hook above, and a `git` push URL of `/nonexistent` in its environment. It has built-in tools only: no LSP, no Serena, no Playwright. That is an accepted cost; a `--plugin-dir` whitelist keyed on repo language is the re-entry mechanism, added only when a `failed` `run.log` shows the agent flailing on something a plugin would have caught.

## The RFC layer

The main path. A batch starts as a decided direction in a file: an RFC, a design doc, a spec, a page of notes. The prose is for the team and the daemon never reads it. `ekky:cut <doc> <rfc-id>` reads the repositories the direction touches, cuts the work into tasks of one PR each, declares dependencies with `after:`, drafts the decisions each task needs from the document and the code, and takes your edits in one message before appending two sections for the machine. The review you give is of `## Decisions`, not the prose; that section is the intent card for the whole batch and the place where the fork rate is controlled. A skimmed decisions section pays for itself in reworks downstream. An RFC-writing skill deliberately refuses file paths and implementation detail, which is why the sections are written by `cut`.

```markdown
## Decisions

### 1. <task title>
- <a decision already taken, one line>
- <another>
- Do not: <the one thing this task must not do>

## Tasks

1. <task title> — `repo: /abs/path` `base: main` `model: opus`
   <Done looks like: the behaviour the reviewer will see, two or three sentences>
2. <task title> — `repo: /abs/path` `base: main` `after: 1`
   <Done looks like: ...>
```

`ekky:decompose <rfc.md> <rfc-id>` reads only those two sections and emits `agent-tasks/<rfc-id>/rfc.md` plus one `tasks/<nn-slug>/task.md` per entry, decisions copied verbatim. It refuses an RFC with a task that has no decisions. A task that builds on another names it with `` `after: N` ``: it is cut from N's branch once N is `done`, re-runs whenever N is reworked, and lands as its own PR stacked on N's. The token is the only declaration `decompose` accepts; an entry that says "once 1 is in" in prose is refused with "add `after: 1`" as the exit, as is an `after` to an entry that does not exist, to itself, in a cycle, or in another repository. Two tasks that merely touch the same area of the same repo are not a dependency, only a likely merge conflict at promote time; that is a warning. Then `ekky tackle <rfc-id>` and, later, `ekky:review` task by task.

## Promote

`ekky:promote <rfc-id>` is the roll-out and the one act that leaves this machine. It refuses until every task in the batch is accepted, and refuses a task whose repo has no `origin` — nowhere to push, so `review`'s accept is that task's terminal state and the local merge is yours. Then in order: creates one open ticket per task; renames `ekky/<id>` to your own naming and pushes; opens a PR per branch with a body rewritten from `result.md` (rejected branches stripped); publishes the RFC as a Linear project document with `## Tasks` turned into a table of PRs and tickets; edits each PR body to carry its ticket and each ticket to carry its PR. Naming is either a prefix, giving `<prefix><rfc-id>-<nn-slug>` mechanically, or a scheme over `<type>`, `<ticket>` and `<slug>` for a repo whose branches embed the ticket id (`feat/ENG-2212/retry-webhooks`) — which is why the tickets are created before the branches are named. Every step is recorded in `agent-tasks/<rfc-id>/promote.md` and checked against the remote before acting, so a run that dies halfway is finished by running it again, and nothing is pushed or created twice.

RFC-level rework, where a comment changes the RFC and the batch is re-decomposed against the existing branches, is deliberately not built. It waits for the first real comment that invalidates branches.
