# git-update

[![CI](https://github.com/rodkranz/git-update/actions/workflows/ci.yml/badge.svg)](https://github.com/rodkranz/git-update/actions/workflows/ci.yml)

A safe full-screen terminal UI for updating many Git repositories from one place.

Built with Bubble Tea v2, Bubbles v2 and Lip Gloss v2.

## Features

- recursively finds top-level Git repositories;
- ignores nested repositories and submodule-style `.git` files;
- shows current branch and local changes;
- provides explicit `Update`, `Discard & Update`, and `SKIP` actions;
- requires confirmation before switching branches or continuing with a dirty worktree;
- requires a separate destructive confirmation before discarding local changes;
- never stashes, force-checks out, force-pulls, or discards changes automatically;
- `Discard & Update` removes tracked/staged changes with `git reset --hard HEAD` and untracked files with `git clean -fd`; ignored files are preserved;
- pulls with `git pull --ff-only origin <branch>`;
- updates clean repositories in parallel (4 workers by default);
- keeps manually skipped repositories out of `Update All` until the next rescan;
- supports `--dry-run`.

Because the executable is named `git-update`, Git exposes it as:

```bash
git update ~/Projects
```

## Requirements

- Go 1.25+
- Git
- Make

Check your environment with:

```bash
make doctor
```

## Make commands

Run:

```bash
make help
```

Available commands include:

```text
make doctor      Check Go, Git and install path
make deps        Prepare Go dependencies
make tools       Install development tools into .bin
make fmt         Format Go code
make fmt-check   Verify formatting
make vet         Run go vet
make test        Run tests with race detector and coverage
make coverage    Show coverage details
make lint        Run the pinned golangci-lint version
make build       Build bin/git-update
make install     Install git-update into GOBIN/GOPATH/bin
make uninstall   Remove the installed binary
make run         Run from source
make ci          Run all validation checks
make clean       Remove build output
make distclean   Remove build output and local development tools
```

## Build

```bash
make build
```

The binary will be created at:

```text
bin/git-update
```

`make build` prepares the Go module dependencies automatically before compiling.

## Install

```bash
make install
```

The binary is installed into `go env GOBIN`. If `GOBIN` is empty, it falls back to:

```text
$(go env GOPATH)/bin
```

Make sure that directory is in your `PATH`. After installation:

```bash
git update ~/Projects
```

`./install.sh` is also available and delegates to `make install`.

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

### Keys

- `↑/↓` or `j/k`: select repository
- `u` or `Enter`: update selected repository; local changes are kept
- `d`: discard local tracked/staged/untracked changes and then update; requires destructive confirmation
- `s`: SKIP selected repository for the current session
- `a`: update all safe repositories; skipped and attention-required repositories are not modified
- `r`: rescan repositories; this resets session SKIP states
- `q`: quit
- `y`/`Enter`: confirm a pending action
- `n`/`Esc`: cancel a pending action

## Safety model

`Update` is non-destructive. It may switch to the target branch after confirmation, but it does not reset, clean, stash, or delete local work.

`Discard & Update` is intentionally destructive and is only available when local changes exist. Before running it, the UI shows the affected changes and asks for a second confirmation. It executes:

```bash
git reset --hard HEAD
git clean -fd
```

This removes tracked, staged, and untracked changes. Git-ignored files are not removed because `git clean` is intentionally called without `-x` or `-X`.

`SKIP` only changes the in-memory state of the current session. It does not touch the repository.

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
- real fast-forward pull against a temporary bare remote;
- destructive discard of tracked, staged, and untracked changes;
- preservation of ignored files during discard;
- non-destructive discard behavior in dry-run mode.

## CI

GitHub Actions runs on every pull request and push to `main`, and can also be started manually.

The pipeline validates the same Make targets used locally:

1. **Test** — `make test`.
2. **Lint** — `make lint`.
3. **Build** — `make build` on Linux and macOS.

Run everything locally with:

```bash
make ci
```
