package sched_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/eduardo-pgoes/ekky/internal/attempt"
	"github.com/eduardo-pgoes/ekky/internal/config"
	"github.com/eduardo-pgoes/ekky/internal/harness"
	"github.com/eduardo-pgoes/ekky/internal/model"
	"github.com/eduardo-pgoes/ekky/internal/render"
	"github.com/eduardo-pgoes/ekky/internal/sched"
)

func TestMain(m *testing.M) {
	harness.MaybeFake()
	attempt.Grace = 500 * time.Millisecond
	os.Exit(m.Run())
}

type buf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (b *buf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *buf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func setup(t *testing.T, slots int) (*harness.Root, *harness.Repo, config.Config) {
	root := harness.NewRoot(t)
	t.Setenv("SLOTS", fmt.Sprint(slots))
	repo := root.NewRepo("r")
	return root, repo, config.FromEnv()
}

func tackle(t *testing.T, cfg config.Config, args ...string) (string, error) {
	t.Helper()
	var out buf
	err := sched.Tackle(cfg, args, &out)
	return out.String(), err
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func backlog(t *testing.T, root *harness.Root) string {
	tree, err := model.Load(root.Dir)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	render.Backlog(&b, tree, false)
	return b.String()
}

func sleeper(root *harness.Root, id string, repo *harness.Repo, secs string) {
	root.Task(id, harness.TaskSpec{Repo: repo, Base: "main", Model: "sonnet", Account: "fake",
		Fake: []string{"sleep " + secs, "commit " + strings.ReplaceAll(id, "/", "-") + ".txt", "emit done"}})
}

func TestTwoReadyTasksUnderTwoSlots(t *testing.T) {
	root, repo, cfg := setup(t, 2)
	sleeper(root, "_/01-a", repo, "0.5")
	sleeper(root, "_/02-b", repo, "0.5")
	start := time.Now()
	out, err := tackle(t, cfg, "all")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Errorf("two 0.5s attempts took %s; they did not run in parallel", took)
	}
	for _, id := range []string{"_/01-a", "_/02-b"} {
		if root.Status(id) != "done" || root.Exists(id, "wt") {
			t.Errorf("%s: %s wt=%v\n%s", id, root.Status(id), root.Exists(id, "wt"), root.Read(id, "run.log"))
		}
	}
	bl := backlog(t, root)
	if !strings.Contains(bl, "01-a") || !strings.Contains(bl, "02-b") {
		t.Errorf("backlog:\n%s", bl)
	}
	for _, want := range []string{"ekky: fetched origin/main in", "ekky: _/01-a ready", "ekky: _/02-b ready", "no ekky-drain.service; draining in the foreground", "ekky: _/01-a → done", "ekky: drain: nothing ready"} {
		if !strings.Contains(out, want) {
			t.Errorf("stderr lacks %q:\n%s", want, out)
		}
	}
	if !sched.Held(sched.LockPath(cfg.Root)) == false {
		t.Error("lock still held after the drain")
	}
}

func TestPushAttemptFailsAndKeepsWorktree(t *testing.T) {
	root, repo, cfg := setup(t, 2)
	root.Task("_/01-push", harness.TaskSpec{Repo: repo, Base: "main", Model: "sonnet", Account: "fake", Fake: []string{"commit p.txt", "push", "emit failed"}})
	if _, err := tackle(t, cfg, "_/01-push"); err != nil {
		t.Fatal(err)
	}
	if root.Status("_/01-push") != "failed" || !root.Exists("_/01-push", "wt") {
		t.Errorf("status %s wt %v", root.Status("_/01-push"), root.Exists("_/01-push", "wt"))
	}
}

func TestReworkRoundTrip(t *testing.T) {
	root, repo, cfg := setup(t, 1)
	root.Simple("_/01-x", repo)
	if _, err := tackle(t, cfg, "_/01-x"); err != nil {
		t.Fatal(err)
	}
	root.Write("_/01-x", "accepted", "now\n")
	os.Remove(root.TaskDir("_/01-x") + "/result.json")
	f, _ := os.OpenFile(root.TaskDir("_/01-x")+"/task.md", os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("\n## Rework\n\nmake it louder\n")
	f.Close()
	if _, err := tackle(t, cfg, "_/01-x"); err != nil {
		t.Fatal(err)
	}
	if root.Status("_/01-x") != "done" || root.Exists("_/01-x", "accepted") {
		t.Errorf("after rework: %s accepted=%v", root.Status("_/01-x"), root.Exists("_/01-x", "accepted"))
	}
	if !root.Exists("_/01-x", "result.json") || !strings.Contains(root.Read("_/01-x", "task.md"), "## Rework\n\nmake it louder") {
		t.Error("the retry did not run cold with the note in the record")
	}
	if subjects := strings.TrimSpace(harness.Git(t, repo.Dir, "log", "--format=%s", "ekky/_-01-x")); subjects != "feat: add _-01-x.txt\nchore: seed" {
		t.Errorf("branch after rework carries %q; it should be fresh from origin/main", subjects)
	}
}

func chain(root *harness.Root, repo *harness.Repo, rfc string, secs string) {
	root.Task(rfc+"/01-a", harness.TaskSpec{Repo: repo, Base: "main", Model: "sonnet", Account: "fake", Fake: []string{"sleep " + secs, "commit a.txt", "emit done"}})
	root.Task(rfc+"/02-b", harness.TaskSpec{Repo: repo, Base: "main", Model: "sonnet", Account: "fake", After: "01-a", Fake: []string{"commit b.txt", "emit done"}})
	root.Task(rfc+"/03-c", harness.TaskSpec{Repo: repo, Base: "main", Model: "sonnet", Account: "fake", After: "02-b", Fake: []string{"commit c.txt", "emit done"}})
}

func TestChainRunsInOrder(t *testing.T) {
	root, repo, cfg := setup(t, 2)
	chain(root, repo, "c", "1")
	var out buf
	done := make(chan error, 1)
	go func() { done <- sched.Tackle(cfg, []string{"c"}, &out) }()
	waitFor(t, "01-a running", func() bool { return root.Status("c/01-a") == "running" })
	bl := backlog(t, root)
	for _, line := range strings.Split(bl, "\n") {
		f := strings.Fields(line)
		if len(f) > 2 && (f[1] == "02-b" || f[1] == "03-c") && f[2] != "blocked" {
			t.Errorf("%s shown %s while 01-a runs", f[1], f[2])
		}
	}
	if !strings.Contains(bl, "blocked  01-a") || !strings.Contains(bl, "blocked  02-b") {
		t.Errorf("upstream column while running:\n%s", bl)
	}
	if err := <-done; err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	for _, id := range []string{"c/01-a", "c/02-b", "c/03-c"} {
		if root.Status(id) != "done" {
			t.Errorf("%s: %s\n%s", id, root.Status(id), root.Read(id, "run.log"))
		}
	}
	subjects := strings.TrimSpace(harness.Git(t, repo.Dir, "log", "--format=%s", "ekky/c-03-c"))
	if subjects != "feat: add c.txt\nfeat: add b.txt\nfeat: add a.txt\nchore: seed" {
		t.Errorf("03-c carries %q", subjects)
	}
	s := out.String()
	if strings.Index(s, "c/01-a → done") > strings.Index(s, "c/02-b → done") || strings.Index(s, "c/02-b → done") > strings.Index(s, "c/03-c → done") {
		t.Errorf("order:\n%s", s)
	}
}

func TestRetackleAcceptedUpstreamCascades(t *testing.T) {
	root, repo, cfg := setup(t, 2)
	chain(root, repo, "c", "0")
	if _, err := tackle(t, cfg, "c"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"c/01-a", "c/02-b", "c/03-c"} {
		root.Write(id, "accepted", "x\n")
	}
	for _, id := range []string{"c/01-a", "c/02-b", "c/03-c"} {
		os.Remove(root.TaskDir(id) + "/result.json")
	}
	out, err := tackle(t, cfg, "c/01-a")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"c/01-a", "c/02-b", "c/03-c"} {
		if root.Exists(id, "accepted") || root.Status(id) != "done" || !root.Exists(id, "result.json") {
			t.Errorf("%s: accepted=%v status=%s reran=%v", id, root.Exists(id, "accepted"), root.Status(id), root.Exists(id, "result.json"))
		}
	}
	for _, want := range []string{"c/01-a ready", "c/02-b ready", "c/03-c ready"} {
		if !strings.Contains(out, want) {
			t.Errorf("stderr lacks %q", want)
		}
	}
}

func TestRefusedWhileDescendantRuns(t *testing.T) {
	root, repo, cfg := setup(t, 1)
	chain(root, repo, "c", "0")
	root.Write("c/01-a", "status", "done\n")
	root.Write("c/01-a", "accepted", "x\n")
	root.Write("c/02-b", "status", "running\n")
	root.Write("c/03-c", "status", "ready\n")
	before := map[string]string{}
	for _, id := range []string{"c/01-a", "c/02-b", "c/03-c"} {
		before[id] = root.Read(id, "status") + root.Read(id, "task.md")
	}
	_, err := tackle(t, cfg, "c/01-a")
	if err == nil || err.Error() != "c/02-b is running; refusing to re-queue c/01-a" {
		t.Errorf("err %v", err)
	}
	for id, b := range before {
		if root.Read(id, "status")+root.Read(id, "task.md") != b {
			t.Errorf("%s was written", id)
		}
	}
	if !root.Exists("c/01-a", "accepted") {
		t.Error("accepted marker removed on a refusal")
	}
}

func TestDependentBeltsAtDrainLevel(t *testing.T) {
	root, repo, cfg := setup(t, 2)
	other := root.NewRepo("other")
	root.Task("x/01-a", harness.TaskSpec{Repo: repo, Base: "main", Model: "sonnet", Account: "fake", Fake: []string{"commit a.txt", "emit done"}})
	root.Task("x/02-b", harness.TaskSpec{Repo: other, Base: "main", Model: "sonnet", Account: "fake", After: "01-a", Fake: []string{"commit b.txt", "emit done"}})
	if _, err := tackle(t, cfg, "x"); err != nil {
		t.Fatal(err)
	}
	if root.Status("x/01-a") != "done" || root.Status("x/02-b") != "failed" || !strings.Contains(root.Read("x/02-b", "run.log"), "is in another repo") {
		t.Errorf("cross-repo: %s %s\n%s", root.Status("x/01-a"), root.Status("x/02-b"), root.Read("x/02-b", "run.log"))
	}

	chain(root, repo, "d", "0")
	root.Write("d/01-a", "status", "done\n")
	if _, err := harness.GitOutput(repo.Dir, "branch", "-D", "ekky/d-01-a"); err == nil {
		t.Fatal("test setup: the upstream branch should not have existed yet")
	}
	if _, err := tackle(t, cfg, "d/02-b"); err != nil {
		t.Fatal(err)
	}
	if root.Status("d/02-b") != "failed" || !strings.Contains(root.Read("d/02-b", "run.log"), "after: branch ekky/d-01-a does not exist") {
		t.Errorf("deleted upstream branch: %s\n%s", root.Status("d/02-b"), root.Read("d/02-b", "run.log"))
	}
	tree, _ := model.Load(root.Dir)
	if c := tree.Get("d/03-c"); c.Raw != model.Ready || c.Shown != model.Blocked {
		t.Errorf("03-c is %s/%s, want ready/blocked", c.Raw, c.Shown)
	}
}

func TestTackleBlockedLogsAlreadyReady(t *testing.T) {
	root, repo, cfg := setup(t, 2)
	chain(root, repo, "c", "0")
	root.Write("c/02-b", "status", "ready\n")
	root.Write("c/02-b", "accepted", "stale\n")
	sched.StartDrain = func(config.Config, ioWriter, int) error { return nil }
	defer func() { sched.StartDrain = sched.DefaultStartDrain }()
	out, err := tackle(t, cfg, "c/02-b")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "c/02-b already ready") || !root.Exists("c/02-b", "accepted") || root.Status("c/03-c") != "ready" {
		t.Errorf("out %q accepted=%v 03=%s", out, root.Exists("c/02-b", "accepted"), root.Status("c/03-c"))
	}
}

