# git-update

[![CI](https://github.com/rodkranz/git-update/actions/workflows/ci.yml/badge.svg)](https://github.com/rodkranz/git-update/actions/workflows/ci.yml)

A safe full-screen terminal UI for updating many Git repositories from one place.

Built with Bubble Tea v2, Bubbles v2 and Lip Gloss v2.

## Features

- recursively finds top-level Git repositories;
- ignores nested repositories and submodule-style `.git` files;
- shows current branch and local changes;
- automatically updates clean repositories already on the target branch;
- runs repository updates in parallel (4 workers by default);
- lets you keep working through repositories that need attention while background updates continue;
- automatically moves to the next repository needing a decision after an action is selected;
- supports pulling the current non-target branch without switching;
- supports switching to the target branch and updating while keeping local changes;
- provides destructive `Discard changes & Update` only after an explicit confirmation;
- supports `SKIP` without touching the repository;
- never stashes, force-checks out, force-pulls, or discards changes automatically;
- pulls with `git pull --ff-only origin <branch>`;
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

## Decision flow

Repositories that are already on the target branch and have no local changes are automatically queued for update as soon as the scan finishes.

For a clean repository on another branch:

```text
[m] Switch to <target> & Update
[p] Pull current branch
[s] SKIP
```

For a dirty repository already on the target branch:

```text
[p] Pull current branch (keep changes)
[d] Discard changes & Update
[s] SKIP
```

For a dirty repository on another branch:

```text
[m] Switch to <target> & Update (keep changes)
[p] Pull current branch (keep changes)
[d] Discard changes, switch to <target> & Update
[s] SKIP
```

After selecting `m`, `p`, `d` (after confirmation), or `s`, the UI immediately moves to the next repository needing attention. The selected update is queued and runs in the background when a worker is available.

### Keys

- `↑/↓` or `j/k`: select repository
- `m`: switch to the target branch and update it
- `p`: pull the currently checked-out branch
- `d`: discard local tracked/staged/untracked changes and update the target branch; requires destructive confirmation
- `s`: SKIP selected repository for the current session
- `r`: rescan when no background work is running
- `q`: quit
- `y`/`Enter`: confirm a pending discard
- `n`/`Esc`: cancel a pending discard

## Safety model

Clean repositories on the target branch are safe and update automatically with:

```bash
git pull --ff-only origin <target>
```

`Pull current branch` does not switch branches and does not discard local changes. If Git cannot fast-forward safely because of local work or branch history, the operation fails and the repository is shown as failed.

`Switch to target & Update` keeps local changes. Git itself will block the branch switch if those changes cannot be carried safely to the target branch.

`Discard changes & Update` is intentionally destructive and is only available when local changes exist. Before running it, the UI shows the affected changes and asks for confirmation. It executes:

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
- dry-run behavior;
- branch switching;
- automatic-update classification;
- moving to the next repository needing attention;
- pulling the currently checked-out non-target branch;
- real fast-forward pulls against temporary bare remotes;
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
