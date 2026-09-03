package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseConfig(t *testing.T) {
	root := t.TempDir()
	cfg, err := parseConfig([]string{root, "--dry-run", "--branch", "main", "--workers", "8"})
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	wantRoot, _ := filepath.Abs(root)
	if cfg.Root != wantRoot || cfg.Branch != "main" || cfg.Workers != 8 || !cfg.DryRun {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if _, err := parseConfig([]string{"--workers", "0"}); err == nil {
		t.Fatal("expected invalid workers error")
	}
}

func TestDiscoverReposFindsOnlyRootRepositories(t *testing.T) {
	root := t.TempDir()
	repoA := filepath.Join(root, "a-service")
	repoB := filepath.Join(root, "group", "b-service")
	mustMkdir(t, filepath.Join(repoA, ".git"))
	mustMkdir(t, filepath.Join(repoA, "nested", ".git"))
	mustMkdir(t, filepath.Join(repoB, ".git"))
	mustMkdir(t, filepath.Join(root, "submodule-like"))
	mustWriteFile(t, filepath.Join(root, "submodule-like", ".git"), "gitdir: ../.git/modules/submodule-like\n")
	mustMkdir(t, filepath.Join(root, "node_modules", "ignored", ".git"))

	got, err := discoverRepos(root)
	if err != nil {
		t.Fatalf("discoverRepos() error = %v", err)
	}
	want := []string{repoA, repoB}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("discoverRepos() = %#v, want %#v", got, want)
	}
}

func TestInspectRepoCleanDirtyAndBranch(t *testing.T) {
	repo := newLocalRepo(t)
	clean := inspectRepo(repo, "master")
	if clean.State != StateReady || clean.Branch != "master" || len(clean.Changes) != 0 {
		t.Fatalf("unexpected clean state: %+v", clean)
	}

	mustWriteFile(t, filepath.Join(repo, "dirty.txt"), "dirty\n")
	dirty := inspectRepo(repo, "master")
	if dirty.State != StateAttention || len(dirty.Changes) != 1 {
		t.Fatalf("unexpected dirty state: %+v", dirty)
	}

	runGit(t, repo, "add", "dirty.txt")
	runGit(t, repo, "commit", "-m", "dirty")
	runGit(t, repo, "switch", "-c", "feature/test")
	other := inspectRepo(repo, "master")
	if other.State != StateAttention || other.Branch != "feature/test" {
		t.Fatalf("unexpected branch state: %+v", other)
	}
}