func TestCascadeOverFailedAppendsPreviousAttempt(t *testing.T) {
	root, repo, cfg := setup(t, 2)
	chain(root, repo, "c", "0")
	root.Write("c/01-a", "status", "done\n")
	root.Write("c/02-b", "status", "failed\n")
	root.Write("c/02-b", "run.log", "boom\nekky: c/02-b → failed\n")
	sched.StartDrain = func(config.Config, ioWriter, int) error { return nil }
	defer func() { sched.StartDrain = sched.DefaultStartDrain }()
	if _, err := tackle(t, cfg, "c/01-a"); err != nil {
		t.Fatal(err)
	}
	md := root.Read("c/02-b", "task.md")
	if !strings.Contains(md, "## Previous attempt") || !strings.Contains(md, "status was failed. Tail of run.log:\n\n```\nboom\nekky: c/02-b → failed\n```") {
		t.Errorf("task.md:\n%s", md)
	}
	if strings.Contains(root.Read("c/01-a", "task.md"), "## Previous attempt") {
		t.Error("01-a got a previous attempt section without having failed")
	}
	for _, id := range []string{"c/01-a", "c/02-b", "c/03-c"} {
		if root.Status(id) != "ready" {
			t.Errorf("%s: %s", id, root.Status(id))
		}
	}
}

