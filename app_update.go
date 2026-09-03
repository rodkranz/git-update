package main

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	if _, ok := msg.(spinner.TickMsg); ok {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case scanDoneMsg:
		m.scanning = false
		m.scanErr = msg.err
		m.repos = msg.repos
		m.queue = nil
		m.active = 0
		m.cursor = 0
		m.decisionIndex = -1
		m.globalLog = nil
		if msg.err == nil {
			m.appendGlobal(fmt.Sprintf("Scan complete: %d repositories", len(m.repos)))
			for i := range m.repos {
				if isSafeAutoUpdate(m.repos[i]) {
					m.enqueueJob(i, jobUpdateTarget)
				}
			}
			m.decisionIndex = m.findNextAttention(-1)
			cmds = append(cmds, m.startQueuedJobs()...)
		}

	case repoUpdatedMsg:
		if msg.index >= 0 && msg.index < len(m.repos) {
			m.repos[msg.index] = msg.repo
			m.appendGlobal(repoResultLine(msg.repo))
		}
		if m.active > 0 {
			m.active--
		}
		cmds = append(cmds, m.startQueuedJobs()...)

	case tea.KeyPressMsg:
		key := msg.String()

		if m.showHelp {
			switch key {
			case "?", "esc", "q":
				m.showHelp = false
			}
			return m, tea.Batch(cmds...)
		}

		if m.confirm != confirmNone {
			switch key {
			case "y", "Y", "enter":
				idx := m.confirmIndex
				m.confirm = confirmNone
				m.confirmIndex = -1
				if idx >= 0 && idx < len(m.repos) {
					m.enqueueJob(idx, jobDiscardAndUpdate)
					m.advanceDecision(idx)
					cmds = append(cmds, m.startQueuedJobs()...)
				}
			case "n", "N", "esc", "q":
				m.confirm = confirmNone
				m.confirmIndex = -1
			}
			return m, tea.Batch(cmds...)
		}

		if key == "?" {
			m.showHelp = true
			return m, tea.Batch(cmds...)
		}

		switch key {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.repos) {
				m.cursor++
			}
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			m.cursor = len(m.repos)
		case "r":
			if m.active == 0 && len(m.queue) == 0 {
				m.scanning = true
				m.scanErr = nil
				cmds = append(cmds, scanCmd(m.cfg))
			}
		case "s":
			if idx, ok := m.actionRepoIndex(); ok {
				repo := m.repos[idx]
				repo.State = StateSkipped
				repo.Message = "skipped by user"
				m.repos[idx] = repo
				m.appendGlobal("↷ " + repo.Name + " — SKIP")
				m.advanceDecision(idx)
			}
		case "m":
			if idx, ok := m.actionRepoIndex(); ok {
				repo := m.repos[idx]
				if !repo.InProgress && repo.TargetBranch != "" && repo.Branch != repo.TargetBranch {
					m.enqueueJob(idx, jobSwitchTargetKeepChanges)
					m.advanceDecision(idx)
					cmds = append(cmds, m.startQueuedJobs()...)
				}
			}
		case "p":
			if idx, ok := m.actionRepoIndex(); ok {
				repo := m.repos[idx]
				if !repo.InProgress && canPullCurrent(repo) {
					m.enqueueJob(idx, jobPullCurrent)
					m.advanceDecision(idx)
					cmds = append(cmds, m.startQueuedJobs()...)
				}
			}
		case "d":
			if idx, ok := m.actionRepoIndex(); ok {
				repo := m.repos[idx]
				if !repo.InProgress && repo.TargetBranch != "" && len(repo.Changes) > 0 {
					m.confirm = confirmDiscardSelected
					m.confirmIndex = idx
				}
			}
		}
	}
	return m, tea.Batch(cmds...)
}

func isSafeAutoUpdate(repo Repo) bool {
	return repo.State == StateReady && !repo.InProgress && repo.TargetBranch != "" && repo.Branch == repo.TargetBranch && len(repo.Changes) == 0
}

func canPullCurrent(repo Repo) bool {
	return repo.Branch != "" && repo.Branch != "DETACHED HEAD"
}

