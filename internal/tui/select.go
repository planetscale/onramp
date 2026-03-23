package tui

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// listItem wraps a string to satisfy the list.Item interface.
type listItem string

func (i listItem) Title() string       { return string(i) }
func (i listItem) Description() string { return "" }
func (i listItem) FilterValue() string { return string(i) }

// simpleDelegate renders a numbered list with a "> " prefix on the selected item.
type simpleDelegate struct {
	normal   lipgloss.Style
	selected lipgloss.Style
}

func (d simpleDelegate) Height() int                             { return 1 }
func (d simpleDelegate) Spacing() int                            { return 0 }
func (d simpleDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
func (d simpleDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	i, ok := item.(listItem)
	if !ok {
		return
	}
	str := fmt.Sprintf("- %s", string(i))
	if index == m.Index() {
		fmt.Fprint(w, d.selected.Render("> "+strings.TrimLeft(str, " ")))
	} else {
		fmt.Fprint(w, d.normal.Render(str))
	}
}

type selectModel struct {
	list   list.Model
	title  string
	onDone func(string)
	chosen string
}

func (m selectModel) Init() tea.Cmd {
	if len(m.list.Items()) == 0 {
		return func() tea.Msg { return stepDoneMsg{} }
	}
	if len(m.list.Items()) == 1 {
		m.onDone(m.list.Items()[0].FilterValue())
		return func() tea.Msg { return stepDoneMsg{} }
	}
	return nil
}

func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(m.list.Items()) <= 1 {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if m.list.FilterState() == list.Filtering {
				break
			}
			if i, ok := m.list.SelectedItem().(listItem); ok {
				m.chosen = string(i)
				if m.onDone != nil {
					m.onDone(m.chosen)
				}
				return m, func() tea.Msg { return stepDoneMsg{} }
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	if cmd == nil {
		return m, nil
	}
	return m, cmd
}

func (m selectModel) View() string {
	if len(m.list.Items()) <= 1 {
		return ""
	}
	if m.title != "" {
		return m.title + ":\n" + m.list.View()
	}
	return m.list.View()
}

func (m selectModel) result() error {
	if len(m.list.Items()) <= 1 {
		return nil
	}
	if m.chosen == "" {
		return errors.New("cancelled")
	}
	return nil
}

func (m selectModel) summary() string {
	if m.title == "" || m.chosen == "" || len(m.list.Items()) == 0 {
		return ""
	}
	if m.chosen == "" {
		m.chosen = m.list.Items()[0].FilterValue()
	}
	return m.title + ": " + m.chosen
}

// NewSelectStep constructs a selectModel from a slice of string items.
func NewSelectStep(title string, items []string, fuzzy bool, onDone func(string)) selectModel {
	listItems := make([]list.Item, len(items))
	for i, s := range items {
		listItems[i] = listItem(s)
	}
	if len(items) > 20 {
		fuzzy = true
	}

	height := min(len(items)+4, 20)

	delegate := simpleDelegate{
		normal:   lipgloss.NewStyle().PaddingLeft(2),
		selected: lipgloss.NewStyle().PaddingLeft(0).Foreground(lipgloss.Color("#fd812d")),
	}

	l := list.New(listItems, delegate, 40, height)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(fuzzy)
	l.SetFilteringEnabled(fuzzy)
	l.SetShowFilter(fuzzy)
	if fuzzy {
		l.DisableQuitKeybindings()
	}

	return selectModel{list: l, title: title, onDone: onDone}
}