func TestUnreachableOriginWithLocalRef(t *testing.T) {
	root, repo, cfg := setup(t, 2)
	root.Simple("_/01-x", repo)
	repo.BreakOrigin(t)
	out, err := tackle(t, cfg, "_/01-x")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "ekky: _/01-x: fetch failed; using origin/main as of just now") {
		t.Errorf("stderr:\n%s", out)
	}
	if root.Status("_/01-x") != "done" {
		t.Errorf("status %s\n%s", root.Status("_/01-x"), root.Read("_/01-x", "run.log"))
	}
}

func TestUnreachableOriginWithoutLocalRefRefuses(t *testing.T) {
	root, repo, cfg := setup(t, 2)
	root.Simple("_/01-x", repo)
	root.Task("_/02-y", harness.TaskSpec{Repo: repo, Base: "main", Model: "sonnet", Account: "fake", After: "01-x", Fake: []string{"commit y.txt"}})
	repo.BreakOrigin(t)
	repo.DropRemoteRef(t, "main")
	out, err := tackle(t, cfg, "_/01-x")
	if err == nil || err.Error() != "nothing tackled" {
		t.Errorf("err %v", err)
	}
	if !strings.Contains(out, "ekky: _/01-x refused: fetch failed and there is no local origin/main in "+repo.Dir) {
		t.Errorf("stderr:\n%s", out)
	}
	if root.Status("_/01-x") != "new" || root.Status("_/02-y") != "new" {
		t.Errorf("written: %s %s", root.Status("_/01-x"), root.Status("_/02-y"))
	}
}

