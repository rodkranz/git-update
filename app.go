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

type confirmMode int

const (
	confirmNone confirmMode = iota
	confirmUpdateSelected
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

	queue  []int
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

func updateRepoCmd(index int, repo Repo, cfg Config, allowSwitch, allowDirty bool) tea.Cmd {
	return func() tea.Msg {
		return repoUpdatedMsg{index: index, repo: updateRepo(repo, cfg, allowSwitch, allowDirty)}
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
		if m.cursor >= len(m.repos) {
			m.cursor = max(0, len(m.repos)-1)
		}
	case repoUpdatedMsg:
		if msg.index >= 0 && msg.index < len(m.repos) {
			m.repos[msg.index] = msg.repo
		}
		if m.active > 0 {
			m.active--
		}
		cmds = append(cmds, m.startQueuedUpdates()...)
	case tea.KeyPressMsg:
		key := msg.String()
		if m.confirm != confirmNone {
			switch key {
			case "y", "Y", "enter":
				idx := m.confirmIndex
				m.confirm = confirmNone
				m.confirmIndex = -1
				if idx >= 0 && idx < len(m.repos) {
					repo := m.repos[idx]
					repo.State = StateUpdating
					repo.Message = "updating..."
					m.repos[idx] = repo
					m.active++
					cmds = append(cmds, updateRepoCmd(idx, repo, m.cfg, true, true))
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
			if m.active == 0 {
				m.scanning = true
				m.scanErr = nil
				cmds = append(cmds, scanCmd(m.cfg))
			}
		case "u", "enter":
			if m.cursor >= 0 && m.cursor < len(m.repos) && m.active < m.cfg.Workers {
				repo := m.repos[m.cursor]
				if repo.State == StateUpdating || repo.InProgress {
					break
				}
				if repo.Branch != m.cfg.Branch || len(repo.Changes) > 0 {
					m.confirm = confirmUpdateSelected
					m.confirmIndex = m.cursor
				} else {
					repo.State = StateUpdating
					repo.Message = "updating..."
					m.repos[m.cursor] = repo
					m.active++
					cmds = append(cmds, updateRepoCmd(m.cursor, repo, m.cfg, false, false))
				}
			}
		case "a":
			if !m.scanning {
				m.queue = nil
				for i := range m.repos {
					r := m.repos[i]
					if r.InProgress || r.State == StateUpdating {
						continue
					}
					if r.Branch == m.cfg.Branch && len(r.Changes) == 0 {
						m.queue = append(m.queue, i)
					} else if r.State != StateFailed {
						r.State = StateAttention
						r.Message = "needs manual confirmation; skipped by Update All"
						m.repos[i] = r
					}
				}
				cmds = append(cmds, m.startQueuedUpdates()...)
			}
		}
	}
	return m, tea.Batch(cmds...)
}

func (m *model) startQueuedUpdates() []tea.Cmd {
	var cmds []tea.Cmd
	for m.active < m.cfg.Workers && len(m.queue) > 0 {
		idx := m.queue[0]
		m.queue = m.queue[1:]
		if idx < 0 || idx >= len(m.repos) {
			continue
		}
		repo := m.repos[idx]
		repo.State = StateUpdating
		repo.Message = "updating..."
		m.repos[idx] = repo
		m.active++
		cmds = append(cmds, updateRepoCmd(idx, repo, m.cfg, false, false))
	}
	return cmds
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
	lines := []string{titleStyle.Render(r.Name), "", labelValue("Status", stateLabel(r.State)), labelValue("Branch", r.Branch), labelValue("Target", m.cfg.Branch), labelValue("Path", r.Path)}
	if r.InProgress {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(red).Bold(true).Render("Git operation in progress"))
	}
	lines = append(lines, "", lipgloss.NewStyle().Bold(true).Foreground(accent2).Render(fmt.Sprintf("Local changes (%d)", len(r.Changes))))
	if len(r.Changes) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(green).Render("✓ clean"))
	} else {
		maxChanges := max(2, (height-14)/2)
		for i, change := range r.Changes {
			if i >= maxChanges {
				lines = append(lines, lipgloss.NewStyle().Foreground(muted).Render(fmt.Sprintf("… and %d more", len(r.Changes)-i)))
				break
			}
			lines = append(lines, lipgloss.NewStyle().Foreground(yellow).Render(change))
		}
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

func (m model) renderFooter(width int) string {
	updated, attention, failed := 0, 0, 0
	for _, r := range m.repos {
		switch r.State {
		case StateUpdated:
			updated++
		case StateAttention:
			attention++
		case StateFailed:
			failed++
		}
	}
	status := fmt.Sprintf(" ✓ %d updated   ! %d attention   ✗ %d failed", updated, attention, failed)
	if m.active > 0 || len(m.queue) > 0 {
		status += fmt.Sprintf("   %s %d updating / %d queued", m.spinner.View(), m.active, len(m.queue))
	}
	keys := "↑↓/jk select   u/enter update   a update all safe   r rescan   q quit"
	if m.confirm != confirmNone {
		keys = "y/enter confirm   n/esc cancel"
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
	var reasons []string
	if r.Branch != m.cfg.Branch {
		reasons = append(reasons, fmt.Sprintf("switch %q → %q", r.Branch, m.cfg.Branch))
	}
	if len(r.Changes) > 0 {
		reasons = append(reasons, fmt.Sprintf("continue with %d local change(s)", len(r.Changes)))
	}
	message := fmt.Sprintf("Update %s?\n\n%s\n\nThis never runs stash/reset/clean/force.\n\n[y] Continue    [n] Cancel", r.Name, strings.Join(reasons, "\n"))
	modal := lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(accent).Padding(1, 2).Width(min(62, max(34, width-10))).Render(message)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
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
