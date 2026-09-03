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

func TestProjectModeMovesToNextAttention(t *testing.T) {
	m := model{
		repos: []Repo{
			{State: StateUpdating},
			{State: StateAttention},
			{State: StateUpdated},
			{State: StateAttention},
		},
		cursor:        1, // repo index 0; 0 itself is All repositories
		decisionIndex: 0,
	}
	m.advanceDecision(0)
	if m.cursor != 2 {
		t.Fatalf("cursor = %d, want list item 2 (repo index 1)", m.cursor)
	}
	if m.decisionIndex != 1 {
		t.Fatalf("decisionIndex = %d, want 1", m.decisionIndex)
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
	for _, want := range []string{"Keyboard shortcuts", "Switch to default branch & update", "Pull current branch", "SKIP repository", "Open / close shortcuts"} {
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
