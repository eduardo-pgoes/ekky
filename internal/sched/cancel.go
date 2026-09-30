package sched

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/eduardo-pgoes/ekky/internal/attempt"
	"github.com/eduardo-pgoes/ekky/internal/config"
	"github.com/eduardo-pgoes/ekky/internal/model"
	"github.com/eduardo-pgoes/ekky/internal/store"
)

// Cancel stops a running attempt: it writes the cancel marker, signals the attempt's process group, and
// leaves the classification to the live drain. With no drain on the lock it lands the task itself. The
// lock is looked at before the kill, because a drain whose last attempt just died exits at once.
func Cancel(cfg config.Config, id string, stderr io.Writer) error {
	logf := func(format string, a ...any) { fmt.Fprintf(stderr, "ekky: "+format+"\n", a...) }
	if _, _, err := model.ParseID(id); err != nil {
		return err
	}
	tree, err := model.Load(cfg.Root)
	if err != nil {
		return err
	}
	task := tree.Get(id)
	if task == nil {
		return fmt.Errorf("no task %s", id)
	}
	if task.Raw != model.Running {
		return fmt.Errorf("%s is %s, not running", id, task.Shown)
	}
	lock, free, err := TryLock(LockPath(cfg.Root))
	if err != nil {
		return err
	}
	if err := store.Touch(task.Dir, "cancel", store.Timestamp(time.Now())+"\n"); err != nil {
		return err
	}
	if b, err := os.ReadFile(filepath.Join(task.Dir, "pgid")); err == nil {
		if pgid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil && pgid > 0 {
			attempt.KillGroup(pgid, attempt.Grace)
		}
	}
	if !free {
		logf("%s: cancel sent; the drain lands it failed / cancelled", id)
		return nil
	}
	defer lock.Unlock()
	_ = store.Remove(task.Dir, "cancel")
	_ = store.Remove(task.Dir, "pgid")
	appendLog(task.Dir, "ekky: attempt cancelled", fmt.Sprintf("ekky: %s → failed · %s", id, store.Timestamp(time.Now())))
	if err := store.SetStatus(task.Dir, model.Failed); err != nil {
		return err
	}
	logf("%s → failed (cancelled; no drain was running)", id)
	return nil
}
