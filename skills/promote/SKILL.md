---
name: promote
description: Use when the user wants to publish what ekky built — either a whole RFC batch at once (rename and push the accepted branches, open the PRs, publish the RFC as a Linear project document, create the linked tickets; e.g. "promote bot-routing", "ship the RFC batch", "roll out what ekky built") or one task at a time against tickets that already exist in a planned epic (review the diff, accept or rework, find the ticket, rename, push, open the PR, move on; e.g. "promote open-finance-foundation one task at a time", "ekky:promote _/03", "ship the next task", "walk the epic task by task"). Re-runnable after a partial failure. Do NOT use for a batch promote before every task has been accepted in ekky:review, or for anything the user has not claimed as theirs.
---

# ekky:promote

The roll-out. Until now nothing has left this machine: the branches are local, the RFC is a file, Linear has never heard of any of it. `promote` is the moment the user's name goes on the work, and it comes in two shapes, because authorship does.

- **Batch.** The batch is a change the user will have to justify: nobody planned it, so the RFC, the tickets and the PRs go out together as one argument. Everything the team sees comes out at once: open tickets, pushed branches under the user's own naming, PRs with bodies from the result cards, and the RFC as a project document. Not four verbs, one, because authorship is one act.
- **One task at a time.** The batch is regular work on an epic the team already planned: the tickets exist, the plan lives in the tracker, and a second design document next to it is noise someone will ask about. Authorship is per ticket, so each task goes out on its own: reviewed, matched to its ticket, renamed, pushed, opened, and then the next one.

It is also the only skill here that writes anywhere outward, so it is idempotent by construction: every step checks the ledger and the remote before acting, and a re-run after a failure finishes the job without doing anything twice.

## Input

An rfc id for a batch, or a task id (`<rfc>/<nn-slug>`), or an rfc id with "one at a time" (or the like) for the per-task mode. `EKKY_ROOT` defaults to `~/ekky`; the batch is `$EKKY_ROOT/agent-tasks/<rfc-id>/`, with `rfc.md`, `tasks/<nn-slug>/`, and the ledger `promote.md` once this skill has run at least once. The null RFC `_` has no `rfc.md` and only ever runs per task.

The mode is recorded in the ledger as `mode: batch` or `mode: task` on the first run and never switches: a batch promote would create tickets for tasks the per-task run left ticketless and publish a document the epic does not want, and a per-task run on a batch leaves the document's task table half filled. A ledger with no `mode:` line and a `doc` line is a batch; one with neither is the user's to call, asked once and written down. Refuse the other mode on an RFC that has one, and say which the ledger records.

Also, asked once and remembered in the ledger:

- the branch naming, in one of two shapes. A **prefix** — the user's own convention (`feat/`, `<initials>/`, whatever `git -C <repo> branch --list` shows them using) — makes renamed branches `<prefix><rfc-id>-<nn-slug>`, mechanically, with nothing to ask per task; under the null RFC the `_-` is dropped, `<prefix><nn-slug>`. A **scheme** is a template over `<type>`, `<ticket>` and `<slug>`, for a repo whose convention embeds the ticket id: `<type>/<ticket>/<slug>` gives `feat/ENG-2212/retry-webhooks`. A scheme with `<ticket>` in it is why a batch creates its tickets first; see step 1. Either way the `ekky/` name never reaches a remote.
- batch only: the Linear project the RFC and tickets belong to, and the team; the comment deadline and named reviewers for the RFC.
- per task only: the epic — the Linear parent issue or project the tickets live under — used only to look for a ticket when a task does not name one.

### Resolving a scheme

`<ticket>` is the task's ticket id, read from the ledger's `ticket <nn-slug>:` line, which has been written by the time a name is resolved. `<type>` and `<slug>` are judgment, so they are proposed and confirmed, in a batch in a single pass over the whole batch and never one question per task, per task once for that task:

- `<type>`: the conventional-commit type that best describes the task as a reviewer would file it. Propose it from the types on the task's own commits (`git -C <repo> log --format=%s <upstream>..ekky/<id>`, majority first), and treat that as a starting point, not an answer: a task whose commits are mostly `refactor` is often a `fix` to the person reading the PR list. Name which prefixes the repo already uses (`git -C <repo> branch --list`), and flag a type with no precedent there rather than refusing it.
- `<slug>`: the task's `nn-slug` with the `nn-` stripped, and a leading ticket id stripped too when the slug already carries one (`eng-2172-retry-webhooks` → `retry-webhooks`), which the user shortens if they want it shorter.
- a ticketless task drops the `<ticket>` segment and the separator after it: `<type>/<ticket>/<slug>` becomes `<type>/<slug>`. Say so in the proposal.

In a batch, present it as one table — task, ticket, proposed type, proposed slug, resulting branch — take the corrections, and only then start step 2. Per task, it is one line. Once a `branch` line is in the ledger the name is settled and a re-run never re-asks.

## Preconditions

Refuse cleanly on any, naming the task:

- Batch only: a task in the batch is not accepted: `status` is not `done`, or `<taskdir>/accepted` is missing. Every branch that goes out has been reviewed; that is the whole point of the private phase. The user can explicitly drop a task from the batch (say so in the ledger) but never promote an unreviewed one. Dropping a task drops its descendants with it, the siblings whose `after:` names it, transitively: their branches carry its commits and have nothing to target without it. The per-task mode reviews in line instead, below.
- `gh auth status` fails. A batch also needs the Linear MCP in the session; per task it degrades, below. Say so; do not hand-wave a publish that cannot happen.
- A task's `repo` has no `origin` remote. There is nowhere to push and no PR to open: the work ends at `review`'s accept for that repo, and the merge into `base` is the user's to do by hand. Say so; never invent a remote.
- The main checkout at `repo:` has the branch `ekky/<rfc-id>-<nn-slug>` neither under that name nor under the name the ledger's `branch <nn-slug>:` line records. The branch was deleted; the task needs a rerun. Under a scheme the renamed name is not recomputable, so the ledger is the only place it is written down — treat a missing `branch` line and a missing `ekky/` branch together as the deletion.

## The ledger

`agent-tasks/<rfc-id>/promote.md` is append-only, one line per completed step, and it is read before every step:

```
mode: batch                                   # or task
prefix: feat/                                 # one of prefix: or scheme:
scheme: <type>/<ticket>/<slug>
project: <Linear project name or id>          # batch
epic: <ENG-xxxx or Linear project>            # task
ticket <nn-slug>: <ENG-xxxx> <URL>
ticketless <nn-slug>                          # task: the user said this one has no ticket
branch <nn-slug>: <renamed branch>            # renamed and pushed
pr <nn-slug>: <PR URL>
doc: <Linear document URL>                    # batch
linked <nn-slug>                              # batch: PR body carries the ticket, ticket and document carry each other
dropped <nn-slug>: <reason>
```

Nothing outside this skill reads the ledger except `ekky backlog`, which takes `branch` and `pr` lines only and skips every other line, so a new key here is free.

A step whose line exists is skipped. A step whose line is missing is checked against the remote before acting anyway (`git ls-remote`, `gh pr list --head`), because a crash can land between the act and the line.

## Pushing and opening, in either mode

These two acts are the same whether one task goes out or twelve.

**Rename and push.** Read `repo`, `base` and `after` from `task.md`. If `ekky/<id>` exists, `git -C <repo> branch -m ekky/<id> <renamed>`. Then `git -C <repo> push -u origin <renamed>`; a branch already on the remote at the same commit is a no-op, a branch on the remote at a different commit is a stop (say which, do not force-push). Append the `branch` line.

**Open the PR.** `gh pr list --repo <origin> --head <branch> --json url` first; if one exists, record it and move on. Else draft the title and body, show both to the user with the base they will target, and take their edits; only the version they approve goes out, with `gh pr create --base <pr-base> --head <branch> --title "<title>" --body-file <tmp>`. The body is the first thing a colleague reads with the user's name on it, and a rewrite of an agent's card is exactly where a wrong emphasis or a leaked fork slips through.

For a task without `after`, `<pr-base>` is its `base`. For a task with `after`, it is the upstream's renamed branch, and that name comes from the ledger's `branch <after>:` line — under a scheme it is not recomputable from anything else — checked against `git -C <repo> ls-remote --heads origin <that name>` at the moment of `gh pr create`, per task. If the remote still has it, that is the base. If it does not, the base is the task's own `base`, because the upstream's PR has merged and its branch was deleted, and the dependent now targets what the upstream merged into. Both halves are load-bearing: the ledger says what the branch is called, the remote says whether it still exists, and neither alone is enough.

The body comes from `result.md`, rewritten for a colleague who was not in the room: the summary; the decisions as what was chosen and why, with the rejected branch and the fork wording stripped; deviations; verify. No mention of ekky, the agent, or the intent card. When the ticket is already known — always per task, never in a batch — its id and link go at the top of the body now; that line is also what lets Linear's GitHub integration attach the PR to the ticket, so nothing here attaches it by hand. Append the `pr` line.

## Batch steps

In this order, all tasks through each step before the next, so a failure leaves a consistent batch. Within a step, tasks go in dependency order: a task before every task whose `after:` names it, so a chain 01 → 02 → 03 is pushed and opened as 01, 02, 03 and the remote sees an ordinary stacked-PR set. Tasks with no `after:` keep `nn` order.

1. **Create tickets.** For each task without a `ticket` line: create one Linear issue in the project, title = task title, description = `Done looks like` from `task.md`; state open, assignee the user. No document link yet — it does not exist — and step 5 adds it; the PR attaches itself through the integration once step 5 puts the ticket id in its body. Append the `ticket` line.

   This is first so that a scheme carrying `<ticket>` can name a branch at all: the id has to exist before the name that embeds it. Under a plain `prefix:` the position is immaterial, and first is still the better place for it — an open ticket with nothing attached is the cheapest thing in the run to be left holding if a later step fails.