func TestTackleBareSlugNamesTheMiss(t *testing.T) {
	root, repo, cfg := setup(t, 1)
	root.Simple("_/01-x", repo)
	if _, err := tackle(t, cfg, "01-x"); err == nil || err.Error() != "no rfc 01-x; did you mean _/01-x?" {
		t.Errorf("bare slug: %v", err)
	}
	if root.Status("_/01-x") != "new" {
		t.Errorf("written: %s", root.Status("_/01-x"))
	}
	if _, err := tackle(t, cfg, "99-zzz"); err == nil || err.Error() != "no rfc or task named 99-zzz" {
		t.Errorf("unknown name: %v", err)
	}
	root.Simple("c/01-x", repo)
	if _, err := tackle(t, cfg, "01-x"); err == nil || err.Error() != "no rfc 01-x; tasks with that slug: _/01-x, c/01-x" {
		t.Errorf("ambiguous slug: %v", err)
	}
}

func TestLocalRepoTackledWithoutFetch(t *testing.T) {
	root, _, cfg := setup(t, 2)
	local := root.NewLocalRepo("loc")
	root.Simple("_/01-x", local)
	orig := sched.Fetch
	sched.Fetch = func(repo, base string) error {
		t.Errorf("fetch called for %s", repo)
		return orig(repo, base)
	}
	defer func() { sched.Fetch = orig }()
	out, err := tackle(t, cfg, "_/01-x")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "ekky: no origin in "+local.Dir+"; using local main") {
		t.Errorf("stderr:\n%s", out)
	}
	if root.Status("_/01-x") != "done" {
		t.Errorf("status %s\n%s", root.Status("_/01-x"), root.Read("_/01-x", "run.log"))
	}
	if subjects := strings.TrimSpace(harness.Git(t, local.Dir, "log", "--format=%s", "ekky/_-01-x")); subjects != "feat: add _-01-x.txt\nchore: seed" {
		t.Errorf("branch carries %q; it should be cut from the local main", subjects)
	}
}

