package main

import (
	"fmt"
	"os"

	"github.com/eduardo-pgoes/ekky/internal/config"
	"github.com/eduardo-pgoes/ekky/internal/model"
	"github.com/eduardo-pgoes/ekky/internal/render"
	"github.com/eduardo-pgoes/ekky/internal/serve"
)

func splitFlags(args []string, known ...string) (flags map[string]bool, rest []string, err error) {
	flags = map[string]bool{}
	for _, a := range args {
		if len(a) > 2 && a[:2] == "--" {
			ok := false
			for _, k := range known {
				if a == k {
					ok = true
				}
			}
			if !ok {
				return nil, nil, fmt.Errorf("unknown flag %s", a)
			}
			flags[a] = true
			continue
		}
		rest = append(rest, a)
	}
	return flags, rest, nil
}

func backlog(args []string) error {
	flags, rest, err := splitFlags(args, "--needs-me", "--json")
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return fmt.Errorf("backlog takes no arguments")
	}
	tree, err := model.Load(config.FromEnv().Root)
	if err != nil {
		return err
	}
	if flags["--json"] {
		return render.JSON(os.Stdout, render.Filter(tree, flags["--needs-me"]))
	}
	render.Backlog(os.Stdout, tree, flags["--needs-me"])
	return nil
}

func show(args []string) error {
	flags, rest, err := splitFlags(args, "--json")
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("show takes one task id")
	}
	cfg := config.FromEnv()
	if _, _, err := model.ParseID(rest[0]); err != nil {
		return err
	}
	tree, err := model.Load(cfg.Root)
	if err != nil {
		return err
	}
	t := tree.Get(rest[0])
	if t == nil {
		return fmt.Errorf("no task %s under %s", rest[0], model.TasksDir(cfg.Root))
	}
	if flags["--json"] {
		return render.JSONOne(os.Stdout, t)
	}
	render.Show(os.Stdout, t, cfg.LogTail)
	return nil
}

func serveUI(args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("serve takes no arguments")
	}
	return serve.Serve(config.FromEnv(), os.Stderr)
}
