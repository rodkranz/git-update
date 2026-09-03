package main

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type scanDoneMsg struct {
	repos []Repo
	err   error
}

type repoUpdatedMsg struct {
	index int
	repo  Repo
}

type jobAction int

const (
	jobUpdateTarget jobAction = iota
	jobSwitchTargetKeepChanges
	jobPullCurrent
	jobDiscardAndUpdate
)

type repoJob struct {
	index  int
	action jobAction
}

type confirmMode int

const (
	confirmNone confirmMode = iota
	confirmDiscardSelected
)

type model struct {
	cfg      Config
	spinner  spinner.Model
	width    int
	height   int
	repos    []Repo
	cursor   int
	scanning bool
	scanErr  error

	confirm      confirmMode
	confirmIndex int

	queue  []repoJob
	active int
}

var (
	accent        = lipgloss.Color("#7D56F4")
	accent2       = lipgloss.Color("#5FD7FF")
	green         = lipgloss.Color("#73DACA")
	yellow        = lipgloss.Color("#E0AF68")
	red           = lipgloss.Color("#F7768E")
	muted         = lipgloss.Color("#737DAA")
	text          = lipgloss.Color("#C0CAF5")
	border        = lipgloss.Color("#414868")
	panelStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).Padding(0, 1)
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(accent2)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")).Background(accent).Padding(0, 1)
)

func newModel(cfg Config, sp spinner.Model) model {
	sp.Style = lipgloss.NewStyle().Foreground(accent2)
	return model{cfg: cfg, spinner: sp, scanning: true, confirmIndex: -1}
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
		if msg.err == nil {
			for i := range m.repos {
				if isSafeAutoUpdate(m.repos[i], m.cfg.Branch) {
					m.enqueueJob(i, jobUpdateTarget)
				}
			}
			m.focusFirstAttention()
			cmds = append(cmds, m.startQueuedJobs()...)
		}

	case repoUpdatedMsg:
		if msg.index >= 0 && msg.index < len(m.repos) {
			m.repos[msg.index] = msg.repo
		}
		if m.active > 0 {
			m.active--
		}
		cmds = append(cmds, m.startQueuedJobs()...)

	case tea.KeyPressMsg:
		key := msg.String()
		if m.confirm != confirmNone {
			switch key {
			case "y", "Y", "enter":
				idx := m.confirmIndex
				m.confirm = confirmNone
				m.confirmIndex = -1
				if idx >= 0 && idx < len(m.repos) {
					m.enqueueJob(idx, jobDiscardAndUpdate)
					m.advanceToNextAttention(idx)
					cmds = append(cmds, m.startQueuedJobs()...)
				}
			case "n", "N", "esc", "q":
				m.confirm = confirmNone
				m.confirmIndex = -1
			}
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
			if m.cursor+1 < len(m.repos) {
				m.cursor++
			}
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			if len(m.repos) > 0 {
				m.cursor = len(m.repos) - 1
			}
		case "r":
			if m.active == 0 && len(m.queue) == 0 {
				m.scanning = true
				m.scanErr = nil
				cmds = append(cmds, scanCmd(m.cfg))
			}
		case "s":
			if m.selectedNeedsAttention() {
				idx := m.cursor
				repo := m.repos[idx]
				repo.State = StateSkipped
				repo.Message = "skipped by user"
				m.repos[idx] = repo
				m.advanceToNextAttention(idx)
			}
		case "m":
			if m.selectedNeedsAttention() {
				idx := m.cursor
				repo := m.repos[idx]
				if !repo.InProgress && repo.Branch != m.cfg.Branch {
					m.enqueueJob(idx, jobSwitchTargetKeepChanges)
					m.advanceToNextAttention(idx)
					cmds = append(cmds, m.startQueuedJobs()...)
				}
			}
		case "p":
			if m.selectedNeedsAttention() {
				idx := m.cursor
				repo := m.repos[idx]
				if !repo.InProgress && canPullCurrent(repo) {
					m.enqueueJob(idx, jobPullCurrent)
					m.advanceToNextAttention(idx)
					cmds = append(cmds, m.startQueuedJobs()...)
				}
			}
		case "d":
			if m.selectedNeedsAttention() {
				repo := m.repos[m.cursor]
				if !repo.InProgress && len(repo.Changes) > 0 {
					m.confirm = confirmDiscardSelected
					m.confirmIndex = m.cursor
				}
			}
		}
	}
	return m, tea.Batch(cmds...)
}

