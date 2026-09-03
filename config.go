package main

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

type Config struct {
	Root    string
	Branch  string
	Workers int
	DryRun  bool
}

func parseConfig(args []string) (Config, error) {
	fs := flag.NewFlagSet("git-update", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	branch := fs.String("branch", "", "override the target branch for every repository (default: detect each repository's default branch)")
	workers := fs.Int("workers", 4, "maximum parallel updates")
	dryRun := fs.Bool("dry-run", false, "show actions without changing repositories")

	rootBeforeFlags := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		rootBeforeFlags = args[0]
		args = args[1:]
	}

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if *workers < 1 {
		return Config{}, fmt.Errorf("--workers must be at least 1")
	}
	if fs.NArg() > 1 {
		return Config{}, fmt.Errorf("expected at most one root path")
	}
	if rootBeforeFlags != "" && fs.NArg() > 0 {
		return Config{}, fmt.Errorf("root path specified more than once")
	}

	root := "."
	if rootBeforeFlags != "" {
		root = rootBeforeFlags
	} else if fs.NArg() == 1 {
		root = fs.Arg(0)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Config{}, fmt.Errorf("invalid root path: %w", err)
	}

	return Config{Root: absRoot, Branch: strings.TrimSpace(*branch), Workers: *workers, DryRun: *dryRun}, nil
}
