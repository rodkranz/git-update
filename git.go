package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type RepoState int

const (
	StateReady RepoState = iota
	StateAttention
	StateUpdating
	StateUpdated
	StateSkipped
	StateFailed
)

type Repo struct {
	Path         string
	Name         string
	Branch       string
	TargetBranch string
	Changes      []string
	InProgress   bool
	State        RepoState
	Message      string
	Log          []string
}

func inspectRepo(path, branchOverride string) Repo {
	repo := Repo{Path: path, Name: filepath.Base(path), State: StateReady}
	branch, err := gitOutput(path, "branch", "--show-current")
	if err != nil {
		repo.State = StateFailed
		repo.Message = err.Error()
		return repo
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		branch = "DETACHED HEAD"
	}
	repo.Branch = branch
	repo.TargetBranch = detectTargetBranch(path, branchOverride, branch)

	status, err := gitOutput(path, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		repo.State = StateFailed
		repo.Message = err.Error()
		return repo
	}
	for _, line := range strings.Split(status, "\n") {
		if strings.TrimSpace(line) != "" {
			repo.Changes = append(repo.Changes, line)
		}
	}

	repo.InProgress = inProgressOperation(path)
	if repo.InProgress {
		repo.State = StateAttention
		repo.Message = "merge/rebase/cherry-pick/revert in progress"
	} else if repo.TargetBranch == "" {
		repo.State = StateAttention
		repo.Message = "default branch could not be detected; pull the current branch, SKIP, or use --branch"
	} else if repo.Branch != repo.TargetBranch || len(repo.Changes) > 0 {
		repo.State = StateAttention
		switch {
		case repo.Branch != repo.TargetBranch && len(repo.Changes) > 0:
			repo.Message = "choose how to handle the current branch and local changes"
		case repo.Branch != repo.TargetBranch:
			repo.Message = "choose whether to update the default branch or pull the current branch"
		default:
			repo.Message = "choose how to handle local changes before pulling"
		}
	} else {
		repo.Message = "ready for automatic update"
	}
	return repo
}

func detectTargetBranch(path, override, currentBranch string) string {
	if override = strings.TrimSpace(override); override != "" {
		return override
	}

	if head, err := gitOutput(path, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD"); err == nil {
		head = strings.TrimSpace(head)
		if strings.HasPrefix(head, "origin/") {
			if branch := strings.TrimPrefix(head, "origin/"); branch != "" && branch != "HEAD" {
				return branch
			}
		}
	}

	remoteMain := gitSuccess(path, "show-ref", "--verify", "--quiet", "refs/remotes/origin/main")
	remoteMaster := gitSuccess(path, "show-ref", "--verify", "--quiet", "refs/remotes/origin/master")
	if remoteMain != remoteMaster {
		if remoteMain {
			return "main"
		}
		return "master"
	}
	if remoteMain && remoteMaster {
		if currentBranch == "main" || currentBranch == "master" {
			return currentBranch
		}
		return ""
	}

	localMain := gitSuccess(path, "show-ref", "--verify", "--quiet", "refs/heads/main")
	localMaster := gitSuccess(path, "show-ref", "--verify", "--quiet", "refs/heads/master")
	if localMain != localMaster {
		if localMain {
			return "main"
		}
		return "master"
	}
	if localMain && localMaster {
		if currentBranch == "main" || currentBranch == "master" {
			return currentBranch
		}
		return ""
	}

	if currentBranch == "main" || currentBranch == "master" {
		return currentBranch
	}
	return ""
}

func updateRepo(repo Repo, cfg Config, allowSwitch, allowDirty bool) Repo {
	repo.State = StateUpdating
	repo.Log = nil
	if repo.InProgress {
		repo.State = StateSkipped
		repo.Message = "operation in progress; skipped"
		return repo
	}
	if repo.TargetBranch == "" {
		repo.State = StateFailed
		repo.Message = "default branch is unknown"
		return repo
	}
	if repo.Branch != repo.TargetBranch && !allowSwitch {
		repo.State = StateSkipped
		repo.Message = "requires branch confirmation"
		return repo
	}
	if len(repo.Changes) > 0 && !allowDirty {
		repo.State = StateSkipped
		repo.Message = "requires local-change confirmation"
		return repo
	}

	target := repo.TargetBranch
	if repo.Branch != target {
		if cfg.DryRun {
			repo.Log = append(repo.Log, "$ git switch "+target+"  # dry-run")
		} else {
			repo.Log = append(repo.Log, "$ git switch "+target)
			out, err := switchToTarget(repo.Path, target)
			if strings.TrimSpace(out) != "" {
				repo.Log = append(repo.Log, splitLines(out)...)
			}
			if err != nil {
				repo.State = StateFailed
				repo.Message = "switch failed: " + err.Error()
				return repo
			}
			repo.Branch = target
		}
	}

	if cfg.DryRun {
		repo.Log = append(repo.Log, "$ git pull --ff-only origin "+target+"  # dry-run")
		repo.State = StateUpdated
		repo.Message = "dry-run complete"
		return repo
	}

	repo.Log = append(repo.Log, "$ git pull --ff-only origin "+target)
	out, err := gitCombinedOutput(repo.Path, "pull", "--ff-only", "origin", target)
	if strings.TrimSpace(out) != "" {
		repo.Log = append(repo.Log, splitLines(out)...)
	}
	if err != nil {
		repo.State = StateFailed
		repo.Message = "pull failed: " + err.Error()
		return repo
	}
	fresh := inspectRepo(repo.Path, cfg.Branch)
	fresh.Log = repo.Log
	fresh.State = StateUpdated
	fresh.Message = lastMeaningfulLine(out, target+" updated")
	return fresh
}

