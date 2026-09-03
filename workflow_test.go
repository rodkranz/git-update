package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPullCurrentRepoKeepsFeatureBranch(t *testing.T) {
	_, work, upstream := newRemoteFixture(t)
	runGit(t, work, "switch", "-c", "feature/test")
	runGit(t, work, "push", "-u", "origin", "feature/test")
	runGit(t, upstream, "fetch", "origin")
	runGit(t, upstream, "switch", "--track", "-c", "feature/test", "origin/feature/test")
	mustWriteFile(t, filepath.Join(upstream, "feature.txt"), "remote\n")
	runGit(t, upstream, "add", "feature.txt")
	runGit(t, upstream, "commit", "-m", "feature update")
	runGit(t, upstream, "push", "origin", "feature/test")

	got := pullCurrentRepo(inspectRepo(work, "master"), Config{Branch: "master"}, true)
	if got.State != StateUpdated {
		t.Fatalf("unexpected state: %+v", got)
	}
	if branch := strings.TrimSpace(runGit(t, work, "branch", "--show-current")); branch != "feature/test" {
		t.Fatalf("branch = %q, want feature/test", branch)
	}
	if _, err := os.Stat(filepath.Join(work, "feature.txt")); err != nil {
		t.Fatalf("pulled feature file missing: %v", err)
	}
}

func TestAutoUpdateClassificationUsesPerRepoTarget(t *testing.T) {
	mainRepo := Repo{Branch: "main", TargetBranch: "main", State: StateReady}
	masterRepo := Repo{Branch: "master", TargetBranch: "master", State: StateReady}
	if !isSafeAutoUpdate(mainRepo) {
		t.Fatal("clean main repository should auto-update")
	}
	if !isSafeAutoUpdate(masterRepo) {
		t.Fatal("clean master repository should auto-update")
	}
	if isSafeAutoUpdate(Repo{Branch: "feature/test", TargetBranch: "main", State: StateAttention}) {
		t.Fatal("non-target branch must require a decision")
	}
	if isSafeAutoUpdate(Repo{Branch: "main", TargetBranch: "main", State: StateAttention, Changes: []string{" M file.go"}}) {
		t.Fatal("dirty target branch must require a decision")
	}
	if isSafeAutoUpdate(Repo{Branch: "main", State: StateReady}) {
		t.Fatal("repository with unknown target branch must not auto-update")
	}
}

func TestAllModeKeepsGlobalSelectionWhileAdvancingDecisions(t *testing.T) {
	m := model{
		repos: []Repo{
			{State: StateUpdating},
			{State: StateAttention},
			{State: StateUpdated},
			{State: StateAttention},
		},
		cursor:        0,
		decisionIndex: 0,
	}
	m.advanceDecision(0)
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want All repositories (0)", m.cursor)
	}
	if m.decisionIndex != 1 {
		t.Fatalf("decisionIndex = %d, want 1", m.decisionIndex)
	}
}

func TestProjectModeKeepsManualSelectionWhileDecisionAdvances(t *testing.T) {
	m := model{
		repos: []Repo{
			{State: StateUpdating},
			{State: StateAttention},
			{State: StateUpdated},
			{State: StateAttention},
		},
		cursor:        1,
		decisionIndex: 0,
	}
	m.advanceDecision(0)
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want selected repository to remain selected", m.cursor)
	}
	if m.decisionIndex != 1 {
		t.Fatalf("decisionIndex = %d, want 1", m.decisionIndex)
	}
}

func TestSelectedSkippedRepoAllowsManualActions(t *testing.T) {
	m := model{
		repos: []Repo{{
			State:        StateSkipped,
			Branch:       "feature/test",
			TargetBranch: "main",
			Changes:      []string{"?? scratch.txt"},
		}},
		cursor: 1,
	}
	idx, ok := m.actionRepoIndex()
	if !ok {
		t.Fatal("manually selected skipped repository should accept valid actions")
	}
	if idx != 0 {
		t.Fatalf("index = %d, want 0", idx)
	}
}

func TestDiscardAndUpdateRemovesNestedUntrackedGitRepository(t *testing.T) {
	_, work, _ := newRemoteFixtureWithBranch(t, "main")
	nested := filepath.Join(work, ".claude", "worktrees", "scratch")
	mustMkdir(t, nested)
	runGit(t, nested, "init", "--initial-branch=main")
	mustWriteFile(t, filepath.Join(nested, "scratch.txt"), "temporary\n")

	repo := inspectRepo(work, "")
	if len(repo.Changes) == 0 {
		t.Fatal("expected nested repository to appear as an untracked local change")
	}

	got := discardAndUpdateRepo(repo, Config{})
	if got.State != StateUpdated {
		t.Fatalf("unexpected state after discard: %+v", got)
	}
	if len(got.Changes) != 0 {
		t.Fatalf("local changes remain after discard: %v", got.Changes)
	}
	if _, err := os.Stat(nested); !os.IsNotExist(err) {
		t.Fatalf("nested untracked Git repository still exists after discard; err=%v", err)
	}
}

func TestDefaultBranchDetectionSupportsMainAndMaster(t *testing.T) {
	_, masterWork, _ := newRemoteFixture(t)
	masterRepo := inspectRepo(masterWork, "")
	if masterRepo.TargetBranch != "master" {
		t.Fatalf("master TargetBranch = %q, want master", masterRepo.TargetBranch)
	}
	if !isSafeAutoUpdate(masterRepo) {
		t.Fatalf("master repo should be safe for automatic update: %+v", masterRepo)
	}

	_, mainWork, _ := newRemoteFixtureWithBranch(t, "main")
	mainRepo := inspectRepo(mainWork, "")
	if mainRepo.TargetBranch != "main" {
		t.Fatalf("main TargetBranch = %q, want main", mainRepo.TargetBranch)
	}
	if !isSafeAutoUpdate(mainRepo) {
		t.Fatalf("main repo should be safe for automatic update: %+v", mainRepo)
	}
}

