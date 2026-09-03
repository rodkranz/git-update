# git-update

[![CI](https://github.com/rodkranz/git-update/actions/workflows/ci.yml/badge.svg)](https://github.com/rodkranz/git-update/actions/workflows/ci.yml)

A safe full-screen terminal UI for updating many Git repositories from one place.

Built with Bubble Tea v2, Bubbles v2 and Lip Gloss v2.

## Features

- recursively finds top-level Git repositories;
- ignores nested repositories and submodule-style `.git` files;
- detects the default branch **per repository** instead of assuming `master` or `main`;
- uses `origin/HEAD` when available, with safe local fallbacks;
- supports an explicit `--branch` override when you intentionally want one branch for every repository;
- starts in **All repositories** mode;
- automatically updates clean repositories already on their own default branch;
- runs updates in parallel (4 workers by default);
- keeps processing while you answer repositories that need a decision;
- shows a global activity stream in All mode;
- shows repository-specific output when you select a repository;
- keeps common keyboard shortcuts visible in the footer;
- opens a full keyboard shortcuts modal with `?`;
- opens the user's native `$SHELL` in the selected repository with `t` and returns to the TUI after `exit`;
- automatically refreshes repository state after returning from a shell session;
- supports pulling the current branch without switching;
- supports switching to the detected default branch and updating while keeping local changes;
- provides destructive `Discard changes & Update` only after explicit confirmation;
- supports `SKIP` without touching the repository;
- never stashes, force-checks out, force-pulls, or discards changes automatically;
- pulls with `git pull --ff-only`;
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

```text
make help         Show available commands
make doctor       Check Go, Git and install path
make deps         Prepare Go dependencies
make tools        Install development tools into .bin
make fmt          Format Go code
make fmt-check    Verify formatting
make vet          Run go vet
make test         Run tests with race detector and coverage
make coverage     Show coverage details
make lint         Run the pinned golangci-lint version
make build        Build bin/git-update
make install      Install git-update into GOBIN/GOPATH/bin
make uninstall    Remove the installed binary
make run          Run from source
make ci           Run all validation checks
make clean        Remove build output
make distclean    Remove build output and local development tools
```

## Install

```bash
make install
```

The binary is installed into `go env GOBIN`. If `GOBIN` is empty, it falls back to:

```text
$(go env GOPATH)/bin
```

After installation:

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
git update ~/Projects --workers 8
```

By default, the target branch is detected independently for every repository.

If you explicitly want to force the same branch for all repositories:

```bash
git update ~/Projects --branch main
```

## Default branch detection

`git-update` does **not** assume that every repository uses `master` or `main`.

For each repository it tries, in order:

1. the explicit `--branch` override, when provided;
2. the symbolic remote default branch from `refs/remotes/origin/HEAD`;
3. an unambiguous remote `origin/main` or `origin/master`;
4. an unambiguous local `main` or `master`;
5. the current branch when it is already `main` or `master`.

If the default branch cannot be determined safely, the repository is marked as needing attention. The app will not guess. You can pull the current branch, SKIP it, open a shell, or restart with an explicit `--branch` override.

## All repositories mode

The first item in the list is:

```text
◎ All repositories
```

It is selected automatically when the scan finishes.

In this mode:

- every safe repository is queued automatically;
- workers update repositories in parallel;
- the right panel shows overall progress and a global activity stream;
- when a repository needs input, its decision is shown in the same panel;
- after you choose an action, the next repository needing input is shown immediately;
- background updates continue while you make decisions.

Selecting a specific repository changes the right panel to that repository's branch, target, local changes, actions and Git output.

## Decision flow

### Default branch + clean

No question is asked. It is updated automatically:

```text
main/master/default + clean
→ automatic background update
```

### Other branch + clean

```text
[m] Switch to <default> & Update
[p] Pull current branch
[s] SKIP
```

### Default branch + local changes

```text
[p] Pull current branch (keep changes)
[d] Discard changes & Update
[s] SKIP
```

### Other branch + local changes

```text
[m] Switch to <default> & Update (keep changes)
[p] Pull current branch (keep changes)
[d] Discard changes, switch to <default> & Update
[s] SKIP
```

After choosing `m`, `p`, `d` (after confirmation), or `s`, the decision flow immediately advances. The selected Git operation runs in the background when a worker is available.

## Shell here

When a specific repository is selected, press:

```text
[t] Shell here
```

A confirmation modal shows the repository, path and shell. After confirmation, Bubble Tea temporarily releases the terminal and starts the user's native `$SHELL` with the selected repository as the working directory.

Use the shell normally, then run:

```bash
exit
```

`git-update` resumes in the same terminal and immediately re-inspects the selected repository. If the shell leaves the repository clean and on its detected default branch, the normal automatic update flow can continue.

The shell action is not available while that repository already has a `git-update` operation running.

## Keys

The footer always shows the common shortcuts, including `? shortcuts`. Press `?` to open the full keyboard shortcuts modal; press `?` or `Esc` to close it.

- `↑/↓` or `j/k`: select All or a repository
- `g` / `Home`: return to All repositories
- `G` / `End`: jump to the last repository
- `m`: switch to the repository's detected default branch and update it
- `p`: pull the currently checked-out branch
- `d`: discard local tracked/staged/untracked changes and update the default branch; requires destructive confirmation
- `t`: open the native shell in the manually selected repository
- `s`: SKIP the repository for the current session
- `r`: rescan when background work is finished
- `?`: open or close the keyboard shortcuts modal
- `q`: quit
- `Ctrl+C`: quit
- `Enter`: open a confirmed shell or confirm a pending action
- `y`: confirm a pending discard
- `n` / `Esc`: cancel a pending confirmation

## Safety model

Safe automatic updates use:

```bash
git pull --ff-only origin <detected-default-branch>
```

`Pull current branch` does not switch branches and does not discard local changes. If Git cannot fast-forward safely, the operation fails and the repository is shown as failed.

`Switch to default & Update` keeps local changes. Git itself blocks the branch switch if those changes cannot be carried safely.

`Discard changes & Update` is intentionally destructive. Before it runs, the UI shows the affected changes and asks for confirmation. It executes:

```bash
git reset --hard HEAD
git clean -ffd
```

Tracked, staged, untracked changes and untracked nested Git directories are removed. Git-ignored files are preserved because `git clean` is intentionally called without `-x` or `-X`.

## Tests

The test suite creates isolated temporary Git repositories, bare remotes, commits and clones. It never touches your real projects.

```bash
make test
```

Coverage includes repository discovery, default-branch detection for both `main` and `master`, branch overrides, clean/dirty worktrees, current-branch pulls, fast-forward updates, decision advancement, All mode, footer/help shortcuts, native-shell command setup, post-shell repository refresh, discard behavior and dry-run safety.

## CI

GitHub Actions runs on every pull request and push to `main` and validates the same Make targets used locally:

1. **Test** — `make test`
2. **Lint** — `make lint`
3. **Build** — `make build` on Linux and macOS

Run everything locally with:

```bash
make ci
```
