---
name: review
description: Use when the user wants to look at what the ekky daemon produced — the next done task, a specific task id, the queue of needs-input questions, or a failed task to triage (e.g. "ekky:review", "review the next task", "what did ekky finish", "answer the daemon's questions", "why did _/03 fail"). Reads the result card, diffs the branch from the main checkout, and either accepts, sends back with a rework note, or answers a parked question. Do NOT use for a generic code review of a branch that ekky did not produce (that is the plugin review skill), for loading tasks (ekky:load), or for publishing anything (ekky:promote).
---

# ekky:review

This is the "I have five PRs to review" moment. The daemon has left branches and result cards behind; you walk the user through them one at a time, from the main checkout, and turn each into one of three outcomes: accepted, sent back with a note, or answered. Nothing here touches a remote, an issue tracker, or a colleague.

The result card is read before the diff, on purpose. It says what the agent built, which forks it took, and where it knowingly deviated; the diff is then read against that account. A deviation the card did not mention is the first thing to flag.

## Input

Optional: a task id (`<rfc>/<nn-slug>`). Without one, run `ekky backlog` and pick in this order: the oldest `needs-input` task (a one-line answer unblocks a whole run), then the oldest `done` task not yet accepted, then any `failed` task. Say which one you picked and why; the user can redirect.

If the picked task has `after:` and its upstream is `done` but not accepted, review the upstream first and say so: the dependent's diff is read against the upstream's branch, and a rework on the upstream re-runs the dependent anyway, so accepting the dependent first is work that may be thrown away. The same applies to a task id the user gives you; redirect with the reason and let them insist.

`EKKY_ROOT` defaults to `~/ekky`; a task lives at `$EKKY_ROOT/agent-tasks/<rfc>/tasks/<nn-slug>/`. Read `task.md` for `repo`, `base` and `after` (flat `key: value` frontmatter) and for the intent card; read `result.md` for the card the agent returned. A task's descendants are the siblings whose `after:` names it, transitively: `grep -l '^after: <nn-slug>' $EKKY_ROOT/agent-tasks/<rfc>/tasks/*/task.md`, then the same for each hit.

## A `done` task

1. Show the result card, compressed to what matters: the summary, each decision as fork → chosen (rejected, why), deviations, and the verify line. Do not paste `result.md` whole.
2. Diff from the main checkout, never from a worktree:
   ```
   git -C <repo> fetch origin <base>
   git -C <repo> log --oneline origin/<base>..ekky/<rfc>-<nn-slug>
   git -C <repo> diff origin/<base>...ekky/<rfc>-<nn-slug>
   ```
   In a repo with no `origin` remote there is nothing to fetch: skip the fetch and use the local `<base>` wherever these commands say `origin/<base>`.
   For a task with `after:`, the base is the upstream's local branch, never `origin/<base>`; against `origin/<base>` the upstream's commits would read as this task's work:
   ```
   git -C <repo> log --oneline ekky/<rfc>-<after>..ekky/<rfc>-<nn-slug>
   git -C <repo> diff ekky/<rfc>-<after>...ekky/<rfc>-<nn-slug>
   ```
   The worktree was removed when the task landed; you do not need it. Read `spec.md` in the task dir when a decision needs its context.
3. Read the diff against the card and the intent card. Surface what a reviewer would: a deviation the card did not declare, a `Do not` that was violated, a decision you would have taken the other way, a test that does not test the behaviour in `Done looks like`, comments that narrate, a commit that bundles two units. Keep it to the findings that would change the outcome; the user decides what matters.
4. Ask for the outcome, with your recommendation first:
   - **accept** — write the current timestamp to `<taskdir>/accepted`. Status stays `done`; the branch waits for `ekky:promote`, whose one-task-at-a-time mode runs this same procedure in line and takes an accepted task straight out. `ekky backlog` shows the task as `accepted`. Accepting a dependent whose upstream is not yet accepted is allowed but never silent: say that a rework on the upstream will re-run this task and clear the acceptance.
   - **rework** — take the note from the user (or draft it from the findings they agreed with and confirm it), append it to `task.md`, remove `<taskdir>/accepted` if present, and run `ekky tackle <id>`. The retry starts cold with the whole record, from fresh `origin/<base>` (the local `<base>` in a repo with no origin) or from the upstream's branch when `after:` is set, so the note has to say what to change, not point at lines of a diff that will not exist. When the task has descendants, `ekky tackle` re-readies all of them: before the note is confirmed, say how many tasks will re-run and which (`rework on 01 re-runs 02 and 03`), because for a small change the tweak below is the cheaper path.
     ```markdown

     ## Rework

     <YYYY-MM-DD>: <the note, in the user's words>
     ```
     A task that has been reworked twice on the same point is not going to get there by itself; say so and offer to tweak instead.
   - **tweak** — for a change too small to send back: `git -C <repo> worktree add <taskdir>/wt ekky/<rfc>-<nn-slug>`, make the edit, commit on that branch in the same style as the agent's commits, then `git -C <repo> worktree remove <taskdir>/wt`. Then accept. The worktree exists only for the tweak. A tweak does not cascade: descendants keep the tip they were cut from, their PR diff stays their own commits, and the merge carries the tweak. That makes it the path to prefer on a task with descendants whenever the change fits in a commit made by hand; the cost is that a tweak which conflicts with a descendant's commits surfaces at merge time, not here.

## A `needs-input` task

Show the question from the card and the options it names. The user answers in a sentence; do not add to it. Append to `task.md` and requeue:

```markdown

## Answers

<YYYY-MM-DD>: Q: <the question, shortened> — A: <the answer verbatim>
```

then `ekky tackle <id>`. If the user thinks the question should never have been asked (the agent should have bet), say so in the answer; the record teaches the next attempt.

## A `failed` task

Show the last lines of `run.log` and the reason the dispatcher wrote there (exit code, timeout, budget, a refused push, an unparseable result). Then the choice: `ekky tackle <id>` to retry (the dispatcher appends the log tail to the record as `## Previous attempt` by itself); edit `model:` in the frontmatter first when the log shows the model flailing rather than the task being wrong; or leave it. A task that failed on a missing capability (needed a browser, needed Linear) is not going to pass on retry; it needs a smaller task or a human.

## Scope fence

Read-only against the branches except for an explicit tweak. Never `git push`, never open a PR, never write to Linear, never edit `status` (only `ekky tackle` does), never delete a branch. Never edit `result.md` or `result.json`. Appending to `task.md` and writing or removing `accepted` are the only writes to the task directory.