func (m model) selectedRepoIndex() (int, bool) {
	if m.cursor <= 0 || m.cursor > len(m.repos) {
		return 0, false
	}
	return m.cursor - 1, true
}

func (m model) actionRepoIndex() (int, bool) {
	if idx, ok := m.selectedRepoIndex(); ok {
		return idx, m.repos[idx].State != StateUpdating
	}
	if m.cursor == 0 && m.decisionIndex >= 0 && m.decisionIndex < len(m.repos) && m.repos[m.decisionIndex].State == StateAttention {
		return m.decisionIndex, true
	}
	return 0, false
}

func (m *model) enqueueJob(index int, action jobAction) {
	if index < 0 || index >= len(m.repos) {
		return
	}
	repo := m.repos[index]
	repo.State = StateUpdating
	repo.Message = queuedMessage(action, repo.TargetBranch, repo.Branch)
	m.repos[index] = repo
	m.queue = append(m.queue, repoJob{index: index, action: action})
}

func queuedMessage(action jobAction, targetBranch, currentBranch string) string {
	switch action {
	case jobSwitchTargetKeepChanges:
		return fmt.Sprintf("queued: switch to %s and update", targetBranch)
	case jobPullCurrent:
		return fmt.Sprintf("queued: pull %s", currentBranch)
	case jobDiscardAndUpdate:
		return fmt.Sprintf("queued: discard changes, switch to %s and update", targetBranch)
	default:
		return fmt.Sprintf("queued: automatic update of %s", targetBranch)
	}
}

func runningMessage(action jobAction, targetBranch, currentBranch string) string {
	switch action {
	case jobSwitchTargetKeepChanges:
		return fmt.Sprintf("switching to %s and updating...", targetBranch)
	case jobPullCurrent:
		return fmt.Sprintf("pulling %s...", currentBranch)
	case jobDiscardAndUpdate:
		return fmt.Sprintf("discarding changes, switching to %s and updating...", targetBranch)
	default:
		return fmt.Sprintf("updating %s...", targetBranch)
	}
}

func (m *model) startQueuedJobs() []tea.Cmd {
	var cmds []tea.Cmd
	for m.active < m.cfg.Workers && len(m.queue) > 0 {
		job := m.queue[0]
		m.queue = m.queue[1:]
		if job.index < 0 || job.index >= len(m.repos) {
			continue
		}
		repo := m.repos[job.index]
		if repo.State != StateUpdating {
			continue
		}
		repo.Message = runningMessage(job.action, repo.TargetBranch, repo.Branch)
		m.repos[job.index] = repo
		m.appendGlobal("▶ " + repo.Name + " — " + repo.Message)
		m.active++
		cmds = append(cmds, repoJobCmd(job, repo, m.cfg))
	}
	return cmds
}

func (m *model) findNextAttention(after int) int {
	if len(m.repos) == 0 {
		return -1
	}
	for offset := 1; offset <= len(m.repos); offset++ {
		idx := (after + offset) % len(m.repos)
		if m.repos[idx].State == StateAttention {
			return idx
		}
	}
	return -1
}

func (m *model) advanceDecision(after int) {
	if m.decisionIndex == after || m.decisionIndex < 0 || m.decisionIndex >= len(m.repos) || m.repos[m.decisionIndex].State != StateAttention {
		m.decisionIndex = m.findNextAttention(after)
	}
}

func (m *model) appendGlobal(line string) {
	if strings.TrimSpace(line) == "" {
		return
	}
	m.globalLog = append(m.globalLog, line)
	const maxGlobalEntries = 250
	if len(m.globalLog) > maxGlobalEntries {
		m.globalLog = append([]string(nil), m.globalLog[len(m.globalLog)-maxGlobalEntries:]...)
	}
}

func repoResultLine(repo Repo) string {
	icon := "✓"
	switch repo.State {
	case StateFailed:
		icon = "✗"
	case StateSkipped:
		icon = "↷"
	}
	message := repo.Message
	if message == "" {
		message = stateLabel(repo.State)
	}
	return fmt.Sprintf("%s %s — %s", icon, repo.Name, message)
}
