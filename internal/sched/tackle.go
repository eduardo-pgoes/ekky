package sched

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/eduardo-pgoes/ekky/internal/config"
	"github.com/eduardo-pgoes/ekky/internal/gitref"
	"github.com/eduardo-pgoes/ekky/internal/model"
	"github.com/eduardo-pgoes/ekky/internal/store"
)

// Fetch runs `git fetch origin <base>` in repo with the session's terminal, so a passphrase prompt reaches the
// user and a session with no tty fails fast.
var Fetch = func(repo, base string) error {
	cmd := exec.Command("git", "-C", repo, "fetch", "--quiet", "origin", base)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stderr, os.Stderr
	return cmd.Run()
}

// StartDrain starts the drain after tackle's writes: the systemd unit when it exists and EKKY_NO_SYSTEMD is
// unset, else a foreground drain in this process. Tests swap it out.
var StartDrain = DefaultStartDrain

// Tackle readies the named tasks (`all`, an rfc id, or task ids) and every task downstream of them, then
// starts the drain. Refusals happen before the first write.
func Tackle(cfg config.Config, args []string, stderr io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("tackle takes task ids, an rfc id, or all")
	}
	logf := func(format string, a ...any) { fmt.Fprintf(stderr, "ekky: "+format+"\n", a...) }
	tree, err := model.Load(cfg.Root)
	if err != nil {
		return err
	}
	targets, err := resolve(cfg, tree, args)
	if err != nil {
		return err
	}
	var chains [][]*model.Task
	for _, task := range targets {
		if tree.Cyclic(task) {
			return fmt.Errorf("after: cycle under %s", task.RFC)
		}
		chain := tree.Chain(task)
		for _, c := range chain {
			if c.Raw == model.Running {
				return fmt.Errorf("%s is running; refusing to re-queue %s", c.ID, task.ID)
			}
		}
		chains = append(chains, chain)
	}

	fetched := map[string]error{}
	announced := map[string]bool{}
	var keep [][]*model.Task
	for _, chain := range chains {
		root := chain[0]
		if root.After != "" {
			keep = append(keep, chain)
			continue
		}
		key := root.Repo + "\x00" + root.Base
		if _, local := gitref.BaseRef(root.Repo, root.Base); local {
			if !refExists(root.Repo, "refs/heads/"+root.Base) {
				logf("%s refused: no origin and no branch %s in %s", root.ID, root.Base, root.Repo)
				continue
			}
			if !announced[key] {
				announced[key] = true
				logf("no origin in %s; using local %s", root.Repo, root.Base)
			}
			keep = append(keep, chain)
			continue
		}
		ferr, done := fetched[key]
		if !done {
			ferr = Fetch(root.Repo, root.Base)
			fetched[key] = ferr
			if ferr == nil {
				logf("fetched origin/%s in %s", root.Base, root.Repo)
			}
		}
		if ferr != nil {
			age, ok := refAge(root.Repo, "origin/"+root.Base)
			if !ok {
				logf("%s refused: fetch failed and there is no local origin/%s in %s", root.ID, root.Base, root.Repo)
				continue
			}
			logf("%s: fetch failed; using origin/%s as of %s", root.ID, root.Base, age)
		}
		keep = append(keep, chain)
	}

	readied := 0
	for _, chain := range keep {
		for _, task := range chain {
			n, err := readyOne(cfg, task, logf)
			if err != nil {
				return err
			}
			readied += n
		}
	}
	if len(keep) == 0 {
		return fmt.Errorf("nothing tackled")
	}
	return StartDrain(cfg, stderr, readied)
}

