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

func TestAutoUpdateClassification(t *testing.T) {
	cleanTarget := Repo{Branch: "master", State: StateReady}
	if !isSafeAutoUpdate(cleanTarget, "master") {
		t.Fatal("clean target branch should auto-update")
	}
	if isSafeAutoUpdate(Repo{Branch: "feature/test", State: StateAttention}, "master") {
		t.Fatal("non-target branch must require a decision")
	}
	if isSafeAutoUpdate(Repo{Branch: "master", State: StateAttention, Changes: []string{" M file.go"}}, "master") {
		t.Fatal("dirty target branch must require a decision")
	}
}

func TestAdvanceToNextAttention(t *testing.T) {
	m := model{repos: []Repo{
		{State: StateUpdating},
		{State: StateAttention},
		{State: StateUpdated},
		{State: StateAttention},
	}}
	m.cursor = 1
	m.repos[1].State = StateUpdating
	m.advanceToNextAttention(1)
	if m.cursor != 3 {
		t.Fatalf("cursor = %d, want 3", m.cursor)
	}
}