func TestBranchOverrideWinsOverDetectedDefault(t *testing.T) {
	_, work, _ := newRemoteFixtureWithBranch(t, "main")
	repo := inspectRepo(work, "release")
	if repo.TargetBranch != "release" {
		t.Fatalf("TargetBranch = %q, want explicit override release", repo.TargetBranch)
	}
	if repo.State != StateAttention {
		t.Fatalf("explicit non-current override should require attention: %+v", repo)
	}
}

func TestParseConfigDefaultsToPerRepoBranchDetection(t *testing.T) {
	cfg, err := parseConfig([]string{t.TempDir()})
	if err != nil {
		t.Fatalf("parseConfig() error = %v", err)
	}
	if cfg.Branch != "" {
		t.Fatalf("Branch = %q, want empty auto-detect override", cfg.Branch)
	}
}

func TestNativeShellCommandUsesUserShellAndRepositoryDirectory(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	cmd := nativeShellCommand(Repo{Path: "/tmp/example-repository"})
	if cmd.Args[0] != "/bin/sh" {
		t.Fatalf("shell = %q, want /bin/sh", cmd.Args[0])
	}
	if cmd.Dir != "/tmp/example-repository" {
		t.Fatalf("Dir = %q, want repository path", cmd.Dir)
	}
}

func TestShellFinishedRefreshesRepositoryState(t *testing.T) {
	_, work, _ := newRemoteFixtureWithBranch(t, "main")
	stale := inspectRepo(work, "")
	mustWriteFile(t, filepath.Join(work, "after-shell.txt"), "created in shell\n")

	m := model{
		cfg:           Config{},
		repos:         []Repo{stale},
		cursor:        1,
		decisionIndex: -1,
	}
	updatedModel, _ := m.Update(shellFinishedMsg{index: 0})
	got := updatedModel.(model)
	if got.repos[0].State != StateAttention {
		t.Fatalf("state = %v, want needs attention after shell-created file", got.repos[0].State)
	}
	if len(got.repos[0].Changes) == 0 {
		t.Fatal("repository was not refreshed after shell exit")
	}
	if !strings.Contains(strings.Join(got.repos[0].Log, "\n"), "repository refreshed") {
		t.Fatalf("activity does not mention refresh: %v", got.repos[0].Log)
	}
}

func TestSelectedRepositoryFooterShowsShellShortcut(t *testing.T) {
	m := model{
		repos: []Repo{{
			State:        StateFailed,
			Branch:       "feature/test",
			TargetBranch: "main",
		}},
		cursor: 1,
	}
	footer := m.renderFooter(220)
	if !strings.Contains(footer, "t shell here") {
		t.Fatalf("selected repository footer does not show shell shortcut: %q", footer)
	}

	m.cursor = 0
	m.decisionIndex = 0
	footer = m.renderFooter(220)
	if strings.Contains(footer, "t shell here") {
		t.Fatalf("All repositories footer should not show shell shortcut: %q", footer)
	}
}

func TestShellConfirmationExplainsExitAndRefresh(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	m := model{
		repos:        []Repo{{Name: "example", Path: "/tmp/example", State: StateAttention}},
		confirm:      confirmShellSelected,
		confirmIndex: 0,
	}
	modal := m.renderConfirm(120, 40)
	for _, want := range []string{"Open shell", "/bin/sh", "Type \"exit\" to return", "refreshed automatically"} {
		if !strings.Contains(modal, want) {
			t.Fatalf("shell confirmation missing %q", want)
		}
	}
}

func TestFooterShowsShortcutsShortcut(t *testing.T) {
	m := model{}
	footer := m.renderFooter(160)
	if !strings.Contains(footer, "? shortcuts") {
		t.Fatalf("footer does not advertise shortcuts help: %q", footer)
	}
}

func TestHelpModalListsKeyboardShortcuts(t *testing.T) {
	m := model{}
	help := m.renderHelp(100, 35)
	for _, want := range []string{"Keyboard shortcuts", "Switch to default branch & update", "Pull current branch", "Open native shell in selected repository", "SKIP repository", "Open / close shortcuts"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help modal missing %q", want)
		}
	}
}

func newRemoteFixtureWithBranch(t *testing.T, branch string) (remote, work, upstream string) {
	t.Helper()
	base := t.TempDir()
	remote = filepath.Join(base, "remote.git")
	seed := filepath.Join(base, "seed")
	work = filepath.Join(base, "work")
	upstream = filepath.Join(base, "upstream")
	mustMkdir(t, remote)
	runGit(t, remote, "init", "--bare", "--initial-branch="+branch)
	mustMkdir(t, seed)
	runGit(t, seed, "init", "--initial-branch="+branch)
	configureGitIdentity(t, seed)
	mustWriteFile(t, filepath.Join(seed, "README.md"), "initial\n")
	runGit(t, seed, "add", "README.md")
	runGit(t, seed, "commit", "-m", "initial")
	runGit(t, seed, "remote", "add", "origin", remote)
	runGit(t, seed, "push", "-u", "origin", branch)
	runGit(t, base, "clone", remote, work)
	runGit(t, base, "clone", remote, upstream)
	configureGitIdentity(t, work)
	configureGitIdentity(t, upstream)
	return remote, work, upstream
}
