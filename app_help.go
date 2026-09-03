package main

import (
	"strings"

	"charm.land/lipgloss/v2"
)

func (m model) renderHelp(width, height int) string {
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(accent2).Width(14)
	labelStyle := lipgloss.NewStyle().Foreground(text)
	sectionStyle := lipgloss.NewStyle().Bold(true).Foreground(yellow)

	shortcut := func(key, label string) string {
		return keyStyle.Render(key) + labelStyle.Render(label)
	}

	lines := []string{
		titleStyle.Render("Keyboard shortcuts"),
		"",
		sectionStyle.Render("Navigation"),
		shortcut("↑ / k", "Previous item"),
		shortcut("↓ / j", "Next item"),
		shortcut("g / Home", "All repositories"),
		shortcut("G / End", "Last repository"),
		"",
		sectionStyle.Render("Repository actions"),
		shortcut("m", "Switch to default branch & update"),
		shortcut("p", "Pull current branch"),
		shortcut("d", "Discard local changes & update"),
		shortcut("t", "Open native shell in selected repository"),
		shortcut("s", "SKIP repository"),
		"",
		sectionStyle.Render("Global"),
		shortcut("r", "Rescan when background work is idle"),
		shortcut("?", "Open / close shortcuts"),
		shortcut("q", "Quit"),
		shortcut("Ctrl+C", "Quit"),
		"",
		sectionStyle.Render("Confirmations"),
		shortcut("Enter", "Open shell / confirm action"),
		shortcut("y", "Confirm discard"),
		shortcut("n / Esc", "Cancel"),
		"",
		lipgloss.NewStyle().Foreground(muted).Render("Repository action keys are available only when they apply."),
		lipgloss.NewStyle().Foreground(muted).Render("Shell here is available only for a manually selected repository."),
		lipgloss.NewStyle().Foreground(muted).Render("Press ? or Esc to close."),
	}

	modalWidth := min(74, max(46, width-10))
	modal := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(accent2).
		Padding(1, 2).
		Width(modalWidth).
		Render(strings.Join(lines, "\n"))

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}
