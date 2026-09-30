// Package attempt runs one attempt at one task: preconditions, the cut, the supervised agent, the event
// stream, classification, result.md, cleanup. The caller writes `running` before; this writes the final
// status exactly once, whatever happens.
package attempt

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/eduardo-pgoes/ekky/internal/config"
	"github.com/eduardo-pgoes/ekky/internal/gitref"
	"github.com/eduardo-pgoes/ekky/internal/model"
	"github.com/eduardo-pgoes/ekky/internal/render"
	"github.com/eduardo-pgoes/ekky/internal/store"
)

// Grace is how long a process group gets between TERM and KILL.
var Grace = 10 * time.Second

// cutMu serialises worktree adds across slots: two of them race the repository's lock.
var cutMu sync.Mutex

// ErrDrainStopped is the cause a drain's stop handler cancels attempts with.
var ErrDrainStopped = errors.New("drain stopped")

// Outcome is what an attempt left behind.
type Outcome struct {
	Status model.Status
	Reason string // the failure reason written as the last line before the final one, "" on a clean result
}

type run struct {
	cfg    config.Config
	task   *model.Task
	dir    string
	wt     string
	branch string
	log    *os.File
	logMu  sync.Mutex
	stderr io.Writer
}

func (r *run) logf(format string, args ...any) {
	r.logMu.Lock()
	defer r.logMu.Unlock()
	fmt.Fprintf(r.log, format+"\n", args...)
}

func (r *run) lines(ls []string) {
	if len(ls) == 0 {
		return
	}
	r.logMu.Lock()
	defer r.logMu.Unlock()
	for _, l := range ls {
		fmt.Fprintln(r.log, l)
	}
}