func isSafeAutoUpdate(repo Repo, targetBranch string) bool {
	return repo.State == StateReady && !repo.InProgress && repo.Branch == targetBranch && len(repo.Changes) == 0
}

func canPullCurrent(repo Repo) bool {
	return repo.Branch != "" && repo.Branch != "DETACHED HEAD"
}

func (m model) selectedNeedsAttention() bool {
	return m.cursor >= 0 && m.cursor < len(m.repos) && m.repos[m.cursor].State == StateAttention
}

func (m *model) enqueueJob(index int, action jobAction) {
	if index < 0 || index >= len(m.repos) {
		return
	}
	repo := m.repos[index]
	repo.State = StateUpdating
	repo.Message = queuedMessage(action, m.cfg.Branch, repo.Branch)
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
		return "queued: automatic update"
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
		return "updating..."
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
		repo.Message = runningMessage(job.action, m.cfg.Branch, repo.Branch)
		m.repos[job.index] = repo
		m.active++
		cmds = append(cmds, repoJobCmd(job, repo, m.cfg))
	}
	return cmds
}

func (m *model) focusFirstAttention() {
	for i := range m.repos {
		if m.repos[i].State == StateAttention {
			m.cursor = i
			return
		}
	}
	if m.cursor >= len(m.repos) {
		m.cursor = max(0, len(m.repos)-1)
	}
}

func (m *model) advanceToNextAttention(after int) {
	if len(m.repos) == 0 {
		return
	}
	for offset := 1; offset <= len(m.repos); offset++ {
		idx := (after + offset) % len(m.repos)
		if m.repos[idx].State == StateAttention {
			m.cursor = idx
			return
		}
	}
}

func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "git update"
	return v
}

func (m model) render() string {
	w := m.width
	if w <= 0 {
		w = 100
	}
	h := m.height
	if h <= 0 {
		h = 30
	}
	header := m.renderHeader(w)
	footer := m.renderFooter(w)
	bodyHeight := max(8, h-lipgloss.Height(header)-lipgloss.Height(footer)-1)

	if m.scanning {
		body := panelStyle.Width(max(30, w-2)).Height(max(5, bodyHeight-2)).Render(fmt.Sprintf("%s  Scanning repositories under\n\n%s", m.spinner.View(), m.cfg.Root))
		return header + "\n" + body + "\n" + footer
	}
	if m.scanErr != nil {
		body := panelStyle.Width(max(30, w-2)).Height(max(5, bodyHeight-2)).Render(lipgloss.NewStyle().Foreground(red).Render("Scan failed: " + m.scanErr.Error()))
		return header + "\n" + body + "\n" + footer
	}
	if len(m.repos) == 0 {
		body := panelStyle.Width(max(30, w-2)).Height(max(5, bodyHeight-2)).Render("No Git repositories found.")
		return header + "\n" + body + "\n" + footer
	}

	leftWidth := clamp(w/3, 28, 48)
	rightWidth := max(30, w-leftWidth-3)
	left := m.renderRepoList(leftWidth, bodyHeight)
	right := m.renderDetails(rightWidth, bodyHeight)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
	if m.confirm != confirmNone {
		body = m.renderConfirm(w, bodyHeight)
	}
	return header + "\n" + body + "\n" + footer
}

func (m model) renderHeader(width int) string {
	mode := ""
	if m.cfg.DryRun {
		mode = lipgloss.NewStyle().Bold(true).Foreground(yellow).Render("  DRY RUN")
	}
	left := titleStyle.Render("Git Update") + mode
	right := lipgloss.NewStyle().Foreground(muted).Render(fmt.Sprintf("target: %s   workers: %d", m.cfg.Branch, m.cfg.Workers))
	space := max(1, width-lipgloss.Width(left)-lipgloss.Width(right)-2)
	return " " + left + strings.Repeat(" ", space) + right
}

