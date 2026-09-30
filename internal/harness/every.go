package harness

import "encoding/json"

// EveryState fills the root with a tree covering every raw word, the accepted marker, a blocked chain,
// a promote ledger with branch and pr lines, a live pgid, a task with a real result, a typo'd after and a cycle.
func (r *Root) EveryState(repo *Repo) {
	spec := func(after string) TaskSpec {
		return TaskSpec{Repo: repo, Base: "main", Model: "opus", Account: "fake", After: after, Source: "ENG-1"}
	}
	r.Task("r/01-a", spec(""))
	r.Write("r/01-a", "status", "done\n")
	r.Write("r/01-a", "accepted", "2026-08-29T10:00:00+02:00\n")
	r.Task("r/02-b", spec("01-a"))
	r.Write("r/02-b", "status", "ready\n")
	r.Task("r/03-c", spec("02-b"))
	r.Write("r/03-c", "status", "ready\n")
	r.Task("r/04-d", spec(""))
	r.Write("r/04-d", "status", "running\n")
	r.Write("r/04-d", "pgid", "12345\n")
	r.Task("r/05-e", spec(""))
	r.Write("r/05-e", "status", "failed\n")
	r.Write("r/05-e", "run.log", "ekky: r/05-e · start\nline two\nline three\nekky: r/05-e → failed\n")
	r.Task("r/06-f", spec(""))
	r.Write("r/06-f", "status", "needs-input\n")
	r.Task("r/07-g", spec(""))
	r.Task("r/08-h", spec(""))
	r.Write("r/08-h", "status", "done\n")
	r.Task("r/09-i", spec(""))
	r.Write("r/09-i", "status", "done\n")
	r.Write("r/09-i", "accepted", "x\n")
	r.Task("r/10-j", spec(""))
	r.Write("r/10-j", "status", "done\n")
	r.Ledger("r", "prefix: feat/", "branch 09-i: feat/r-09-i", "pr 09-i: https://github.com/x/y/pull/9", "branch 10-j: feat/r-10-j")

	fixture, _ := FixtureEvents()
	last, _ := json.Marshal(fixture[len(fixture)-1])
	r.Task("_/00-smoke", TaskSpec{Repo: repo, Base: "main", Model: "sonnet", Source: "hand-written smoke fixture", Title: "Add a --shout flag to greet"})
	r.Write("_/00-smoke", "status", "done\n")
	r.Write("_/00-smoke", "result.json", string(last))
	r.Write("_/00-smoke", "run.log", "ekky: _/00-smoke · start\ntool Bash: ls\nekky: _/00-smoke → done\n")

	r.Task("t/01-typo", spec("99-nope"))
	r.Write("t/01-typo", "status", "ready\n")

	r.Task("c/01-x", spec("02-y"))
	r.Task("c/02-y", spec("01-x"))
}