// Run runs one attempt over task; ctx cancellation (with ErrDrainStopped as cause) is the drain's stop.
// stderr receives the dispatcher's own one-line summary, as the script's `log` did.
func Run(ctx context.Context, cfg config.Config, task *model.Task, stderr io.Writer) Outcome {
	r := &run{cfg: cfg, task: task, dir: task.Dir, wt: filepath.Join(task.Dir, "wt"), branch: model.BranchOf(task.ID), stderr: stderr}
	for _, f := range []string{"result.json", "result.md", "cancel", "pgid"} {
		_ = store.Remove(r.dir, f)
	}
	for _, f := range []string{"run.log", "events.jsonl"} {
		_ = os.WriteFile(filepath.Join(r.dir, f), nil, 0o644)
	}
	log, err := os.OpenFile(filepath.Join(r.dir, "run.log"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(stderr, "ekky: %s: %v\n", task.ID, err)
		return r.finish(model.Failed, "")
	}
	r.log = log
	defer log.Close()

	account := task.Account
	if account == "" {
		account = "inherited"
	}
	configDir := model.ConfigDirOf(task.Account)
	after := ""
	if task.After != "" {
		after = " · after " + task.After
	}
	r.logf("ekky: %s · %s · %s · %s · %s (%s) · %s%s · %s", task.ID, task.Repo, task.Base, task.Model, account, configDir, r.branch, after, store.Timestamp(time.Now()))

	cut, reason := r.preconditions(configDir)
	if reason != "" {
		r.logf("ekky: %s", reason)
		r.logf("ekky: attempt exited 1")
		return r.finish(model.Failed, reason)
	}
	if err := r.cut(cut); err != nil {
		r.logf("ekky: %v", err)
		r.logf("ekky: attempt exited 1")
		return r.finish(model.Failed, err.Error())
	}
	return r.spawn(ctx, configDir)
}

// preconditions checks the record in the script's order and returns the start point to cut from.
func (r *run) preconditions(configDir string) (cut, reason string) {
	t := r.task
	rfc := t.RFC
	switch {
	case t.Repo == "" || t.Base == "" || t.Model == "":
		return "", "task.md needs repo, base and model in its frontmatter"
	case !isDir(filepath.Join(t.Repo, ".git")):
		return "", "repo is not a git checkout: " + t.Repo
	case !isFile(filepath.Join(configDir, ".credentials.json")):
		account := t.Account
		if account == "" {
			account = "inherited"
		}
		return "", fmt.Sprintf("no credentials in %s for account '%s'", configDir, account)
	}
	if t.After == "" {
		ref, local := gitref.BaseRef(t.Repo, t.Base)
		if local {
			if _, err := git(t.Repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+ref); err != nil {
				return "", fmt.Sprintf("no origin and no branch %s in %s", t.Base, t.Repo)
			}
			return ref, ""
		}
		if _, err := git(t.Repo, "rev-parse", "--verify", "--quiet", "refs/remotes/"+ref); err != nil {
			return "", fmt.Sprintf("no local %s in %s; ekky tackle fetches it from a session that can", ref, t.Repo)
		}
		return ref, ""
	}
	upDir, _ := model.TaskDir(r.cfg.Root, rfc+"/"+t.After)
	if !isFile(filepath.Join(upDir, "task.md")) {
		return "", fmt.Sprintf("after: %s names no task under %s", t.After, rfc)
	}
	upRec, _, _ := model.ReadRecord(filepath.Join(upDir, "task.md"))
	if upRec.Repo != t.Repo {
		return "", fmt.Sprintf("after: %s is in another repo (%s); nothing to cut from", t.After, upRec.Repo)
	}
	ubranch := model.BranchOf(rfc + "/" + t.After)
	if _, err := git(t.Repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+ubranch); err != nil {
		return "", fmt.Sprintf("after: branch %s does not exist in %s", ubranch, t.Repo)
	}
	return ubranch, ""
}

// cut removes a stale worktree and adds a fresh one on the branch, under the process-wide mutex.
func (r *run) cut(start string) error {
	if err := os.RemoveAll(r.wt); err != nil {
		return err
	}
	cutMu.Lock()
	defer cutMu.Unlock()
	if out, err := git(r.task.Repo, "worktree", "prune"); err != nil {
		return fmt.Errorf("worktree prune: %v: %s", err, strings.TrimSpace(out))
	}
	if out, err := git(r.task.Repo, "worktree", "add", "--quiet", "--no-track", "-B", r.branch, r.wt, start); err != nil {
		return fmt.Errorf("worktree add: %v: %s", err, strings.TrimSpace(out))
	}
	r.logf("ekky: worktree cut at %s from %s", r.wt, start)
	return nil
}

func (r *run) spawn(parent context.Context, configDir string) Outcome {
	t := r.task
	schema, err := os.ReadFile(filepath.Join(r.cfg.Root, "agent", "result.schema.json"))
	if err != nil {
		r.logf("ekky: %v", err)
		return r.finish(model.Failed, err.Error())
	}
	record, err := os.ReadFile(filepath.Join(r.dir, "task.md"))
	if err != nil {
		r.logf("ekky: %v", err)
		return r.finish(model.Failed, err.Error())
	}
	ctx, cancel := context.WithTimeoutCause(parent, r.cfg.Timeout, errDeadline)
	defer cancel()

	cmd := exec.Command(r.cfg.Claude, "-p",
		"--append-system-prompt-file", filepath.Join(r.cfg.Root, "agent", "execute.md"),
		"--add-dir", r.dir,
		"--model", t.Model,
		"--dangerously-skip-permissions",
		"--setting-sources", "project",
		"--strict-mcp-config",
		"--max-budget-usd", r.cfg.Budget,
		"--output-format", "stream-json", "--verbose",
		"--json-schema", string(schema))
	cmd.Dir = r.wt
	cmd.Env = append(os.Environ(),
		"EKKY_TASK="+t.ID,
		"GH_CONFIG_DIR=/nonexistent",
		"CLAUDE_CONFIG_DIR="+configDir,
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=remote.origin.pushurl", "GIT_CONFIG_VALUE_0=/nonexistent")
	cmd.Stdin = bytes.NewReader(record)
	cmd.Stderr = r.log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		r.logf("ekky: %v", err)
		return r.finish(model.Failed, err.Error())
	}
	if err := cmd.Start(); err != nil {
		r.logf("ekky: cannot start %s: %v", r.cfg.Claude, err)
		r.logf("ekky: attempt exited 1")
		return r.finish(model.Failed, "cannot start "+r.cfg.Claude)
	}
	pgid := cmd.Process.Pid
	_ = store.Touch(r.dir, "pgid", strconv.Itoa(pgid)+"\n")

	reaped, supervised := make(chan struct{}), make(chan struct{})
	grace := Grace
	go func() {
		defer close(supervised)
		select {
		case <-ctx.Done():
			_ = syscall.Kill(-pgid, syscall.SIGTERM)
			select {
			case <-reaped:
			case <-time.After(grace):
				_ = syscall.Kill(-pgid, syscall.SIGKILL)
			}
		case <-reaped:
		}
	}()

	events, _ := os.OpenFile(filepath.Join(r.dir, "events.jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
	var last []byte
	reader := bufio.NewReaderSize(stdout, 1<<20)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if events != nil {
				events.Write(line)
			}
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) > 0 {
				if isResult(trimmed) {
					last = append([]byte(nil), trimmed...)
				}
				r.lines(renderLines(trimmed, time.Now()))
			}
		}
		if err != nil {
			break
		}
	}
	if events != nil {
		events.Close()
	}
	waitErr := cmd.Wait()
	close(reaped)
	<-supervised
	_ = store.Remove(r.dir, "pgid")

	rc := 0
	if waitErr != nil {
		rc = -1
		var exit *exec.ExitError
		if errors.As(waitErr, &exit) {
			rc = exit.ExitCode()
		}
	}
	if last != nil {
		_ = os.WriteFile(filepath.Join(r.dir, "result.json"), append(last, '\n'), 0o644)
	}
	cancelled := isFile(filepath.Join(r.dir, "cancel"))
	_ = store.Remove(r.dir, "cancel")

	var parsed *model.Result
	if last != nil {
		parsed, _ = model.ReadResult(filepath.Join(r.dir, "result.json"))
	}
	final, reason := model.Failed, ""
	switch {
	case rc == 0 && parsed != nil && !parsed.IsError && resultWord(parsed.Status):
		final = model.Status(parsed.Status)
	case cancelled:
		reason = "attempt cancelled"
	case context.Cause(ctx) == errDeadline:
		reason = fmt.Sprintf("attempt exited %d (deadline %s)", rc, r.cfg.Timeout)
	case parent.Err() != nil:
		reason = "attempt stopped: " + causeText(parent)
	case rc != 0:
		reason = fmt.Sprintf("attempt exited %d", rc)
	default:
		reason = "no parseable result"
	}
	if reason != "" {
		r.logf("ekky: %s", reason)
		if last != nil && (parsed == nil || parsed.Status == "") {
			r.logf("ekky: claude result %s", resultSummary(last))
		}
	}
	if parsed != nil && parsed.Status != "" {
		r.renderResult(parsed)
	}
	if final == model.Done {
		if out, err := git(t.Repo, "worktree", "remove", "--force", r.wt); err != nil {
			r.logf("%s", strings.TrimSpace(out))
			_ = os.RemoveAll(r.wt)
		}
	}
	return r.finish(final, reason)
}

