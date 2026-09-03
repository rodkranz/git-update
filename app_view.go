package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

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
	target := "auto per repository"
	if m.cfg.Branch != "" {
		target = "override: " + m.cfg.Branch
	}
	right := lipgloss.NewStyle().Foreground(muted).Render(fmt.Sprintf("target: %s   workers: %d", target, m.cfg.Workers))
	space := max(1, width-lipgloss.Width(left)-lipgloss.Width(right)-2)
	return " " + left + strings.Repeat(" ", space) + right
}

func (m model) renderRepoList(width, height int) string {
	innerHeight := max(3, height-2)
	visible := max(1, innerHeight-2)
	itemCount := len(m.repos) + 1
	start := 0
	if m.cursor >= visible {
		start = m.cursor - visible + 1
	}
	end := min(itemCount, start+visible)
	lines := []string{titleStyle.Render(fmt.Sprintf("PROJECTS  %d", len(m.repos))), ""}
	for item := start; item < end; item++ {
		var line string
		if item == 0 {
			nameWidth := max(8, width-18)
			line = fmt.Sprintf("◎ %-*s %s", nameWidth, truncate("All repositories", nameWidth), m.allListStatus())
		} else {
			r := m.repos[item-1]
			prefix := stateIcon(r.State, m.spinner.View())
			branch := r.Branch
			if branch == "" {
				branch = "?"
			}
			nameWidth := max(8, width-18)
			line = fmt.Sprintf("%s %-*s %s", prefix, nameWidth, truncate(r.Name, nameWidth), truncate(branch, 12))
		}
		if item == m.cursor {
			line = selectedStyle.Width(max(1, width-4)).Render(line)
		} else {
			line = lipgloss.NewStyle().Foreground(text).Render(line)
		}
		lines = append(lines, line)
	}
	return panelStyle.Width(width).Height(height).Render(strings.Join(lines, "\n"))
}

func (m model) allListStatus() string {
	updated := 0
	for _, r := range m.repos {
		if r.State == StateUpdated {
			updated++
		}
	}
	return fmt.Sprintf("%d/%d", updated, len(m.repos))
}

func (m model) renderDetails(width, height int) string {
	if m.cursor == 0 {
		return m.renderAllDetails(width, height)
	}
	idx, ok := m.selectedRepoIndex()
	if !ok {
		return panelStyle.Width(width).Height(height).Render("No repository selected.")
	}
	return m.renderRepoDetails(m.repos[idx], width, height)
}

func (m model) renderAllDetails(width, height int) string {
	updated, attention, skipped, failed, updating := m.countStates()
	lines := []string{
		titleStyle.Render("All repositories"),
		"",
		fmt.Sprintf("%d repositories   %s %d running   %d queued", len(m.repos), m.spinner.View(), m.active, len(m.queue)),
		fmt.Sprintf("✓ %d updated   ! %d attention   ↷ %d skipped   ✗ %d failed", updated, attention, skipped, failed),
	}
	if updating > m.active {
		lines = append(lines, fmt.Sprintf("%d waiting for a worker", updating-m.active))
	}

	if m.decisionIndex >= 0 && m.decisionIndex < len(m.repos) && m.repos[m.decisionIndex].State == StateAttention {
		r := m.repos[m.decisionIndex]
		lines = append(lines, "", lipgloss.NewStyle().Bold(true).Foreground(yellow).Render("Needs input"))
		lines = append(lines,
			titleStyle.Render(r.Name),
			labelValue("Branch", r.Branch),
			labelValue("Target", displayTarget(r.TargetBranch)),
		)
		if len(r.Changes) > 0 {
			lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(accent2).Render(fmt.Sprintf("Local changes (%d)", len(r.Changes))))
			maxChanges := min(len(r.Changes), max(2, (height-len(lines)-12)/2))
			for i := 0; i < maxChanges; i++ {
				lines = append(lines, lipgloss.NewStyle().Foreground(yellow).Render(r.Changes[i]))
			}
			if len(r.Changes) > maxChanges {
				lines = append(lines, lipgloss.NewStyle().Foreground(muted).Render(fmt.Sprintf("… and %d more", len(r.Changes)-maxChanges)))
			}
		}
		lines = append(lines, m.renderActions(r))
	} else if m.active > 0 || len(m.queue) > 0 {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(green).Render("No input needed right now. Background updates are still running."))
	} else {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(green).Render("No repositories are waiting for input."))
	}

	lines = append(lines, "", lipgloss.NewStyle().Bold(true).Foreground(accent2).Render("Global activity"))
	maxLog := max(1, height-len(lines)-4)
	if len(m.globalLog) == 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(muted).Render("Waiting for activity..."))
	} else {
		start := max(0, len(m.globalLog)-maxLog)
		for _, line := range m.globalLog[start:] {
			lines = append(lines, lipgloss.NewStyle().Foreground(muted).Render(truncate(line, max(10, width-5))))
		}
	}
	return panelStyle.Width(width).Height(height).Render(strings.Join(lines, "\n"))
}
