package tui

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// FillProgress holds atomic counters written by the fill goroutine and read by the TUI.
type FillProgress struct {
	Rows     atomic.Int64
	Bytes    atomic.Int64 // current DB size in bytes; only updated in size mode
	TargetGB int          // 0 = rows mode
	Title    string
}

type (
	fillTickMsg time.Time
	fillDoneMsg struct{ err error }
)

// windowSize is the number of ticks retained for the sliding-window rows/s calculation.
// At 200 ms/tick this gives a ~5 s measurement window.
const windowSize = 25

type tickSample struct {
	t    time.Time
	rows int64
}

type fillModel struct {
	spinner    spinner.Model
	progress   *FillProgress
	fn         func() error
	startTime  time.Time
	samples    [windowSize]tickSample
	head       int // index of the oldest sample (ring buffer)
	filled     int // how many slots are populated
	rowsPerSec float64
	done       bool
	err        error
}

func waitForFillDone(ch <-chan error) tea.Cmd {
	return func() tea.Msg { return fillDoneMsg{err: <-ch} }
}

func fillTick() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(t time.Time) tea.Msg {
		return fillTickMsg(t)
	})
}

func (m fillModel) Init() tea.Cmd {
	ch := make(chan error, 1)
	go func() { ch <- m.fn() }()
	return tea.Batch(m.spinner.Tick, waitForFillDone(ch), fillTick())
}

func (m fillModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case fillDoneMsg:
		m.err = msg.err
		m.done = true
		return m, tea.Quit

	case fillTickMsg:
		// Write new sample into the ring buffer.
		slot := (m.head + m.filled) % windowSize
		m.samples[slot] = tickSample{t: time.Time(msg), rows: m.progress.Rows.Load()}
		if m.filled < windowSize {
			m.filled++
		} else {
			m.head = (m.head + 1) % windowSize // oldest evicted, advance head
		}

		// Compute rows/s across the full window.
		if m.filled > 1 {
			oldest := m.samples[m.head]
			newest := m.samples[(m.head+m.filled-1)%windowSize]
			if secs := newest.t.Sub(oldest.t).Seconds(); secs > 0 {
				m.rowsPerSec = float64(newest.rows-oldest.rows) / secs
			}
		}
		return m, fillTick()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.done = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m fillModel) View() string {
	if m.done {
		elapsed := time.Since(m.startTime)
		rows := m.progress.Rows.Load()
		avg := int64(0)
		if s := elapsed.Seconds(); s > 0 {
			avg = int64(float64(rows) / s)
		}
		if m.err != nil {
			return fmt.Sprintf("✗ fill failed: %s\n", m.err)
		}
		return fmt.Sprintf("✓ %s rows in %s  (%s rows/s avg)\n",
			commaInt(rows), elapsed.Round(time.Millisecond), commaInt(avg))
	}

	rows := m.progress.Rows.Load()
	rps := int64(m.rowsPerSec)

	if m.progress.TargetGB > 0 {
		curGB := float64(m.progress.Bytes.Load()) / (1 << 30)
		return fmt.Sprintf("%s %s  %s rows  ·  %s rows/s  ·  %.2f / %d GB\n",
			m.spinner.View(), m.progress.Title, commaInt(rows), commaInt(rps), curGB, m.progress.TargetGB)
	}
	return fmt.Sprintf("%s %s  %s rows  ·  %s rows/s\n",
		m.spinner.View(), m.progress.Title, commaInt(rows), commaInt(rps))
}

// RunFillSpinner runs fn in a goroutine and shows a live spinner with rows/s stats.
func RunFillSpinner(p *FillProgress, fn func() error) error {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#fd812d"))
	m := fillModel{
		spinner:   s,
		progress:  p,
		fn:        fn,
		startTime: time.Now(),
	}
	result, err := tea.NewProgram(m).Run()
	if err != nil {
		return err
	}
	return result.(fillModel).err
}

// commaInt formats n with thousands separators (e.g. 1234567 → "1,234,567").
func commaInt(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	rem := len(s) % 3
	b.WriteString(s[:rem])
	for i := rem; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
