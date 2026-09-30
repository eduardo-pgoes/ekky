package main

import (
	"context"
	"fmt"
	"os"

	"github.com/eduardo-pgoes/ekky/internal/config"
	"github.com/eduardo-pgoes/ekky/internal/sched"
)

func tackle(args []string) error {
	return sched.Tackle(config.FromEnv(), args, os.Stderr)
}

func drain(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("drain takes no arguments")
	}
	return sched.Drain(context.Background(), config.FromEnv(), os.Stderr)
}

func cancel(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("cancel takes one task id")
	}
	return sched.Cancel(config.FromEnv(), args[0], os.Stderr)
}