func TestUpdateRepoRequiresConfirmation(t *testing.T) {
	repo := newLocalRepo(t)
	mustWriteFile(t, filepath.Join(repo, "dirty.txt"), "dirty\n")
	state := inspectRepo(repo, "master")
	got := updateRepo(state, Config{Branch: "master"}, true, false)
	if got.State != StateSkipped || !strings.Contains(got.Message, "local-change confirmation") {
		t.Fatalf("unexpected result: %+v", got)
	}

	runGit(t, repo, "add", "dirty.txt")
	runGit(t, repo, "commit", "-m", "dirty")
	runGit(t, repo, "switch", "-c", "feature/test")
	state = inspectRepo(repo, "master")
	got = updateRepo(state, Config{Branch: "master"}, false, true)
	if got.State != StateSkipped || !strings.Contains(got.Message, "branch confirmation") {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestUpdateRepoPullsFastForward(t *testing.T) {
	_, work, upstream := newRemoteFixture(t)
	mustWriteFile(t, filepath.Join(upstream, "from-upstream.txt"), "hello\n")
	runGit(t, upstream, "add", "from-upstream.txt")
	runGit(t, upstream, "commit", "-m", "upstream change")
	runGit(t, upstream, "push", "origin", "master")

	got := updateRepo(inspectRepo(work, "master"), Config{Branch: "master"}, false, false)
	if got.State != StateUpdated {
		t.Fatalf("State = %v, want StateUpdated; message=%q log=%#v", got.State, got.Message, got.Log)
	}
	if _, err := os.Stat(filepath.Join(work, "from-upstream.txt")); err != nil {
		t.Fatalf("pulled file missing: %v", err)
	}
}

func TestUpdateRepoSwitchAndDryRun(t *testing.T) {
	_, work, _ := newRemoteFixture(t)
	runGit(t, work, "switch", "-c", "feature/test")
	got := updateRepo(inspectRepo(work, "master"), Config{Branch: "master"}, true, false)
	if got.State != StateUpdated {
		t.Fatalf("switch update failed: %+v", got)
	}
	if branch := strings.TrimSpace(runGit(t, work, "branch", "--show-current")); branch != "master" {
		t.Fatalf("branch = %q, want master", branch)
	}

	runGit(t, work, "switch", "feature/test")
	got = updateRepo(inspectRepo(work, "master"), Config{Branch: "master", DryRun: true}, true, false)
	if got.State != StateUpdated {
		t.Fatalf("dry run failed: %+v", got)
	}
	if branch := strings.TrimSpace(runGit(t, work, "branch", "--show-current")); branch != "feature/test" {
		t.Fatalf("dry-run changed branch to %q", branch)
	}
}

func TestDiscardAndUpdateRepoRemovesChangesButKeepsIgnoredFiles(t *testing.T) {
	_, work, _ := newRemoteFixture(t)

	mustWriteFile(t, filepath.Join(work, ".gitignore"), "ignored.tmp\n")
	runGit(t, work, "add", ".gitignore")
	runGit(t, work, "commit", "-m", "add ignore rule")

	mustWriteFile(t, filepath.Join(work, "README.md"), "modified\n")
	mustWriteFile(t, filepath.Join(work, "staged.txt"), "staged\n")
	runGit(t, work, "add", "staged.txt")
	mustWriteFile(t, filepath.Join(work, "untracked.txt"), "untracked\n")
	mustWriteFile(t, filepath.Join(work, "ignored.tmp"), "keep me\n")

	state := inspectRepo(work, "master")
	if len(state.Changes) != 3 {
		t.Fatalf("expected 3 visible changes, got %#v", state.Changes)
	}

	got := discardAndUpdateRepo(state, Config{Branch: "master"})
	if got.State != StateUpdated {
		t.Fatalf("discard and update failed: %+v", got)
	}
	if len(got.Changes) != 0 {
		t.Fatalf("changes remain after discard: %#v", got.Changes)
	}
	readme, err := os.ReadFile(filepath.Join(work, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(readme) != "initial\n" {
		t.Fatalf("README content = %q, want initial content", readme)
	}
	if _, err := os.Stat(filepath.Join(work, "staged.txt")); !os.IsNotExist(err) {
		t.Fatalf("staged file should have been removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(work, "untracked.txt")); !os.IsNotExist(err) {
		t.Fatalf("untracked file should have been removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(work, "ignored.tmp")); err != nil {
		t.Fatalf("ignored file should be kept: %v", err)
	}
}

func TestDiscardAndUpdateRepoDryRunDoesNotChangeFiles(t *testing.T) {
	_, work, _ := newRemoteFixture(t)
	mustWriteFile(t, filepath.Join(work, "README.md"), "modified\n")
	mustWriteFile(t, filepath.Join(work, "untracked.txt"), "untracked\n")

	got := discardAndUpdateRepo(inspectRepo(work, "master"), Config{Branch: "master", DryRun: true})
	if got.State != StateUpdated {
		t.Fatalf("dry-run discard failed: %+v", got)
	}
	readme, err := os.ReadFile(filepath.Join(work, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(readme) != "modified\n" {
		t.Fatalf("dry-run changed tracked file: %q", readme)
	}
	if _, err := os.Stat(filepath.Join(work, "untracked.txt")); err != nil {
		t.Fatalf("dry-run removed untracked file: %v", err)
	}
	if strings.Join(got.Log, "\n") == "" || !strings.Contains(strings.Join(got.Log, "\n"), "git reset --hard HEAD") {
		t.Fatalf("dry-run log does not show discard commands: %#v", got.Log)
	}
}

func TestInspectRepoDetectsOperationInProgress(t *testing.T) {
	repo := newLocalRepo(t)
	gitDir := strings.TrimSpace(runGit(t, repo, "rev-parse", "--git-dir"))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repo, gitDir)
	}
	mustWriteFile(t, filepath.Join(gitDir, "MERGE_HEAD"), "deadbeef\n")
	state := inspectRepo(repo, "master")
	if !state.InProgress || state.State != StateAttention {
		t.Fatalf("operation not detected: %+v", state)
	}
}

func newLocalRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	mustMkdir(t, repo)
	runGit(t, repo, "init", "--initial-branch=master")
	configureGitIdentity(t, repo)
	mustWriteFile(t, filepath.Join(repo, "README.md"), "initial\n")
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "initial")
	return repo
}

func newRemoteFixture(t *testing.T) (remote, work, upstream string) {
	t.Helper()
	base := t.TempDir()
	remote = filepath.Join(base, "remote.git")
	seed := filepath.Join(base, "seed")
	work = filepath.Join(base, "work")
	upstream = filepath.Join(base, "upstream")
	mustMkdir(t, remote)
	runGit(t, remote, "init", "--bare", "--initial-branch=master")
	mustMkdir(t, seed)
	runGit(t, seed, "init", "--initial-branch=master")
	configureGitIdentity(t, seed)
	mustWriteFile(t, filepath.Join(seed, "README.md"), "initial\n")
	runGit(t, seed, "add", "README.md")
	runGit(t, seed, "commit", "-m", "initial")
	runGit(t, seed, "remote", "add", "origin", remote)
	runGit(t, seed, "push", "-u", "origin", "master")
	runGit(t, base, "clone", remote, work)
	runGit(t, base, "clone", remote, upstream)
	configureGitIdentity(t, work)
	configureGitIdentity(t, upstream)
	return remote, work, upstream
}

func configureGitIdentity(t *testing.T, repo string) {
	t.Helper()
	runGit(t, repo, "config", "user.name", "git-update tests")
	runGit(t, repo, "config", "user.email", "git-update-tests@example.invalid")
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s (dir %s): %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return string(out)
}
