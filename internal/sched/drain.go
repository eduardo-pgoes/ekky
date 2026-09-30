package sched

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/eduardo-pgoes/ekky/internal/attempt"
	"github.com/eduardo-pgoes/ekky/internal/config"
	"github.com/eduardo-pgoes/ekky/internal/model"
	"github.com/eduardo-pgoes/ekky/internal/store"
)

// Drain runs every ready, unblocked task, Slots at a time, until none is left. It holds the drain lock for
// its whole life, reconciles orphaned `running` tasks before scheduling anything, and on TERM or INT stops
// every live attempt (each lands failed / drain stopped) before exiting.
func Drain(parent context.Context, cfg config.Config, stderr io.Writer) error {
	lock, held, err := TryLock(LockPath(cfg.Root))
	if err != nil {
		return err
	}
	if !held {
		return fmt.Errorf("another drain holds %s", LockPath(cfg.Root))
	}
	defer lock.Unlock()

	logf := func(format string, args ...any) { fmt.Fprintf(stderr, "ekky: "+format+"\n", args...) }
	if err := reconcile(cfg, logf); err != nil {
		return err
	}

	ctx, stop := context.WithCancelCause(parent)
	defer stop(nil)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(signals)
	go func() {
		select {
		case sig := <-signals:
			logf("drain: %s received; stopping every live attempt", sig)
			stop(attempt.ErrDrainStopped)
		case <-ctx.Done():
		}
	}()

	type finished struct {
		id  string
		out attempt.Outcome
	}
	results := make(chan finished)
	live := map[string]bool{}
	for {
		started := 0
		if ctx.Err() == nil {
			tree, err := model.Load(cfg.Root)
			if err != nil {
				return err
			}
			for _, task := range tree.Tasks {
				if len(live) >= cfg.Slots {
					break
				}
				if task.Raw != model.Ready || task.Shown == model.Blocked || live[task.ID] {
					continue
				}
				if err := store.SetStatus(task.Dir, model.Running); err != nil {
					logf("%s: %v", task.ID, err)
					continue
				}
				live[task.ID] = true
				started++
				go func(task *model.Task) {
					results <- finished{task.ID, attempt.Run(ctx, cfg, task, stderr)}
				}(task)
			}
		}
		if len(live) == 0 && started == 0 {
			if ctx.Err() == nil && readyAfterUnlock(cfg, lock) {
				continue
			}
			break
		}
		r := <-results
		delete(live, r.id)
	}
	if ctx.Err() != nil {
		logf("drain: stopped")
		return nil
	}
	logf("drain: nothing ready")
	return nil
}

// readyAfterUnlock is the drain's exit protocol: release the lock, look once more, and if a tackle landed
// after the final scan, take the lock back and keep going. A tackle that found the lock held relies on
// this look; one that found it free starts the next drain itself.
func readyAfterUnlock(cfg config.Config, lock *Lock) bool {
	lock.Unlock()
	tree, err := model.Load(cfg.Root)
	if err != nil {
		return false
	}
	pending := false
	for _, task := range tree.Tasks {
		if task.Raw == model.Ready && task.Shown != model.Blocked {
			pending = true
			break
		}
	}
	if !pending {
		return false
	}
	again, err := WaitLock(LockPath(cfg.Root))
	if err != nil {
		return false
	}
	*lock = *again
	return true
}

// reconcile lands every task at `running` as failed / orphaned: a single drain holds the lock, so a
// previous one is gone and nothing else can finish them.
func reconcile(cfg config.Config, logf func(string, ...any)) error {
	tree, err := model.Load(cfg.Root)
	if err != nil {
		return err
	}
	for _, task := range tree.Tasks {
		if task.Raw != model.Running {
			continue
		}
		_ = store.Remove(task.Dir, "pgid")
		_ = store.Remove(task.Dir, "cancel")
		appendLog(task.Dir, "ekky: orphaned: no drain was running", fmt.Sprintf("ekky: %s → failed · %s", task.ID, store.Timestamp(time.Now())))
		if err := store.SetStatus(task.Dir, model.Failed); err != nil {
			return err
		}
		logf("%s → failed (orphaned: no drain was running)", task.ID)
	}
	return nil
}

func appendLog(dir string, lines ...string) {
	f, err := os.OpenFile(filepath.Join(dir, "run.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	for _, l := range lines {
		fmt.Fprintln(f, l)
	}
}