func (m model) renderRepoList(width, height int) string {
	innerHeight := max(3, height-2)
	visible := max(1, innerHeight-2)
	start := 0
	if m.cursor >= visible {
		start = m.cursor - visible + 1
	}
	end := min(len(m.repos), start+visible)
	lines := []string{titleStyle.Render(fmt.Sprintf("PROJECTS  %d", len(m.repos))), ""}
	for i := start; i < end; i++ {
		r := m.repos[i]
		prefix := stateIcon(r.State, m.spinner.View())
		branch := r.Branch
		if branch == "" {
			branch = "?"
		}
		nameWidth := max(8, width-18)
		line := fmt.Sprintf("%s %-*s %s", prefix, nameWidth, truncate(r.Name, nameWidth), truncate(branch, 12))
		if i == m.cursor {
			line = selectedStyle.Width(max(1, width-4)).Render(line)
		} else {
			line = lipgloss.NewStyle().Foreground(text).Render(line)
		}
		lines = append(lines, line)
	}
	return panelStyle.Width(width).Height(height).Render(strings.Join(lines, "\n"))
}

func (m model) renderDetails(width, height int) string {
	r := m.repos[m.cursor]
	lines := []string{
		titleStyle.Render(r.Name),
		"",
		labelValue("Status", stateLabel(r.State)),
		labelValue("Branch", r.Branch),
		labelValue("Target", m.cfg.Branch),
		labelValue("Path", r.Path),
	}
	if r.InProgress {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(red).Bold(true).Render("Git operation in progress"))
	}
	if len(r.Changes) > 0 {
		lines = append(lines, "", lipgloss.NewStyle().Bold(true).Foreground(accent2).Render(fmt.Sprintf("Local changes (%d)", len(r.Changes))))
		maxChanges := max(2, (height-18)/2)
		for i, change := range r.Changes {
			if i >= maxChanges {
				lines = append(lines, lipgloss.NewStyle().Foreground(muted).Render(fmt.Sprintf("… and %d more", len(r.Changes)-i)))
				break
			}
			lines = append(lines, lipgloss.NewStyle().Foreground(yellow).Render(change))
		}
	}

	if r.State == StateAttention {
		lines = append(lines, "", lipgloss.NewStyle().Bold(true).Foreground(accent2).Render("Choose an action"))
		lines = append(lines, m.renderActions(r))
	}

	lines = append(lines, "", lipgloss.NewStyle().Bold(true).Foreground(accent2).Render("Activity"))
	if r.Message != "" {
		lines = append(lines, stateMessageStyle(r.State).Render(r.Message))
	}
	maxLog := max(1, height-len(lines)-4)
	if len(r.Log) > 0 {
		start := max(0, len(r.Log)-maxLog)
		for _, line := range r.Log[start:] {
			lines = append(lines, lipgloss.NewStyle().Foreground(muted).Render(truncate(line, max(10, width-5))))
		}
	}
	return panelStyle.Width(width).Height(height).Render(strings.Join(lines, "\n"))
}

func (m model) renderActions(r Repo) string {
	if r.InProgress {
		return actionKey("s", "SKIP")
	}

	var actions []string
	wrongBranch := r.Branch != m.cfg.Branch
	dirty := len(r.Changes) > 0

	if wrongBranch {
		switchLabel := fmt.Sprintf("Switch to %s & Update", m.cfg.Branch)
		if dirty {
			switchLabel += " (keep changes)"
		}
		actions = append(actions, actionKey("m", switchLabel))
	}
	if canPullCurrent(r) {
		pullLabel := fmt.Sprintf("Pull %s", r.Branch)
		if dirty {
			pullLabel += " (keep changes)"
		}
		actions = append(actions, actionKey("p", pullLabel))
	}
	if dirty {
		discardLabel := "Discard changes & Update"
		if wrongBranch {
			discardLabel = fmt.Sprintf("Discard changes, switch to %s & Update", m.cfg.Branch)
		}
		actions = append(actions, actionKey("d", discardLabel))
	}
	actions = append(actions, actionKey("s", "SKIP"))
	return strings.Join(actions, "    ")
}

