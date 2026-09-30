// Package config reads the environment knobs. Their names and defaults are the script's.
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Config is every knob the dispatcher reads from the environment.
type Config struct {
	Root      string        // EKKY_ROOT, default ~/ekky
	Slots     int           // SLOTS, default 2
	Budget    string        // EKKY_BUDGET in USD, default 15
	Timeout   time.Duration // EKKY_TIMEOUT, default 90m
	Claude    string        // EKKY_CLAUDE, default `claude` on PATH or ~/.local/bin/claude
	LogTail   int           // EKKY_LOG_TAIL, default 40
	NoSystemd bool          // EKKY_NO_SYSTEMD set: tackle drains in the foreground
	UIAddr    string        // EKKY_UI_ADDR, default 127.0.0.1:5178; serve binds loopback only
}

// FromEnv reads the knobs.
func FromEnv() Config {
	home, _ := os.UserHomeDir()
	c := Config{
		Root:      filepath.Join(home, "ekky"),
		Slots:     2,
		Budget:    "15",
		Timeout:   90 * time.Minute,
		Claude:    filepath.Join(home, ".local", "bin", "claude"),
		LogTail:   40,
		NoSystemd: os.Getenv("EKKY_NO_SYSTEMD") != "",
		UIAddr:    "127.0.0.1:5178",
	}
	if v := os.Getenv("EKKY_ROOT"); v != "" {
		c.Root = v
	}
	if n, err := strconv.Atoi(os.Getenv("SLOTS")); err == nil && n > 0 {
		c.Slots = n
	}
	if v := os.Getenv("EKKY_BUDGET"); v != "" {
		c.Budget = v
	}
	if d, ok := parseTimeout(os.Getenv("EKKY_TIMEOUT")); ok {
		c.Timeout = d
	}
	if v := os.Getenv("EKKY_CLAUDE"); v != "" {
		c.Claude = v
	} else if p, err := lookPath("claude"); err == nil {
		c.Claude = p
	}
	if n, err := strconv.Atoi(os.Getenv("EKKY_LOG_TAIL")); err == nil && n >= 0 {
		c.LogTail = n
	}
	if v := os.Getenv("EKKY_UI_ADDR"); v != "" {
		c.UIAddr = v
	}
	return c
}

// parseTimeout accepts Go durations and timeout(1)'s bare-number-plus-suffix forms (90m, 2h, 30s, 45).
func parseTimeout(v string) (time.Duration, bool) {
	if v == "" {
		return 0, false
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d, true
	}
	unit := time.Second
	num := v
	switch v[len(v)-1] {
	case 's':
		num = v[:len(v)-1]
	case 'm':
		unit, num = time.Minute, v[:len(v)-1]
	case 'h':
		unit, num = time.Hour, v[:len(v)-1]
	case 'd':
		unit, num = 24*time.Hour, v[:len(v)-1]
	}
	f, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0, false
	}
	return time.Duration(f * float64(unit)), true
}
