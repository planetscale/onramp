package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type resultMsg struct{ err error }

func waitForResult(ch <-chan error) tea.Cmd {
	return func() tea.Msg { return resultMsg{err: <-ch} }
}

type spinnerModel struct {
	spinner spinner.Model
	msg     string // evaluated lazily at View time; overrides msg when set
	done    bool
	err     error
	fn      func() error // launched in Init so closures see prior-step values
}

func (m spinnerModel) Init() tea.Cmd {
	ch := make(chan error, 1)
	go func() { ch <- m.fn() }()
	return tea.Batch(m.spinner.Tick, waitForResult(ch))
}

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case resultMsg:
		m.err = msg.err
		m.done = true
		return m, func() tea.Msg { return stepDoneMsg{} }
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m spinnerModel) View() string {
	if m.done {
		return ""
	}
	label := m.msg
	return fmt.Sprintf("%s %s...\n", m.spinner.View(), label)
}

func (m spinnerModel) result() error   { return m.err }
func (m spinnerModel) summary() string { return "" }

// NewSpinnerStep is like newSpinnerStep but evaluates the display message
// lazily so closures that reference prior-step outputs are resolved at render time.
func NewSpinnerStep(msg string, fn func() error) Step {
	s := spinner.New()
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#fd812d"))
	s.Spinner = spinner.Dot
	return spinnerModel{spinner: s, msg: msg, fn: fn}
}
