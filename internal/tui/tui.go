// Package tui
package tui

import (
	"errors"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// stepDoneMsg is sent by a step when it finishes.
type stepDoneMsg struct{}

// Step is a tea.Model that also reports whether it completed successfully
// and provides a one-line summary to display after it finishes.
type Step interface {
	tea.Model
	result() error
	summary() string
}

// seqModel runs steps one at a time inside a single tea.Program.
type seqModel struct {
	funcs   []func() Step
	steps   *[]Step
	current int
	done    []string // summary lines from completed steps
	err     error
}

func (m seqModel) Init() tea.Cmd {
	(*m.steps)[m.current] = m.funcs[m.current]()
	return (*m.steps)[m.current].Init()
}

var updateMu sync.Mutex

func (m seqModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Global ctrl+c cancels the whole wizard.
	updateMu.Lock()
	defer updateMu.Unlock()
	if k, ok := msg.(tea.KeyMsg); ok && k.String() == "ctrl+c" {
		m.err = errors.New("cancelled")
		return m, tea.Quit
	}

	if _, ok := msg.(stepDoneMsg); ok {
		if err := (*m.steps)[m.current].result(); err != nil {
			m.err = err
			return m, tea.Quit
		}
		if s := (*m.steps)[m.current].summary(); s != "" {
			m.done = append(m.done, s)
		}
		for {
			m.current++
			if m.current >= len(*m.steps) {
				return m, tea.Quit
			}
			(*m.steps)[m.current] = m.funcs[m.current]()
			if (*m.steps)[m.current] != nil {
				break
			}
		}
		return m, (*m.steps)[m.current].Init()
	}

	updated, cmd := (*m.steps)[m.current].Update(msg)
	(*m.steps)[m.current] = updated.(Step)
	return m, cmd
}

func (m seqModel) View() string {
	var sb strings.Builder
	for _, line := range m.done {
		sb.WriteString(line + "\n")
	}
	if m.current < len(*m.steps) {
		sb.WriteString((*m.steps)[m.current].View())
	}
	return sb.String()
}

// RunWizard runs all steps sequentially inside a single tea.Program.
func RunWizard(funcs []func() Step) error {
	steps := make([]Step, len(funcs))
	p := tea.NewProgram(seqModel{funcs: funcs, steps: &steps})
	result, err := p.Run()
	if err != nil {
		return err
	}
	return result.(seqModel).err
}
