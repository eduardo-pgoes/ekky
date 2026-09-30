---
name: load
description: Use when the user wants to hand a single stray task to the ekky daemon — from a spec file under llms/specs/, a ticket id, a vault note, or the last stretch of this conversation (e.g. "load this into ekky", "ekky:load ENG-1234", "queue this for the daemon", "make a task out of what we just discussed"). Writes one task record under the null RFC with an intent card, with the user present. Do NOT use for a batch of tasks that should come from one document (that is ekky:cut, then ekky:decompose), for running or reviewing a task (ekky tackle / ekky:review), or for anything that is not yet a decided piece of work.
---

# ekky:load

You write one task record for the daemon, with the user present, in about thirty seconds of their attention. The record is what makes the source executable: a vault note is not a spec and a ticket is not a decision, and the daemon must never ingest either on its own. The intent card you write is the whole context the headless agent will ever have.

## Input

One of:

- a path under `llms/specs/` — read it; the card comes almost entirely from its Decisions and Out of scope sections;
- a ticket id (`[A-Z]+-\d+`) — read it via the Linear MCP if one is available in this session, read-only; if not, ask the user to paste the ticket;
- a vault note (a title or path in the zettelkasten) — read it;
- nothing, or "this" — the source is the conversation so far.

Also resolve, asking only when you cannot infer:

- `repo`: the main checkout the task runs in. The current directory if it is a git repository; otherwise ask. Store the absolute path.
- `base`: the branch to cut from and diff against. The repository's default (`origin/HEAD`) when it resolves, else `main`, else ask. It must exist on `origin`; in a repo with no `origin` remote the daemon works from the local branch instead, so it must exist locally.
- `model`: `opus` unless the user names another.
- `ticket`: the ticket the work will go out under, when there is one: the source's id when the source is a ticket, or an id the user names. `ekky:promote` reads it one task at a time instead of searching for it. Leave the line out when there is none; ticketless work is legitimate.
- `account`: which Claude login bills the run. Read `agent-tasks/accounts` under the ekky root: one `<path-prefix> <account>` per line, `~` allowed; the first prefix `repo` starts with wins, and `perso` when none does or the file is absent; say which you picked when you show the card. The dispatcher maps `perso` to `~/.claude` and any other name to `~/.claude-<name>`.

## Write the intent card

Draft the card from the source, then show it to the user in one message and take their edits. Do not interview them section by section; the draft is the interview. Ask at most the two or three questions whose answers become `Decisions made` when the source is too thin to draft from, which is usually the case for a vault note or a conversation fragment.

The card has exactly three sections, and the standard for each is what a headless agent needs, not what a reader enjoys:

- `## Done looks like` — the behaviour the reviewer will see, in a few sentences. A command and its output, a screen and what is on it, a query and its result. Files and functions are not behaviour.
- `## Decisions made` — the two or three calls that are already taken, one line each. Everything else the agent decides by itself, pick-and-flag. A card with no decisions sends the agent to guess on everything; a card with ten is a spec pretending to be a card. If the source is a spec, lift them from its Decisions; if the source is a conversation, they are the things the user said "yes, that" to.
- `## Do not` — the one thing that must not happen, if there is one. It is a hard constraint on the agent, above everything else. If nothing comes to mind, leave the section with a single line: `- nothing beyond the repo's CLAUDE.md`.

Keep the user's language. Keep the record short: the whole file fits on one screen.

## Write the record

The null RFC is `_`. Pick the next two-digit number from the existing directories under `$EKKY_ROOT/agent-tasks/_/tasks/` (`EKKY_ROOT` defaults to `~/ekky`; start at `00` when empty) and a slug from the title: lowercase, dashes, at most six words. Write `agent-tasks/_/tasks/<nn>-<slug>/task.md`:

```markdown
---
repo: <absolute path>
base: <branch>
model: <model>
account: <perso | name from agent-tasks/accounts>
source: <spec path | ticket id | note title | conversation>
ticket: <ticket id, only when there is one>
---
# <title, one line>

## Done looks like
...

## Decisions made
- ...

## Do not
- ...
```

Nothing else goes in the directory. Do not write `status`; the dispatcher owns it and `ekky tackle` is the only way to set it.

Then print the task id and the command that runs it:

```
loaded _/<nn>-<slug>
ekky tackle _/<nn>-<slug>
```

If the user says to run it, run that command yourself. Otherwise stop; tackling is their call, and a loaded task costs nothing until they take it.

## Scope fence

Read-only against Linear and the vault. No branches, no worktrees, no commits, no status writes, no edits to any repository. One record per invocation; a batch of related tasks belongs to `ekky:cut`.
