package main

import (
	"os"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func newModel(cfg Config, sp spinner.Model) model {
	sp.Style = lipgloss.NewStyle().Foreground(accent2)
	return model{
		cfg:           cfg,
		spinner:       sp,
		scanning:      true,
		decisionIndex: -1,
		confirmIndex:  -1,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, scanCmd(m.cfg))
}

func scanCmd(cfg Config) tea.Cmd {
	return func() tea.Msg {
		paths, err := discoverRepos(cfg.Root)
		if err != nil {
			return scanDoneMsg{err: err}
		}
		repos := make([]Repo, len(paths))
		for i, path := range paths {
			repos[i] = inspectRepo(path, cfg.Branch)
		}
		return scanDoneMsg{repos: repos}
	}
}

func repoJobCmd(job repoJob, repo Repo, cfg Config) tea.Cmd {
	return func() tea.Msg {
		var updated Repo
		switch job.action {
		case jobSwitchTargetKeepChanges:
			updated = updateRepo(repo, cfg, true, true)
		case jobPullCurrent:
			updated = pullCurrentRepo(repo, cfg, true)
		case jobDiscardAndUpdate:
			updated = discardAndUpdateRepo(repo, cfg)
		default:
			updated = updateRepo(repo, cfg, false, false)
		}
		return repoUpdatedMsg{index: job.index, repo: updated}
	}
}

func shellCmd(index int, repo Repo) tea.Cmd {
	cmd := nativeShellCommand(repo)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return shellFinishedMsg{index: index, err: err}
	})
}

func nativeShellCommand(repo Repo) *exec.Cmd {
	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell) //nolint:gosec // The shell is intentionally selected from the user's SHELL environment variable.
	cmd.Dir = repo.Path
	return cmd
}
