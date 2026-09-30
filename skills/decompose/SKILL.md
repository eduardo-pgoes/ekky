---
name: decompose
description: Use when the user has a document carrying the ekky `## Decisions` and `## Tasks` sections — written by ekky:cut at the bottom of an RFC, a design doc or a page of notes — and wants it turned into task records for the daemon (e.g. "decompose this RFC", "ekky:decompose llms/rfc-bot-routing.md", "cut the RFC into tasks", "load the RFC into ekky"). Emits one task directory per entry in `## Tasks`, intent cards derived from `## Decisions`, with no per-task human touch. Do NOT use for a single stray task (ekky:load), for writing the two sections themselves (ekky:cut), or for a document whose `## Decisions` section is missing or empty — send the user to ekky:cut.
---

# ekky:decompose

You turn one reviewed internal RFC into a batch of task records. The RFC review is where the user's judgment entered, once, as decisions at task granularity; your job is to carry those decisions into intent cards without adding, softening, or reinterpreting them. If the RFC is thin, the daemon will fork on every task and the rework rate will say so. That is the RFC's problem to fix, not yours to paper over: refuse and say what is missing.

You read exactly two sections of the RFC: `## Decisions` and `## Tasks`. The prose above them is for the team; it is not your input.

## Input

A path to the RFC markdown, or the RFC in the conversation. Also an rfc id: a short slug the user picks (`bot-routing`, `sync-v2`); propose one from the title if they do not. It becomes the directory name and the branch prefix segment (`ekky/<rfc-id>-<nn-slug>`), so keep it short and lowercase.

`EKKY_ROOT` defaults to `~/ekky`. The batch lives at `$EKKY_ROOT/agent-tasks/<rfc-id>/`.

## The two sections

The document above these two sections is whatever the user wrote for people: an RFC, a design doc, notes. `ekky:cut` appends the two sections at the end. Their shape is a contract with this skill:

```markdown
## Decisions

### 1. <task title>
- <a decision already taken, one line>
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

Numbers in `## Tasks` and `## Decisions` match. `model:` is optional and defaults to `opus`; `account:` is optional and is inferred from the repo path through `agent-tasks/accounts` under the ekky root (one `<path-prefix> <account>` per line, first match wins, `perso` otherwise); `repo:` and `base:` are required per task because a batch can span repositories. `ticket:` is optional: the id of the ticket a task goes out under when the batch works an epic whose tickets already exist, carried into the record so `ekky:promote` one task at a time does not have to search for it.

`after: N` is the one way to declare that a task builds on another. The dispatcher cuts such a task from N's local `ekky/` branch instead of `origin/<base>`, runs it only once N is `done`, and re-runs it whenever N is reworked; `promote` opens its PR against N's pushed branch. One value: a chain or a tree, never two parents. N must be an entry in this RFC with the same `repo:`; `base:` stays what the task eventually merges into. The token is the declaration. A dependency that exists only in the prose is not carried, it is refused.

## Refuse when

Refuse the whole batch, with the reason, and write nothing:

- `## Decisions` or `## Tasks` is missing, or a task has no subsection under `## Decisions`, or that subsection has no decision (a lone `Do not` does not count). Name the task. The exit is `ekky:cut`, which writes the sections with the user present.
- A dependency between tasks that the later entry does not declare with `after: N`. A dependency is any of: a task's `Done looks like` can only be true after another task lands; a task's decisions refer to something another task creates; a `## Tasks` entry says "after" or "depends on" or "once N is in" in prose. Name both tasks and the exit: add `` `after: N` `` to the later entry. Never infer the token from the prose and never write it into the record yourself; never quietly merge the tasks either. If the user would rather not stack, the older exits still stand: merge the two into one task, or move the later one to a second RFC.
- An `after` you cannot carry: it names a number with no entry, names its own entry, closes a cycle (1 after 2 and 2 after 1, at any length), or names an entry whose `repo:` differs (nothing to cut from). Name the entries and what is wrong with the token.
- A `repo:` path is not a git checkout, or `base` does not exist where the daemon will cut from: on `origin` when the repo has that remote (`git -C <repo> ls-remote --heads origin <base>`), as a local branch when it has none (`git -C <repo> rev-parse --verify refs/heads/<base>`). A repo without an `origin` is legitimately local-only, not a refusal.
- A batch directory `agent-tasks/<rfc-id>/` already exists. This skill does not re-decompose; that is RFC-level rework and it is not built yet. Ask for a different id or stop.

Two tasks touching the same area of the same repo is not a dependency, only a likely merge conflict at promote time. Warn, do not refuse.

## Emit

1. Write `agent-tasks/<rfc-id>/rfc.md`: the RFC, verbatim.
2. For each entry `N` in `## Tasks`, write `agent-tasks/<rfc-id>/tasks/<NN>-<slug>/task.md`, where `NN` is the entry number zero-padded to two digits and the slug is from the title (lowercase, dashes, at most six words):

   ```markdown
   ---
   repo: <from the Tasks entry>
   base: <from the Tasks entry>
   model: <from the Tasks entry, or opus>
   account: <from the Tasks entry, or inferred from repo>
   after: <the <NN>-<slug> of entry N, only when the entry carries `after: N`>
   source: rfc <rfc-id>
   ticket: <from the Tasks entry, only when it carries one>
   ---
   # <task title>

   ## Done looks like
   <the Tasks entry's description, verbatim>

   ## Decisions made
   - <each decision bullet from the Decisions subsection, verbatim, minus the Do not line>

   ## Do not
   - <the Do not line, or `nothing beyond the repo's CLAUDE.md` if there is none>
   ```

   Verbatim means verbatim. You do not improve the user's decisions on the way to the card. `after:` is the directory name of the entry it points at, so the dispatcher can resolve it without reading the RFC; an entry without the token gets no `after:` line at all.
3. Write nothing else in those directories. No `status`: the tasks are `new` until tackled.
4. Print the table: number, slug, repo, base, after (or `-`), ticket (or `-`), and the count of decisions. Then the command that runs the batch:

   ```
   ekky tackle <rfc-id>
   ```

   Run it if the user says so; otherwise stop.

## Scope fence

Read-only against Linear, the vault and every repository except for `ls-remote`. No branches, no worktrees, no commits, no status writes, no edits to the RFC. One batch per invocation.
