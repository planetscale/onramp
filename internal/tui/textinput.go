package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type textInputModel struct {
	input     textinput.Model
	prompt    string
	value     string
	def       string
	cancelled bool
	onDone    func(string)
}

func (m textInputModel) Init() tea.Cmd { return textinput.Blink }

func (m textInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			m.value = strings.TrimSpace(m.input.Value())
			if m.onDone != nil {
				m.onDone(m.value)
			}
			return m, func() tea.Msg { return stepDoneMsg{} }
		case "esc":
			m.cancelled = true
			return m, func() tea.Msg { return stepDoneMsg{} }
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m textInputModel) View() string {
	prompt := m.prompt
	if m.def != "" {
		prompt = prompt + fmt.Sprintf(" (default=%s)", m.def)
	}
	return fmt.Sprintf("%s\n%s\n", prompt, m.input.View())
}

func (m textInputModel) result() error {
	if m.cancelled {
		return errors.New("cancelled")
	}
	return nil
}

func (m textInputModel) summary() string {
	label := strings.TrimSuffix(strings.TrimSpace(m.prompt), ":")
	v := m.value
	if v == "" {
		v = m.def
	}
	return label + ": " + v
}

func NewTextInputStep(prompt string, out *string, def string) Step {
	ti := textinput.New()
	ti.Focus()
	return textInputModel{
		input:  ti,
		def:    def,
		prompt: prompt,
		onDone: func(s string) {
			if s == "" {
				*out = def
				return
			}
			*out = s
		},
	}
}
