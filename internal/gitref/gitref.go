// Package gitref decides where a task's base lives. A repo with an origin remote works from the
// fetched origin/<base>; a repo with none is legitimately local-only — there is nothing to fetch
// and no staleness to warn about, so the local branch itself is the base.
package gitref

import "os/exec"

// BaseRef is the ref a root task cuts from and is diffed against: origin/<base> when repo has an
// origin remote, the local <base> when it has none. Both tackle and the attempt runner take the
// mode from this one function, so a task readied by tackle is never refused at cut time for the
// other reading.
func BaseRef(repo, base string) (ref string, local bool) {
	if exec.Command("git", "-C", repo, "remote", "get-url", "origin").Run() == nil {
		return "origin/" + base, false
	}
	return base, true
}