func (m model) renderFooter(width int) string {
	updated, attention, skipped, failed := 0, 0, 0, 0
	for _, r := range m.repos {
		switch r.State {
		case StateUpdated:
			updated++
		case StateAttention:
			attention++
		case StateSkipped:
			skipped++
		case StateFailed:
			failed++
		}
	}
	status := fmt.Sprintf(" ✓ %d updated   ! %d attention   ↷ %d skipped   ✗ %d failed", updated, attention, skipped, failed)
	if m.active > 0 || len(m.queue) > 0 {
		status += fmt.Sprintf("   %s %d running / %d queued", m.spinner.View(), m.active, len(m.queue))
	}

	keys := "↑↓/jk select   r rescan   q quit"
	if m.selectedNeedsAttention() {
		r := m.repos[m.cursor]
		parts := []string{"↑↓/jk select"}
		if !r.InProgress && r.Branch != m.cfg.Branch {
			parts = append(parts, "m switch+update")
		}
		if !r.InProgress && canPullCurrent(r) {
			parts = append(parts, "p pull current")
		}
		if !r.InProgress && len(r.Changes) > 0 {
			parts = append(parts, "d discard+update")
		}
		parts = append(parts, "s SKIP", "r rescan", "q quit")
		keys = strings.Join(parts, "   ")
	}
	if m.confirm != confirmNone {
		keys = "y/enter confirm discard   n/esc cancel"
	}
	keysStyled := lipgloss.NewStyle().Foreground(muted).Render(keys)
	statusStyled := lipgloss.NewStyle().Foreground(text).Render(status)
	if lipgloss.Width(statusStyled)+lipgloss.Width(keysStyled)+3 <= width {
		space := width - lipgloss.Width(statusStyled) - lipgloss.Width(keysStyled)
		return statusStyled + strings.Repeat(" ", max(1, space)) + keysStyled
	}
	return statusStyled + "\n" + keysStyled
}

func (m model) renderConfirm(width, height int) string {
	if m.confirmIndex < 0 || m.confirmIndex >= len(m.repos) {
		return ""
	}
	r := m.repos[m.confirmIndex]
	maxChanges := min(len(r.Changes), 8)
	changes := make([]string, 0, maxChanges+1)
	for i := 0; i < maxChanges; i++ {
		changes = append(changes, "  "+r.Changes[i])
	}
	if len(r.Changes) > maxChanges {
		changes = append(changes, fmt.Sprintf("  … and %d more", len(r.Changes)-maxChanges))
	}
	warning := "This permanently removes tracked, staged, and untracked changes.\nIgnored files are kept."
	if m.cfg.DryRun {
		warning = "DRY RUN: no files will be changed."
	}
	action := fmt.Sprintf("update %s", m.cfg.Branch)
	if r.Branch != m.cfg.Branch {
		action = fmt.Sprintf("switch to %s and update", m.cfg.Branch)
	}
	message := fmt.Sprintf("Discard local changes in %s, then %s?\n\n%s\n\n%s\n\n[y] Discard & Continue    [n] Cancel", r.Name, action, strings.Join(changes, "\n"), warning)
	modal := lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(red).Padding(1, 2).Width(min(76, max(40, width-10))).Render(message)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

func actionKey(key, label string) string {
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(accent2)
	labelStyle := lipgloss.NewStyle().Foreground(text)
	return keyStyle.Render("["+key+"]") + " " + labelStyle.Render(label)
}

func stateIcon(state RepoState, spin string) string {
	switch state {
	case StateReady:
		return lipgloss.NewStyle().Foreground(green).Render("●")
	case StateAttention:
		return lipgloss.NewStyle().Foreground(yellow).Render("!")
	case StateUpdating:
		return spin
	case StateUpdated:
		return lipgloss.NewStyle().Foreground(green).Render("✓")
	case StateSkipped:
		return lipgloss.NewStyle().Foreground(muted).Render("↷")
	case StateFailed:
		return lipgloss.NewStyle().Foreground(red).Render("✗")
	default:
		return "?"
	}
}

func stateLabel(state RepoState) string {
	switch state {
	case StateReady:
		return "ready"
	case StateAttention:
		return "needs attention"
	case StateUpdating:
		return "updating"
	case StateUpdated:
		return "updated"
	case StateSkipped:
		return "skipped"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

func stateMessageStyle(state RepoState) lipgloss.Style {
	switch state {
	case StateUpdated, StateReady:
		return lipgloss.NewStyle().Foreground(green)
	case StateAttention:
		return lipgloss.NewStyle().Foreground(yellow)
	case StateFailed:
		return lipgloss.NewStyle().Foreground(red)
	default:
		return lipgloss.NewStyle().Foreground(text)
	}
}

func labelValue(label, value string) string {
	return lipgloss.NewStyle().Foreground(muted).Render(label+":") + " " + lipgloss.NewStyle().Foreground(text).Render(value)
}

func truncate(s string, maxWidth int) string {
	if maxWidth <= 0 || lipgloss.Width(s) <= maxWidth {
		return s
	}
	if maxWidth <= 1 {
		return "…"
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > maxWidth {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