func TestLocalRepoWithoutBaseBranchRefuses(t *testing.T) {
	root, _, cfg := setup(t, 2)
	local := root.NewLocalRepo("loc")
	root.Task("_/01-x", harness.TaskSpec{Repo: local, Base: "trunk", Model: "sonnet", Account: "fake", Fake: []string{"commit x.txt", "emit done"}})
	root.Task("_/02-y", harness.TaskSpec{Repo: local, Base: "trunk", Model: "sonnet", Account: "fake", After: "01-x", Fake: []string{"commit y.txt"}})
	out, err := tackle(t, cfg, "_/01-x")
	if err == nil || err.Error() != "nothing tackled" {
		t.Errorf("err %v", err)
	}
	if !strings.Contains(out, "ekky: _/01-x refused: no origin and no branch trunk in "+local.Dir) {
		t.Errorf("stderr:\n%s", out)
	}
	if root.Status("_/01-x") != "new" || root.Status("_/02-y") != "new" {
		t.Errorf("written: %s %s", root.Status("_/01-x"), root.Status("_/02-y"))
	}
}

func TestCancelRunningTask(t *testing.T) {
	root, repo, cfg := setup(t, 1)
	sleeper(root, "_/01-slow", repo, "30")
	var out buf
	done := make(chan error, 1)
	go func() { done <- sched.Tackle(cfg, []string{"_/01-slow"}, &out) }()
	waitFor(t, "pgid", func() bool { return root.Exists("_/01-slow", "pgid") })
	var cout buf
	if err := sched.Cancel(cfg, "_/01-slow", &cout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cout.String(), "cancel sent; the drain lands it") {
		t.Errorf("cancel stderr %q", cout.String())
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if root.Status("_/01-slow") != "failed" || !root.Exists("_/01-slow", "wt") || root.Exists("_/01-slow", "pgid") || root.Exists("_/01-slow", "cancel") {
		t.Errorf("status %s wt=%v pgid=%v cancel=%v", root.Status("_/01-slow"), root.Exists("_/01-slow", "wt"), root.Exists("_/01-slow", "pgid"), root.Exists("_/01-slow", "cancel"))
	}
	if log := root.Read("_/01-slow", "run.log"); !strings.Contains(log, "ekky: attempt cancelled\n") {
		t.Errorf("run.log:\n%s", log)
	}
	if err := sched.Cancel(cfg, "_/01-slow", &cout); err == nil || !strings.Contains(err.Error(), "is failed, not running") {
		t.Errorf("cancel on a failed task: %v", err)
	}
}

func TestCancelWithoutDrainLandsItself(t *testing.T) {
	root, repo, cfg := setup(t, 1)
	root.Simple("_/01-x", repo)
	root.Write("_/01-x", "status", "running\n")
	root.Write("_/01-x", "pgid", "999999999\n")
	var out buf
	if err := sched.Cancel(cfg, "_/01-x", &out); err != nil {
		t.Fatal(err)
	}
	if root.Status("_/01-x") != "failed" || root.Exists("_/01-x", "pgid") || root.Exists("_/01-x", "cancel") || !strings.Contains(root.Read("_/01-x", "run.log"), "ekky: attempt cancelled") {
		t.Errorf("status %s\n%s", root.Status("_/01-x"), root.Read("_/01-x", "run.log"))
	}
}

func TestDrainReconcilesOrphans(t *testing.T) {
	root, repo, cfg := setup(t, 1)
	root.Simple("_/01-stale", repo)
	root.Write("_/01-stale", "status", "running\n")
	root.Write("_/01-stale", "pgid", "4242\n")
	root.Write("_/01-stale", "cancel", "")
	root.Write("_/01-stale", "run.log", "ekky: started once\n")
	root.Simple("_/02-fresh", repo)
	root.Write("_/02-fresh", "status", "ready\n")
	var out buf
	if err := sched.Drain(context.Background(), cfg, &out); err != nil {
		t.Fatal(err)
	}
	if root.Status("_/01-stale") != "failed" || root.Exists("_/01-stale", "pgid") || root.Exists("_/01-stale", "cancel") {
		t.Errorf("orphan: %s", root.Status("_/01-stale"))
	}
	if log := root.Read("_/01-stale", "run.log"); !strings.HasPrefix(log, "ekky: started once\nekky: orphaned: no drain was running\nekky: _/01-stale → failed") {
		t.Errorf("orphan run.log:\n%s", log)
	}
	if root.Status("_/02-fresh") != "done" {
		t.Errorf("fresh: %s", root.Status("_/02-fresh"))
	}
	if s := out.String(); strings.Index(s, "orphaned") > strings.Index(s, "_/02-fresh → done") {
		t.Error("reconciliation ran after scheduling")
	}
}

func TestTermStopsEveryLiveAttempt(t *testing.T) {
	root, repo, cfg := setup(t, 2)
	sleeper(root, "_/01-a", repo, "30")
	sleeper(root, "_/02-b", repo, "30")
	root.Write("_/01-a", "status", "ready\n")
	root.Write("_/02-b", "status", "ready\n")
	var out buf
	done := make(chan error, 1)
	go func() { done <- sched.Drain(context.Background(), cfg, &out) }()
	waitFor(t, "both pgids", func() bool { return root.Exists("_/01-a", "pgid") && root.Exists("_/02-b", "pgid") })
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("drain did not stop")
	}
	for _, id := range []string{"_/01-a", "_/02-b"} {
		if root.Status(id) != "failed" || !strings.Contains(root.Read(id, "run.log"), "ekky: attempt stopped: drain stopped") || root.Exists(id, "pgid") {
			t.Errorf("%s: %s\n%s", id, root.Status(id), root.Read(id, "run.log"))
		}
	}
	if !strings.Contains(out.String(), "drain: terminated received") || !strings.Contains(out.String(), "drain: stopped") {
		t.Errorf("stderr:\n%s", out.String())
	}
}