var errDeadline = errors.New("deadline")

// resultWord is one of the three words the agent may return.
func resultWord(s string) bool {
	switch model.Status(s) {
	case model.Done, model.Failed, model.NeedsInput:
		return true
	}
	return false
}

func causeText(ctx context.Context) string {
	if c := context.Cause(ctx); c != nil {
		return c.Error()
	}
	return "cancelled"
}

func (r *run) renderResult(res *model.Result) {
	var b bytes.Buffer
	render.Result(&b, r.task.Title, r.branch, r.task.Model, res)
	tmp := filepath.Join(r.dir, "result.md.tmp")
	if err := os.WriteFile(tmp, b.Bytes(), 0o644); err == nil {
		_ = os.Rename(tmp, filepath.Join(r.dir, "result.md"))
	}
}

// finish writes the final line and the status, once.
func (r *run) finish(final model.Status, reason string) Outcome {
	if r.log != nil {
		r.logf("ekky: %s → %s · %s", r.task.ID, final, store.Timestamp(time.Now()))
	}
	if err := store.SetStatus(r.dir, final); err != nil {
		fmt.Fprintf(r.stderr, "ekky: %s: %v\n", r.task.ID, err)
	}
	fmt.Fprintf(r.stderr, "ekky: %s → %s\n", r.task.ID, final)
	return Outcome{Status: final, Reason: reason}
}

func isResult(line []byte) bool {
	var ev struct {
		Type string `json:"type"`
	}
	return json.Unmarshal(line, &ev) == nil && ev.Type == "result"
}

func resultSummary(line []byte) string {
	var ev struct {
		Subtype string          `json:"subtype"`
		IsError bool            `json:"is_error"`
		Result  json.RawMessage `json:"result"`
	}
	_ = json.Unmarshal(line, &ev)
	res := ""
	var s string
	if json.Unmarshal(ev.Result, &s) == nil {
		res = s
	} else {
		res = string(ev.Result)
	}
	if len(res) > 400 {
		res = res[:400]
	}
	return fmt.Sprintf("subtype=%s is_error=%v result=%s", ev.Subtype, ev.IsError, res)
}

// KillGroup sends TERM to a process group, then KILL after grace unless it is gone.
func KillGroup(pgid int, grace time.Duration) {
	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
		return
	}
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		if err := syscall.Kill(-pgid, 0); err != nil {
			return
		}
	}
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
}

func git(repo string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	b, err := cmd.CombinedOutput()
	return string(b), err
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