func pullCurrentRepo(repo Repo, cfg Config, allowDirty bool) Repo {
	repo.State = StateUpdating
	repo.Log = nil
	if repo.InProgress {
		repo.State = StateSkipped
		repo.Message = "operation in progress; skipped"
		return repo
	}
	if repo.Branch == "" || repo.Branch == "DETACHED HEAD" {
		repo.State = StateFailed
		repo.Message = "cannot pull a detached HEAD"
		return repo
	}
	if len(repo.Changes) > 0 && !allowDirty {
		repo.State = StateSkipped
		repo.Message = "requires local-change confirmation"
		return repo
	}

	if cfg.DryRun {
		repo.Log = append(repo.Log, "$ git pull --ff-only origin "+repo.Branch+"  # dry-run")
		repo.State = StateUpdated
		repo.Message = "dry-run complete"
		return repo
	}

	branch := repo.Branch
	repo.Log = append(repo.Log, "$ git pull --ff-only origin "+branch)
	out, err := gitCombinedOutput(repo.Path, "pull", "--ff-only", "origin", branch)
	if strings.TrimSpace(out) != "" {
		repo.Log = append(repo.Log, splitLines(out)...)
	}
	if err != nil {
		repo.State = StateFailed
		repo.Message = "pull failed: " + err.Error()
		return repo
	}
	fresh := inspectRepo(repo.Path, cfg.Branch)
	fresh.Log = repo.Log
	fresh.State = StateUpdated
	fresh.Message = lastMeaningfulLine(out, branch+" updated")
	return fresh
}

func discardAndUpdateRepo(repo Repo, cfg Config) Repo {
	repo.State = StateUpdating
	repo.Log = nil
	if repo.InProgress {
		repo.State = StateSkipped
		repo.Message = "operation in progress; discard blocked"
		return repo
	}
	if repo.TargetBranch == "" {
		repo.State = StateFailed
		repo.Message = "default branch is unknown; discard blocked"
		return repo
	}

	if len(repo.Changes) == 0 {
		return updateRepo(repo, cfg, true, false)
	}

	discardLog := []string{
		"$ git reset --hard HEAD",
		"$ git clean -ffd",
	}

	if cfg.DryRun {
		discardLog[0] += "  # dry-run"
		discardLog[1] += "  # dry-run"
		cleaned := repo
		cleaned.Changes = nil
		updated := updateRepo(cleaned, cfg, true, false)
		updated.Log = append(discardLog, updated.Log...)
		return updated
	}

	out, err := gitCombinedOutput(repo.Path, "reset", "--hard", "HEAD")
	if strings.TrimSpace(out) != "" {
		discardLog = append(discardLog, splitLines(out)...)
	}
	if err != nil {
		repo.State = StateFailed
		repo.Message = "discard failed during reset: " + err.Error()
		repo.Log = discardLog
		return repo
	}

	out, err = gitCombinedOutput(repo.Path, "clean", "-ffd")
	if strings.TrimSpace(out) != "" {
		discardLog = append(discardLog, splitLines(out)...)
	}
	if err != nil {
		repo.State = StateFailed
		repo.Message = "discard failed during clean: " + err.Error()
		repo.Log = discardLog
		return repo
	}

	fresh := inspectRepo(repo.Path, cfg.Branch)
	if fresh.State == StateFailed {
		fresh.Log = discardLog
		return fresh
	}
	updated := updateRepo(fresh, cfg, true, false)
	updated.Log = append(discardLog, updated.Log...)
	return updated
}

func switchToTarget(path, branch string) (string, error) {
	if gitSuccess(path, "show-ref", "--verify", "--quiet", "refs/heads/"+branch) {
		return gitCombinedOutput(path, "switch", branch)
	}
	if gitSuccess(path, "show-ref", "--verify", "--quiet", "refs/remotes/origin/"+branch) {
		return gitCombinedOutput(path, "switch", "--track", "-c", branch, "origin/"+branch)
	}
	return "", fmt.Errorf("neither local %q nor origin/%s exists", branch, branch)
}

func inProgressOperation(path string) bool {
	gitDir, err := gitOutput(path, "rev-parse", "--git-dir")
	if err != nil {
		return false
	}
	gitDir = strings.TrimSpace(gitDir)
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(path, gitDir)
	}
	for _, marker := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD", "rebase-merge", "rebase-apply"} {
		if _, err := os.Stat(filepath.Join(gitDir, marker)); err == nil {
			return true
		}
	}
	return false
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", err
	}
	return string(out), nil
}

func gitCombinedOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return string(out), errors.New(msg)
	}
	return string(out), nil
}

func gitSuccess(dir string, args ...string) bool {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Run() == nil
}

func splitLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func lastMeaningfulLine(s, fallback string) string {
	lines := splitLines(s)
	if len(lines) == 0 {
		return fallback
	}
	return lines[len(lines)-1]
}
