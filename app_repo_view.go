package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

func (m model) renderRepoDetails(r Repo, width, height int) string {
	lines := []string{
		titleStyle.Render(r.Name),
		"",
		labelValue("Status", stateLabel(r.State)),
		labelValue("Branch", r.Branch),
		labelValue("Target", displayTarget(r.TargetBranch)),
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
	wrongBranch := r.TargetBranch != "" && r.Branch != r.TargetBranch
	dirty := len(r.Changes) > 0

	if wrongBranch {
		switchLabel := fmt.Sprintf("Switch to %s & Update", r.TargetBranch)
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
	if dirty && r.TargetBranch != "" {
		discardLabel := "Discard changes & Update"
		if wrongBranch {
			discardLabel = fmt.Sprintf("Discard changes, switch to %s & Update", r.TargetBranch)
		}
		actions = append(actions, actionKey("d", discardLabel))
	}
	actions = append(actions, actionKey("s", "SKIP"))
	return strings.Join(actions, "    ")
}

func (m model) renderFooter(width int) string {
	updated, attention, skipped, failed, _ := m.countStates()
	status := fmt.Sprintf(" ✓ %d updated   ! %d attention   ↷ %d skipped   ✗ %d failed", updated, attention, skipped, failed)
	if m.active > 0 || len(m.queue) > 0 {
		status += fmt.Sprintf("   %s %d running / %d queued", m.spinner.View(), m.active, len(m.queue))
	}

	keys := "↑↓/jk select   g all   r rescan   q quit"
	if idx, ok := m.actionRepoIndex(); ok {
		r := m.repos[idx]
		parts := []string{"↑↓/jk select"}
		if !r.InProgress && r.TargetBranch != "" && r.Branch != r.TargetBranch {
			parts = append(parts, "m switch+update")
		}
		if !r.InProgress && canPullCurrent(r) {
			parts = append(parts, "p pull current")
		}
		if !r.InProgress && r.TargetBranch != "" && len(r.Changes) > 0 {
			parts = append(parts, "d discard+update")
		}
		parts = append(parts, "s SKIP", "g all", "q quit")
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

func (m model) countStates() (updated, attention, skipped, failed, updating int) {
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
		case StateUpdating:
			updating++
		}
	}
	return
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
	action := fmt.Sprintf("update %s", r.TargetBranch)
	if r.Branch != r.TargetBranch {
		action = fmt.Sprintf("switch to %s and update", r.TargetBranch)
	}
	message := fmt.Sprintf("Discard local changes in %s, then %s?\n\n%s\n\n%s\n\n[y] Discard & Continue    [n] Cancel", r.Name, action, strings.Join(changes, "\n"), warning)
	modal := lipgloss.NewStyle().Border(lipgloss.DoubleBorder()).BorderForeground(red).Padding(1, 2).Width(min(76, max(40, width-10))).Render(message)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

func displayTarget(target string) string {
	if target == "" {
		return "unknown"
	}
	return target
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
