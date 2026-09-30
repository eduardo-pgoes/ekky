---
name: cut
description: Use when the user has a decided direction — an RFC from the plugin rfc skill, a design doc, a ticket-spec, a page of notes, or the last stretch of this conversation — and wants it turned into an ekky batch (e.g. "cut this into ekky tasks", "ekky:cut llms/rfc-bot-routing.md", "make a batch out of this design", "prepare this RFC for ekky"). Reads the repositories, cuts the work into tasks, declares dependencies with `after:`, interviews the user for the decisions each task needs, and appends the `## Decisions` and `## Tasks` sections that ekky:decompose consumes. Do NOT use for a single stray task (ekky:load), for the mechanical emit of task records from a document that already carries the two sections (ekky:decompose), or for a proposal still seeking consensus — send that to the plugin rfc skill first.
---

# ekky:cut

You write the two sections that make a document executable by the daemon: `## Decisions`, task by task, and `## Tasks`, with the coordinates of each. `ekky:load` does this for one task with the user present, in thirty seconds. You do it for a batch, in a few minutes, and the batch's rework rate is decided here. Every fork the headless agent takes later is a decision you did not take now; every `needs-input` is a question you could have answered with the user in the room.

An RFC-writing skill will not write these sections and should not: its document argues a direction to people who were not there, and it refuses file paths and implementation detail on purpose. Your sections are the opposite kind of text, written for an agent that starts cold with a repository and nothing else. They live at the bottom of the same file because `promote` publishes the whole thing, decisions included, and the decisions are what the team is meant to argue with.

## Input

A path to the direction document, in whatever shape it arrived: an RFC, a TDD, a spec under `llms/specs/`, notes. Or nothing, when the direction is the conversation so far; then you write `llms/<rfc-id>.md` with a title and the direction in a few paragraphs, and the two sections under it. The prose above the sections is not your input beyond reading it; you neither edit nor improve it.

Also an rfc id, a short lowercase slug (`bot-routing`, `sync-v2`); propose one from the title if the user does not give one. `decompose` uses it as the directory name and the branch segment, and `agent-tasks/<rfc-id>/` must not exist yet.

The direction has to be decided. A document whose open questions block a task is not ready: name the question and the task it blocks and stop, rather than turning an open question into a `Decisions` bullet the user never took. An open question that no task depends on is fine; it stays open and the team argues it at promote.

## Read the code first

`decompose` checks that `repo:` is a git checkout and that `base` exists where the daemon will cut from (`origin` when the repo has that remote, the local branch when it has none). You do more than that, because the decisions have to come from somewhere and the document, by design, does not say where things live.

For each repository the direction touches: resolve the absolute path and the default branch (`origin/HEAD`, else `main`; must exist on `origin`, or locally in a repo with no `origin`). Read `CLAUDE.md`, the areas the work will obviously touch, their tests, and the git log of those files. You are looking for the things a cold agent will have to decide and the conventions it will have to match: an existing pattern the direction did not mention, a module that already half-does the thing, two reasonable places for the new code, a test style. Each of those is either a decision you take now or a fork the agent takes later.

If prior ekky results exist for the same repository, read their fork logs; they are the record of what agents actually stop on in this codebase:

```
jq -r '.structured_output.decisions[]?.fork' $EKKY_ROOT/agent-tasks/*/tasks/*/result.json
```

`EKKY_ROOT` defaults to `~/ekky`.

## Cut

A task is the unit of one headless run, one branch, one PR, and one sitting of the user's review. The executor decomposes a task into one to five work units of one commit each and returns `failed` when it cannot fit; so a task that needs more than five commits is two tasks. A task with one trivial commit is not a task; fold it into a neighbour.

The cut is by behaviour, not by layer. A task's `Done looks like` is something the reviewer can run and see; "add the model", "add the endpoint", "add the tests" are three layers of one task, not three tasks. When a slice would only be verifiable once another slice lands, that is a dependency, below, or it is one task.

## Dependencies

For every pair of tasks in the same repository ask one question: can B's `Done looks like` be true before A lands? If not, B carries `` `after: A` ``. One parent per task; a chain or a tree, never two parents. `after:` does not cross repositories, and a task cannot depend on a task in another batch; a dependency you cannot express as `after: N` inside this RFC means merging the tasks or moving the later one to a second RFC.

Then the softer check: two tasks with no dependency that will both edit the same function or file are going to conflict at promote, and the user resolves that by hand. Say so and offer the choice: serialise with `after:` (cheaper merge, slower batch), accept the conflict, or re-cut along a different seam.

Keep independent tasks independent. A chain is slower than a fan-out, and a rework on the head of a chain re-runs everything below it. `after:` declares a real dependency, never a preferred order.

## Decisions

For each task, the question is: what would a cold agent fork on here? Draft the answers from the direction document and from what you read in the code, one line each, stated as the chosen side, and one `Do not:` line. Then show the whole `## Decisions` section to the user in one message and take their edits. The draft is the interview; do not walk the user through the tasks one at a time. Ask the two or three questions whose answers become bullets when the direction is too thin to draft from, and stop there.

A decision is a call with a real alternative behind it, the one the agent would otherwise weigh: which of two modules owns the new code, whether a new function wraps or replaces an existing one, which error shape, what the flag is called, what is deliberately left unchanged. Two or three per task, five at the outside. A task with none sends the agent to guess on everything; a task with ten is a spec pretending to be a card, and the executor will treat the surplus as constraints it cannot honour.

Not a decision: a restatement of `Done looks like`, an implementation step, a preference with no fork behind it, or a decision the repository's `CLAUDE.md` already makes. `Do not` is the one thing that must not happen, the constraint above all others; if the user has none, leave the line out and `decompose` supplies the default.

`model:` is `opus` unless the user says otherwise; say `sonnet` when a task is mechanical and you would bet a smaller model lands it, and leave the choice to the user. `account:` is inferred by `decompose` from the repo path; do not write it.

## Write

Append to the document, in the language it is written in, exactly the shape `decompose` reads:

```markdown
## Decisions

### 1. <task title>
- <a decision, one line, stated as the chosen side>
- <another>
- Do not: <the one thing this task must not do>

### 2. <task title>
- ...

## Tasks

1. <task title> — `repo: /abs/path` `base: main` `model: opus`
   <Done looks like: the behaviour the reviewer will see, two or three sentences>
2. <task title> — `repo: /abs/path` `base: main` `after: 1`
   <Done looks like: ...>
```

Numbers match across the two sections. `repo:` and `base:` are on every entry, because a batch can span repositories. `after: N` appears only where the dependency pass put it. `` `ticket: ENG-xxxx` `` appears only when the direction works an epic whose tickets already exist: read the epic's issues via the Linear MCP, read-only, propose the task-to-ticket mapping in the table below, and write the tokens the user confirms. A task no ticket fits carries none; it goes out ticketless. Nothing else goes in these sections; `decompose` refuses what it does not recognise and takes what it does verbatim, so a hedge in a bullet becomes a hedge in the card.

Then print the table: number, title, repo, base, after (or `-`), ticket (or `-`), decisions count. And the next command:

```
ekky:decompose <path> <rfc-id>
```

Run it if the user says so; otherwise stop. Cutting costs nothing until the batch is decomposed and tackled.

## Scope fence

Read-only against every repository, Linear and the vault. No branches, no worktrees, no commits, no status writes, nothing under `agent-tasks/`. The one write is the two sections at the end of the direction document, or the new file under `llms/` when the direction is the conversation. One batch per invocation.
