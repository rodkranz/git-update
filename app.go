package main

import (
	"charm.land/bubbles/v2/spinner"
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

type shellFinishedMsg struct {
	index int
	err   error
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
	confirmShellSelected
)

type model struct {
	cfg      Config
	spinner  spinner.Model
	width    int
	height   int
	repos    []Repo
	cursor   int // 0 = All repositories, 1..N = repository index + 1
	scanning bool
	scanErr  error
	showHelp bool

	decisionIndex int
	confirm       confirmMode
	confirmIndex  int

	queue     []repoJob
	active    int
	globalLog []string
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
