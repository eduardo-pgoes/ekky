// Package store is the dispatcher's writes into a task directory. The model reads; this writes.
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eduardo-pgoes/ekky/internal/model"
	"github.com/eduardo-pgoes/ekky/internal/render"
)

// SetStatus writes one of the five words, tmp + mv.
func SetStatus(dir string, s model.Status) error {
	if !model.Writable(s) {
		return fmt.Errorf("bad status: %s", s)
	}
	tmp := filepath.Join(dir, "status.tmp")
	if err := os.WriteFile(tmp, []byte(string(s)+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "status"))
}

// Remove deletes a dispatcher-owned file from the task dir; a missing file is not an error.
func Remove(dir, name string) error {
	err := os.Remove(filepath.Join(dir, name))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Touch writes a marker file.
func Touch(dir, name, content string) error {
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
}

// Timestamp is the `date -Iseconds` form the script wrote.
func Timestamp(t time.Time) string { return t.Format("2006-01-02T15:04:05-07:00") }

// AppendPreviousAttempt appends the tail of run.log to task.md under ## Previous attempt, in the script's shape.
// An absent or empty run.log appends nothing.
func AppendPreviousAttempt(dir string, logTail int, now time.Time) error {
	tail := render.Tail(filepath.Join(dir, "run.log"), logTail)
	if tail == "" {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(dir, "task.md"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "\n## Previous attempt\n\n%s — status was %s. Tail of run.log:\n\n```\n%s```\n",
		Timestamp(now), model.StatusOf(dir), strings.TrimRight(tail, "\n")+"\n")
	return err
}