func TestSecondDrainRefused(t *testing.T) {
	root, repo, cfg := setup(t, 1)
	sleeper(root, "_/01-a", repo, "2")
	root.Write("_/01-a", "status", "ready\n")
	var out buf
	done := make(chan error, 1)
	go func() { done <- sched.Drain(context.Background(), cfg, &out) }()
	waitFor(t, "running", func() bool { return root.Status("_/01-a") == "running" })
	err := sched.Drain(context.Background(), cfg, &out)
	if err == nil || !strings.HasPrefix(err.Error(), "another drain holds ") {
		t.Errorf("second drain: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestBacklogJSONOverEveryState(t *testing.T) {
	root, _, _ := setup(t, 1)
	root.EveryState(root.NewRepo("e"))
	tree, _ := model.Load(root.Dir)
	var b strings.Builder
	if err := render.JSON(&b, render.Filter(tree, true)); err != nil {
		t.Fatal(err)
	}
	var tasks []model.Task
	if err := json.Unmarshal([]byte(b.String()), &tasks); err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		if task.Owner != model.OwnerMe {
			t.Errorf("--needs-me --json includes %s (%s)", task.ID, task.Owner)
		}
	}
	shown := map[model.Status]bool{}
	for _, task := range tree.Tasks {
		shown[task.Shown] = true
	}
	for _, s := range []model.Status{model.New, model.Ready, model.Running, model.Done, model.Failed, model.NeedsInput, model.Accepted, model.Blocked, model.Promoted} {
		if !shown[s] {
			t.Errorf("tree lacks %s", s)
		}
	}
}

// A tackle that lands while a foreground drain is finishing its last attempt is never stranded at ready:
// either the drain's post-unlock look sees it, or the tackle finds the lock free and drains itself.
func TestTackleDuringDrainExitIsNeverStranded(t *testing.T) {
	root, repo, cfg := setup(t, 1)
	for i := 0; i < 20; i++ {
		first, second := fmt.Sprintf("s/%02d-first", i), fmt.Sprintf("s/%02d-second", i)
		sleeper(root, first, repo, "0.3")
		root.Simple(second, repo)
		root.Write(first, "status", "ready\n")
		var out buf
		drained := make(chan error, 1)
		go func() { drained <- sched.Drain(context.Background(), cfg, &out) }()
		waitFor(t, "first running", func() bool { return root.Status(first) == "running" })
		time.Sleep(time.Duration(200+i*15) * time.Millisecond)
		tackled := make(chan error, 1)
		go func() { tackled <- sched.Tackle(cfg, []string{second}, &out) }()
		if err := <-drained; err != nil {
			t.Fatalf("run %d drain: %v", i, err)
		}
		if err := <-tackled; err != nil {
			t.Fatalf("run %d tackle: %v\n%s", i, err, out.String())
		}
		if root.Status(first) != "done" || root.Status(second) != "done" {
			t.Fatalf("run %d: first %s second %s\n%s", i, root.Status(first), root.Status(second), out.String())
		}
	}
}

type ioWriter = io.Writer