2. **Rename and push branches.** Resolve every name first, in one pass — mechanically under a `prefix:`, or through *Resolving a scheme* under a `scheme:`. Then rename and push each task, parents first.
3. **Open PRs.** Draft every missing body first and show them in one pass, taking the edits for all of them; then, for each task without a `pr` line, parents first, open the PR with its approved body.
4. **Publish the RFC.** If no `doc` line: build the document from `rfc.md` with `## Tasks` replaced by a table of task title, PR link and ticket, and `## Decisions` kept as it is; the decisions are what the team is meant to argue with. Both columns are filled — tickets and PRs both exist by now — so nothing is left `pending` and nothing needs backfilling. Create it as a Linear project document via the MCP, status `In Review`, with the deadline and reviewers. Append the `doc` line.

   A `doc` line seeded by hand means the document already exists and this step is a no-op, but its task table is still filled in step 5, exactly as one written here would have been.
5. **Link.** For each task without a `linked` line: `gh pr edit <url> --body-file <tmp>` with the ticket id and link added at the top of the body, which is what lets the integration attach the PR to the ticket; add the document link to the ticket; and fill that task's row of the document's task table with its PR and ticket. Append the `linked` line.
6. **Report.** The table: task, branch, PR, PR base, ticket. Then confirm with `git -C <repo> ls-remote --heads origin 'ekky/*'` that nothing named `ekky/` is on any remote; if something is, say so and stop, that is a bug. A dependent's PR base is always a renamed name or its `base`, never `ekky/`; the check covers it unchanged.

## One task at a time

A loop over the batch, one task all the way out before the next is touched. The user can stop between any two tasks; a re-run starts at the first task without a `pr` line.

**Order.** Dependency order, as in a batch: a parent before every task whose `after:` names it, `nn` order otherwise. Given a task id, start there — but a task whose upstream has no `pr` line cannot go first, because its PR would target a branch the remote has never seen. Say so and start at the upstream; the user can insist only by dropping the dependency, which is a rework, not a promote.

For each task:

1. **Gate.** Read `status` and `<taskdir>/accepted`.
   - `done`, not accepted: run `ekky:review`'s *A `done` task* procedure here, all four steps, on this task — card, diff, findings, outcome. **accept** and **tweak** continue to step 2. **rework** takes the task out of this sitting: `ekky tackle` has re-readied it and its descendants, so name them and skip all of them. In a chain a rework takes its whole subtree out of the sitting, which makes the tweak the path that keeps the loop moving; weigh that out loud before the note is confirmed.
   - `done` and accepted: it was reviewed already. Say when (`accepted 2026-09-12`) and go to step 2; show the diff again only if the user asks.
   - anything else (`needs-input`, `failed`, `running`, `new`): skip it and its descendants, and say which state held it back. A `needs-input` question is a one-line answer through `ekky:review` if the user wants to take it now.
2. **Ticket.** Resolve the one ticket this task goes out under.
   - The ledger has a `ticket` or `ticketless` line: settled, move on.
   - `task.md` has `ticket:` in its frontmatter: read that issue via the Linear MCP and show its id and title in one line, as a check that the binding survived; the user confirms or corrects.
   - Neither: look under the ledger's `epic:` for it. List the epic's issues, rank them against the task's title and `Done looks like`, and propose the best one or two with the reason each fits; drop the ones the ledger already binds to another task from the top, but show them if nothing else fits, because one ticket can legitimately carry two PRs. The user picks one, names another id, or says the task has none.
   - No ticket is an answer, not a refusal: ekky runs ticketless work too. Append `ticketless <nn-slug>` and carry on. Never create a ticket in this mode; if the user wants one, they file it and name the id.

   Append `ticket <nn-slug>: <ENG-xxxx> <URL>` for a bound one. Without the Linear MCP in the session, a ticket named in `task.md` or by the user still goes out by id — the URL is `https://linear.app/<workspace>/issue/<id>`; the epic search cannot happen, so ask for an id or take ticketless.
3. **Branch.** Resolve this task's name alone — mechanically under a `prefix:`, as one line through *Resolving a scheme* under a `scheme:` — then rename and push.
4. **PR.** Draft the body with the ticket at the top, or none if the task is ticketless; show it, take the edits, then open it. The ticket needs nothing from this skill afterwards: the integration attaches the PR and moves the state, and a second writer only races it.
5. **Next.** One line — task, branch, PR, PR base, ticket or `ticketless` — then go straight on to the next task; the user says stop when they want to.

When nothing promotable is left, report the sitting: promoted this run, sent back (and the tasks each rework re-runs), skipped (and the state that held each back), and what remains for another sitting. Then the same `ekky/*` check against every remote the sitting pushed to.

## Scope fence

This skill pushes, opens PRs, and writes to Linear; it does exactly the writes listed above and no others. It never force-pushes, never deletes a remote branch, never merges, never closes a ticket or moves its state, never edits a task's `status` or `result.md`. In a batch it creates tickets and publishes a document; one task at a time it does neither, and its only writes to a task directory are the ones `ekky:review`'s procedure makes — `accepted`, an appended `## Rework`, a tweak commit — plus the `ekky tackle` a rework runs. Under the null RFC `_` it runs one task at a time and never as a batch.
