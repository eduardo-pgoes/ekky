package attempt_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/eduardo-pgoes/ekky/internal/attempt"
	"github.com/eduardo-pgoes/ekky/internal/config"
	"github.com/eduardo-pgoes/ekky/internal/harness"
	"github.com/eduardo-pgoes/ekky/internal/model"
	"github.com/eduardo-pgoes/ekky/internal/store"
)

func TestMain(m *testing.M) {
	harness.MaybeFake()
	os.Exit(m.Run())
}

type stderr struct{ strings.Builder }

func setup(t *testing.T) (*harness.Root, *harness.Repo, config.Config) {
	root := harness.NewRoot(t)
	repo := root.NewRepo("r")
	cfg := config.FromEnv()
	return root, repo, cfg
}

func runTask(t *testing.T, root *harness.Root, cfg config.Config, id string) (attempt.Outcome, string) {
	t.Helper()
	tree, err := model.Load(root.Dir)
	if err != nil {
		t.Fatal(err)
	}
	task := tree.Get(id)
	if task == nil {
		t.Fatalf("no task %s", id)
	}
	if err := store.SetStatus(task.Dir, model.Running); err != nil {
		t.Fatal(err)
	}
	var errOut stderr
	out := attempt.Run(context.Background(), cfg, task, &errOut)
	return out, errOut.String()
}

