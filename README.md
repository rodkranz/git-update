# git-update

[![CI](https://github.com/rodkranz/git-update/actions/workflows/ci.yml/badge.svg)](https://github.com/rodkranz/git-update/actions/workflows/ci.yml)

A safe full-screen terminal UI for updating many Git repositories from one place.

Built with Bubble Tea v2, Bubbles v2 and Lip Gloss v2.

## Features

- recursively finds top-level Git repositories;
- ignores nested repositories and submodule-style `.git` files;
- shows current branch and local changes;
- prompts before switching branches or continuing with a dirty worktree;
- never runs stash, reset, clean, force checkout or force pull;
- pulls with `git pull --ff-only origin <branch>`;
- updates clean repositories in parallel (4 workers by default);
- supports `--dry-run`.

Because the executable is named `git-update`, Git exposes it as:

```bash
git update ~/Projects
```

## Build

Requirements: Go 1.25+ and Git in `PATH`.

```bash
go build -trimpath -o git-update .
```

Or:

```bash
./install.sh
```

## Usage

```bash
git update [path] [flags]
```

Examples:

```bash
git update ~/Projects
git update ~/Projects --dry-run
git update ~/Projects --branch main
git update ~/Projects --workers 8
```

Keys:

- `↑/↓` or `j/k`: select repository
- `u` or `Enter`: update selected repository
- `a`: update all safe repositories
- `r`: rescan
- `q`: quit
- `y/n`: confirm/cancel actions that need attention

## Tests

The test suite creates isolated temporary Git repositories, bare remotes, commits and clones. It never touches your real projects and does not require network access for the Git integration fixtures.

```bash
make test
```

Coverage includes:

- CLI/config parsing;
- repository discovery;
- nested repo and submodule exclusion;
- clean/dirty worktrees;
- different branches;
- merge/rebase-style operation detection;
- confirmation safety rules;
- dry-run behavior;
- branch switching;
- real fast-forward pull against a temporary bare remote.

## CI

GitHub Actions runs on every pull request and push to `main`, and can also be started manually.

The pipeline has three validation gates:

1. **Test** — `go test -race` with coverage.
2. **Lint** — `golangci-lint` pinned to `v2.13.2`.
3. **Build** — builds on Linux and macOS.

Run the equivalent checks locally with:

```bash
make ci
```