// resolve turns the arguments into the tasks to ready: `all` is every new task, an rfc id every new task in
// it, and a task id that task whatever its status.
func resolve(cfg config.Config, tree *model.Tree, args []string) ([]*model.Task, error) {
	var out []*model.Task
	seen := map[string]bool{}
	add := func(t *model.Task) {
		if !seen[t.ID] {
			seen[t.ID] = true
			out = append(out, t)
		}
	}
	for _, a := range args {
		switch {
		case a == "all":
			for _, t := range tree.Tasks {
				if t.Raw == model.New {
					add(t)
				}
			}
		case !strings.Contains(a, "/"):
			// An arg without a slash is an rfc id. When no task carries it, the likely typo is a
			// bare slug; refuse with the full id rather than guessing, because nn-slugs repeat
			// across rfcs and a guess that works for `_` today misleads the day it is ambiguous.
			in := tree.InRFC(a)
			if len(in) == 0 {
				var ids []string
				for _, t := range tree.Tasks {
					if t.Slug == a {
						ids = append(ids, t.ID)
					}
				}
				switch len(ids) {
				case 0:
					return nil, fmt.Errorf("no rfc or task named %s", a)
				case 1:
					return nil, fmt.Errorf("no rfc %s; did you mean %s?", a, ids[0])
				default:
					return nil, fmt.Errorf("no rfc %s; tasks with that slug: %s", a, strings.Join(ids, ", "))
				}
			}
			for _, t := range in {
				if t.Raw == model.New {
					add(t)
				}
			}
		default:
			dir, err := model.TaskDir(cfg.Root, a)
			if err != nil {
				return nil, err
			}
			t := tree.Get(a)
			if _, err := os.Stat(filepath.Join(dir, "task.md")); err != nil || t == nil {
				return nil, fmt.Errorf("no task.md in %s", dir)
			}
			add(t)
		}
	}
	return out, nil
}

// readyOne is the write half: `ready` is left alone with a log line, `failed` gets its ## Previous attempt,
// the accepted marker goes, and `ready` is written.
func readyOne(cfg config.Config, task *model.Task, logf func(string, ...any)) (int, error) {
	switch task.Raw {
	case model.Ready:
		logf("%s already ready", task.ID)
		return 0, nil
	case model.Failed:
		if err := store.AppendPreviousAttempt(task.Dir, cfg.LogTail, time.Now()); err != nil {
			return 0, err
		}
	}
	if err := store.Remove(task.Dir, "accepted"); err != nil {
		return 0, err
	}
	if err := store.SetStatus(task.Dir, model.Ready); err != nil {
		return 0, err
	}
	logf("%s ready", task.ID)
	return 1, nil
}

// refExists reports whether ref resolves in repo.
func refExists(repo, ref string) bool {
	return exec.Command("git", "-C", repo, "rev-parse", "--verify", "--quiet", ref).Run() == nil
}

// refAge is how old the commit a local ref points at is, humanised, and whether the ref exists.
func refAge(repo, ref string) (string, bool) {
	out, err := exec.Command("git", "-C", repo, "log", "-1", "--format=%ct", ref).Output()
	if err != nil {
		return "", false
	}
	secs, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return "", false
	}
	return humanAge(time.Since(time.Unix(secs, 0))), true
}

func humanAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// confirmWindow is how long tackle watches for the drain to take the lock after a systemd start.
var confirmWindow = 3 * time.Second

// DefaultStartDrain is what StartDrain does unless a test replaces it.
func DefaultStartDrain(cfg config.Config, stderr io.Writer, readied int) error {
	logf := func(format string, a ...any) { fmt.Fprintf(stderr, "ekky: "+format+"\n", a...) }
	path := LockPath(cfg.Root)
	if Held(path) {
		logf("a drain is live; it picks up what was readied")
		return nil
	}
	if !cfg.NoSystemd && unitExists() {
		if err := systemctl("start", "--no-block", "ekky-drain.service"); err != nil {
			return fmt.Errorf("systemctl start: %v", err)
		}
		logf("drain started (journalctl --user -u ekky-drain -f)")
		deadline := time.Now().Add(confirmWindow)
		for time.Now().Before(deadline) {
			if Held(path) || !anyReady(cfg) {
				return nil
			}
			time.Sleep(200 * time.Millisecond)
		}
		if err := systemctl("start", "--no-block", "ekky-drain.service"); err != nil {
			return fmt.Errorf("systemctl start: %v", err)
		}
		logf("drain not seen on the lock within %s; started it again", confirmWindow)
		return nil
	}
	logf("no ekky-drain.service; draining in the foreground")
	return Drain(context.Background(), cfg, stderr)
}

func anyReady(cfg config.Config) bool {
	tree, err := model.Load(cfg.Root)
	if err != nil {
		return false
	}
	for _, t := range tree.Tasks {
		if t.Raw == model.Ready && t.Shown != model.Blocked {
			return true
		}
	}
	return false
}

func unitExists() bool {
	cmd := exec.Command("systemctl", "--user", "cat", "ekky-drain.service")
	cmd.Stdout, cmd.Stderr = nil, nil
	return cmd.Run() == nil
}

func systemctl(args ...string) error {
	cmd := exec.Command("systemctl", append([]string{"--user"}, args...)...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