func lastLines(root *harness.Root, id string, n int) []string {
	lines := strings.Split(strings.TrimRight(root.Read(id, "run.log"), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

func TestDoneAttempt(t *testing.T) {
	root, repo, cfg := setup(t)
	root.Task("_/01-hello", harness.TaskSpec{Repo: repo, Base: "main", Model: "sonnet", Account: "fake", Title: "Say hello",
		Fake: []string{"text looking around", "commit hello.txt hi there", "emit done built hello"}})
	repo.BreakOrigin(t)

	out, errOut := runTask(t, root, cfg, "_/01-hello")
	if out.Status != model.Done || out.Reason != "" {
		t.Fatalf("outcome %+v\n%s", out, root.Read("_/01-hello", "run.log"))
	}
	if root.Status("_/01-hello") != "done" || errOut != "ekky: _/01-hello → done\n" {
		t.Errorf("status %s stderr %q", root.Status("_/01-hello"), errOut)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(root.Read("_/01-hello", "result.json")), &env); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"structured_output", "total_cost_usd", "duration_ms", "num_turns", "is_error", "subtype", "result"} {
		if _, ok := env[k]; !ok {
			t.Errorf("result.json lacks %s", k)
		}
	}
	if env["type"] != "result" {
		t.Error("result.json is not the result event")
	}
	want := "# Say hello\n\ndone · `ekky/_-01-hello` · sonnet · $0 · 0s · 2 turns\n\nbuilt hello\n\n## Decisions\n\n- fake fork\n  chose: a\n  rejected: b\n  why: because\n\n## Deviations\n\nnone\n\n## Verify\n\ncat hello.txt\n\n"
	if got := root.Read("_/01-hello", "result.md"); got != want {
		t.Errorf("result.md:\n%s\nwant:\n%s", got, want)
	}
	events := strings.Split(strings.TrimSpace(root.Read("_/01-hello", "events.jsonl")), "\n")
	var types []string
	for _, l := range events {
		var ev map[string]any
		if err := json.Unmarshal([]byte(l), &ev); err != nil {
			t.Fatalf("events.jsonl line %q: %v", l, err)
		}
		types = append(types, ev["type"].(string))
	}
	if types[0] != "system" || types[len(types)-1] != "result" || len(types) != 7 {
		t.Errorf("events: %v", types)
	}
	log := root.Read("_/01-hello", "run.log")
	for _, want := range []string{
		"ekky: _/01-hello · " + repo.Dir + " · main · sonnet · fake (" + root.Home + "/.claude-fake) · ekky/_-01-hello · ",
		"ekky: worktree cut at " + root.TaskDir("_/01-hello") + "/wt from origin/main",
		"  init: model sonnet · claude 2.1.251 · session ",
		"  text: looking around",
		"  Write: hello.txt",
		"  Bash: git add hello.txt && git commit -q -m 'feat: add hello.txt'",
		"  result: success · $0.00 · 2 turns",
		"ekky: _/01-hello → done · ",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("run.log lacks %q:\n%s", want, log)
		}
	}
	if i, j := strings.Index(log, "Write: hello.txt"), strings.Index(log, "Bash: git add"); i > j {
		t.Error("tool calls out of order")
	}
	if root.Exists("_/01-hello", "wt") || root.Exists("_/01-hello", "pgid") || root.Exists("_/01-hello", "cancel") {
		t.Error("wt, pgid or cancel left behind")
	}
	if got := strings.TrimSpace(harness.Git(t, repo.Dir, "log", "--format=%s", "-1", "ekky/_-01-hello")); got != "feat: add hello.txt" {
		t.Errorf("branch tip %q", got)
	}
	if lines, _ := harness.GitOutput(repo.Dir, "worktree", "list"); strings.Contains(lines, "01-hello") {
		t.Errorf("worktree still registered:\n%s", lines)
	}
}

func TestNoLocalOriginRef(t *testing.T) {
	root, repo, cfg := setup(t)
	root.Simple("_/01-x", repo)
	repo.DropRemoteRef(t, "main")
	out, _ := runTask(t, root, cfg, "_/01-x")
	if out.Status != model.Failed || !strings.HasPrefix(out.Reason, "no local origin/main in ") {
		t.Errorf("outcome %+v", out)
	}
	lines := lastLines(root, "_/01-x", 3)
	if !strings.HasPrefix(lines[0], "ekky: no local origin/main in "+repo.Dir) || lines[1] != "ekky: attempt exited 1" || !strings.HasPrefix(lines[2], "ekky: _/01-x → failed") {
		t.Errorf("run.log tail: %q", lines)
	}
	if root.Exists("_/01-x", "wt") {
		t.Error("a worktree was cut without a start point")
	}
}

func TestPreconditionsInScriptOrder(t *testing.T) {
	root, repo, cfg := setup(t)
	other := root.NewRepo("other")
	root.Task("p/01-nomodel", harness.TaskSpec{Repo: repo, Base: "main", Account: "fake"})
	root.Task("p/02-norepo", harness.TaskSpec{Repo: &harness.Repo{Dir: filepath.Join(root.Dir, "nope")}, Base: "main", Model: "opus", Account: "fake"})
	root.Task("p/03-nocreds", harness.TaskSpec{Repo: repo, Base: "main", Model: "opus", Account: "nobody"})
	root.Task("p/04-noup", harness.TaskSpec{Repo: repo, Base: "main", Model: "opus", Account: "fake", After: "99-missing"})
	root.Task("p/05-up", harness.TaskSpec{Repo: other, Base: "main", Model: "opus", Account: "fake"})
	root.Task("p/06-otherrepo", harness.TaskSpec{Repo: repo, Base: "main", Model: "opus", Account: "fake", After: "05-up"})
	root.Task("p/07-upnobranch", harness.TaskSpec{Repo: repo, Base: "main", Model: "opus", Account: "fake"})
	root.Task("p/08-nobranch", harness.TaskSpec{Repo: repo, Base: "main", Model: "opus", Account: "fake", After: "07-upnobranch"})
	local := root.NewLocalRepo("loc")
	root.Task("p/09-localnobranch", harness.TaskSpec{Repo: local, Base: "trunk", Model: "opus", Account: "fake"})
	want := map[string]string{
		"p/01-nomodel":       "task.md needs repo, base and model in its frontmatter",
		"p/02-norepo":        "repo is not a git checkout: " + filepath.Join(root.Dir, "nope"),
		"p/03-nocreds":       "no credentials in " + root.Home + "/.claude-nobody for account 'nobody'",
		"p/04-noup":          "after: 99-missing names no task under p",
		"p/06-otherrepo":     "after: 05-up is in another repo (" + other.Dir + "); nothing to cut from",
		"p/08-nobranch":      "after: branch ekky/p-07-upnobranch does not exist in " + repo.Dir,
		"p/09-localnobranch": "no origin and no branch trunk in " + local.Dir,
	}
	for id, reason := range want {
		out, _ := runTask(t, root, cfg, id)
		if out.Status != model.Failed || out.Reason != reason {
			t.Errorf("%s: %+v, want %q", id, out, reason)
		}
		if lines := lastLines(root, id, 3); lines[0] != "ekky: "+reason || lines[1] != "ekky: attempt exited 1" {
			t.Errorf("%s run.log tail %q", id, lines)
		}
		if root.Status(id) != "failed" {
			t.Errorf("%s status %s", id, root.Status(id))
		}
	}
}

func TestLocalRepoCutsFromLocalBase(t *testing.T) {
	root, _, cfg := setup(t)
	local := root.NewLocalRepo("loc")
	root.Simple("_/01-x", local)
	out, _ := runTask(t, root, cfg, "_/01-x")
	if out.Status != model.Done {
		t.Fatalf("outcome %+v\n%s", out, root.Read("_/01-x", "run.log"))
	}
	if log := root.Read("_/01-x", "run.log"); !strings.Contains(log, "worktree cut at "+root.TaskDir("_/01-x")+"/wt from main") {
		t.Errorf("run.log:\n%s", log)
	}
	if subjects := strings.TrimSpace(harness.Git(t, local.Dir, "log", "--format=%s", "ekky/_-01-x")); subjects != "feat: add _-01-x.txt\nchore: seed" {
		t.Errorf("branch carries %q", subjects)
	}
}

func TestDependentCutsFromUpstreamBranch(t *testing.T) {
	root, repo, cfg := setup(t)
	root.Simple("c/01-a", repo)
	root.Task("c/02-b", harness.TaskSpec{Repo: repo, Base: "main", Model: "sonnet", Account: "fake", After: "01-a", Fake: []string{"commit b.txt", "emit done"}})
	if out, _ := runTask(t, root, cfg, "c/01-a"); out.Status != model.Done {
		t.Fatalf("01-a: %+v", out)
	}
	repo.BreakOrigin(t)
	out, _ := runTask(t, root, cfg, "c/02-b")
	if out.Status != model.Done {
		t.Fatalf("02-b: %+v\n%s", out, root.Read("c/02-b", "run.log"))
	}
	if !strings.Contains(root.Read("c/02-b", "run.log"), "from ekky/c-01-a") {
		t.Error("02-b was not cut from the upstream branch")
	}
	subjects := strings.TrimSpace(harness.Git(t, repo.Dir, "log", "--format=%s", "ekky/c-02-b"))
	if subjects != "feat: add b.txt\nfeat: add c-01-a.txt\nchore: seed" {
		t.Errorf("02-b branch carries %q", subjects)
	}
}

func TestDeadlineKillsTheGroup(t *testing.T) {
	root, repo, cfg := setup(t)
	cfg.Timeout = 2 * time.Second
	attempt.Grace = 500 * time.Millisecond
	defer func() { attempt.Grace = 10 * time.Second }()
	root.Task("_/01-hang", harness.TaskSpec{Repo: repo, Base: "main", Model: "sonnet", Account: "fake", Fake: []string{"hang"}})
	start := time.Now()
	out, _ := runTask(t, root, cfg, "_/01-hang")
	if out.Status != model.Failed || !strings.Contains(out.Reason, "deadline 2s") {
		t.Fatalf("outcome %+v", out)
	}
	if took := time.Since(start); took > 6*time.Second {
		t.Errorf("deadline took %s", took)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(root.Read("_/01-hang", "fake-child.pid")))
	if err != nil {
		t.Fatal("fake wrote no child pid")
	}
	deadline := time.Now().Add(2 * time.Second)
	for syscall.Kill(pid, 0) == nil && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if err := syscall.Kill(pid, 0); err == nil {
		t.Errorf("child process %d survived the group kill", pid)
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	if !root.Exists("_/01-hang", "wt") || root.Exists("_/01-hang", "pgid") {
		t.Error("wt should be kept and pgid removed")
	}
	if lines := lastLines(root, "_/01-hang", 2); !strings.Contains(lines[0], "(deadline 2s)") {
		t.Errorf("tail %q", lines)
	}
}

func TestCancelMarkerLandsCancelled(t *testing.T) {
	root, repo, cfg := setup(t)
	root.Task("_/01-slow", harness.TaskSpec{Repo: repo, Base: "main", Model: "sonnet", Account: "fake", Fake: []string{"sleep 30", "emit done"}})
	done := make(chan attempt.Outcome, 1)
	go func() {
		out, _ := runTask(t, root, cfg, "_/01-slow")
		done <- out
	}()
	deadline := time.Now().Add(5 * time.Second)
	for !root.Exists("_/01-slow", "pgid") && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	pgid, err := strconv.Atoi(strings.TrimSpace(root.Read("_/01-slow", "pgid")))
	if err != nil {
		t.Fatal("no pgid")
	}
	root.Write("_/01-slow", "cancel", "")
	attempt.KillGroup(pgid, time.Second)
	select {
	case out := <-done:
		if out.Status != model.Failed || out.Reason != "attempt cancelled" {
			t.Errorf("outcome %+v", out)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("attempt did not end after the group was killed")
	}
	if root.Exists("_/01-slow", "cancel") || root.Exists("_/01-slow", "pgid") || !root.Exists("_/01-slow", "wt") {
		t.Error("cancel marker consumed, pgid removed, wt kept — one of them is wrong")
	}
	if lines := lastLines(root, "_/01-slow", 2); lines[0] != "ekky: attempt cancelled" {
		t.Errorf("tail %q", lines)
	}
}

func TestDrainStoppedCause(t *testing.T) {
	root, repo, cfg := setup(t)
	root.Task("_/01-slow", harness.TaskSpec{Repo: repo, Base: "main", Model: "sonnet", Account: "fake", Fake: []string{"sleep 30"}})
	tree, _ := model.Load(root.Dir)
	task := tree.Get("_/01-slow")
	_ = store.SetStatus(task.Dir, model.Running)
	ctx, cancel := context.WithCancelCause(context.Background())
	done := make(chan attempt.Outcome, 1)
	go func() { done <- attempt.Run(ctx, cfg, task, &stderr{}) }()
	for !root.Exists("_/01-slow", "pgid") {
		time.Sleep(20 * time.Millisecond)
	}
	cancel(attempt.ErrDrainStopped)
	out := <-done
	if out.Status != model.Failed || out.Reason != "attempt stopped: drain stopped" {
		t.Errorf("outcome %+v", out)
	}
}

func TestOtherOutcomes(t *testing.T) {
	root, repo, cfg := setup(t)
	root.Task("o/01-exit", harness.TaskSpec{Repo: repo, Base: "main", Model: "sonnet", Account: "fake", Fake: []string{"exit 3"}})
	root.Task("o/02-error", harness.TaskSpec{Repo: repo, Base: "main", Model: "sonnet", Account: "fake", Fake: []string{"error"}})
	root.Task("o/03-needs", harness.TaskSpec{Repo: repo, Base: "main", Model: "sonnet", Account: "fake", Fake: []string{"emit needs-input which?"}})
	root.Task("o/04-failed", harness.TaskSpec{Repo: repo, Base: "main", Model: "sonnet", Account: "fake", Fake: []string{"commit x.txt", "emit failed broke"}})
	root.Task("o/05-push", harness.TaskSpec{Repo: repo, Base: "main", Model: "sonnet", Account: "fake", Fake: []string{"commit p.txt", "push", "emit failed"}})

	if out, _ := runTask(t, root, cfg, "o/01-exit"); out.Status != model.Failed || out.Reason != "attempt exited 3" {
		t.Errorf("exit: %+v", out)
	}
	out, _ := runTask(t, root, cfg, "o/02-error")
	if out.Status != model.Failed || out.Reason != "attempt exited 1" {
		t.Errorf("error: %+v", out)
	}
	if log := root.Read("o/02-error", "run.log"); !strings.Contains(log, "ekky: claude result subtype=error_during_execution is_error=true result=fake: error during execution") {
		t.Errorf("error run.log:\n%s", log)
	}
	if root.Exists("o/02-error", "result.md") {
		t.Error("result.md rendered without a structured result")
	}
	if out, _ := runTask(t, root, cfg, "o/03-needs"); out.Status != model.NeedsInput || !root.Exists("o/03-needs", "wt") {
		t.Errorf("needs-input: %+v wt %v", out, root.Exists("o/03-needs", "wt"))
	}
	if md := root.Read("o/03-needs", "result.md"); !strings.HasSuffix(md, "## Question\n\nwhich one? a or b\n\n") || !strings.HasPrefix(md, "# Task 03-needs\n\nneeds-input · `ekky/o-03-needs` · sonnet · ") {
		t.Errorf("needs-input result.md:\n%s", md)
	}
	if out, _ := runTask(t, root, cfg, "o/04-failed"); out.Status != model.Failed || out.Reason != "" || !root.Exists("o/04-failed", "wt") {
		t.Errorf("failed: %+v", out)
	}
	out, _ = runTask(t, root, cfg, "o/05-push")
	if out.Status != model.Failed || !root.Exists("o/05-push", "wt") {
		t.Errorf("push: %+v", out)
	}
	if refs := strings.TrimSpace(harness.Git(t, repo.Origin, "for-each-ref", "--format=%(refname)")); refs != "refs/heads/main" {
		t.Errorf("origin refs after a push attempt: %q", refs)
	}
	if !strings.Contains(root.Read("o/05-push", "result.md"), "push refused") {
		t.Errorf("push result:\n%s", root.Read("o/05-push", "result.md"))
	}
}

func TestParallelCutsShareTheRepo(t *testing.T) {
	root, repo, cfg := setup(t)
	ids := []string{"m/01-a", "m/02-b", "m/03-c", "m/04-d"}
	for _, id := range ids {
		root.Simple(id, repo)
	}
	tree, _ := model.Load(root.Dir)
	done := make(chan attempt.Outcome, len(ids))
	for _, id := range ids {
		task := tree.Get(id)
		_ = store.SetStatus(task.Dir, model.Running)
		go func() { done <- attempt.Run(context.Background(), cfg, task, &stderr{}) }()
	}
	for range ids {
		if out := <-done; out.Status != model.Done {
			t.Errorf("parallel attempt: %+v", out)
		}
	}
	for _, id := range ids {
		if root.Status(id) != "done" {
			t.Errorf("%s: %s\n%s", id, root.Status(id), root.Read(id, "run.log"))
		}
	}
}

func TestPreviousAttemptShape(t *testing.T) {
	root, repo, _ := setup(t)
	root.Simple("_/01-x", repo)
	root.Write("_/01-x", "status", "failed\n")
	root.Write("_/01-x", "run.log", "one\ntwo\nthree\n")
	now := time.Date(2026, 8, 29, 10, 0, 0, 0, time.FixedZone("CEST", 2*3600))
	if err := store.AppendPreviousAttempt(root.TaskDir("_/01-x"), 2, now); err != nil {
		t.Fatal(err)
	}
	want := "\n## Previous attempt\n\n2026-08-29T10:00:00+02:00 — status was failed. Tail of run.log:\n\n```\ntwo\nthree\n```\n"
	if got := root.Read("_/01-x", "task.md"); !strings.HasSuffix(got, want) {
		t.Errorf("task.md tail:\n%s", got)
	}
	root.Write("_/01-x", "run.log", "")
	before := root.Read("_/01-x", "task.md")
	_ = store.AppendPreviousAttempt(root.TaskDir("_/01-x"), 2, now)
	if root.Read("_/01-x", "task.md") != before {
		t.Error("an empty run.log appended a section")
	}
}
