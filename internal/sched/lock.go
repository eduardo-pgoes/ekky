// Package sched is the drain and its two doors, tackle and cancel.
package sched

import (
	"os"
	"path/filepath"
	"syscall"

	"github.com/eduardo-pgoes/ekky/internal/model"
)

// LockPath is the drain's lock file under an EKKY_ROOT.
func LockPath(root string) string { return filepath.Join(model.TasksDir(root), ".drain.lock") }

// Lock is a held flock on the drain lock file.
type Lock struct{ f *os.File }

func open(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
}

// TryLock takes the lock without waiting; held is false when another process has it.
func TryLock(path string) (l *Lock, held bool, err error) {
	f, err := open(path)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if err == syscall.EWOULDBLOCK {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &Lock{f: f}, true, nil
}

// WaitLock blocks until the lock is free and takes it.
func WaitLock(path string) (*Lock, error) {
	f, err := open(path)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return &Lock{f: f}, nil
}

// Unlock releases the lock.
func (l *Lock) Unlock() {
	if l == nil || l.f == nil {
		return
	}
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	l.f.Close()
	l.f = nil
}

// Held reports whether some process holds the lock right now.
func Held(path string) bool {
	l, held, err := TryLock(path)
	if err != nil {
		return false
	}
	if held {
		l.Unlock()
		return false
	}
	return true
}
