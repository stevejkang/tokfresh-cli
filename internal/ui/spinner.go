package ui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
)

func RunWithSpinner(title string, action func() error) error {
	var actionErr error

	err := spinner.New().
		Title(title).
		Action(func() {
			actionErr = action()
		}).
		Type(spinner.Line).
		Run()
	if err != nil {
		return err
	}

	return actionErr
}

type doneMsg struct{ err error }
type tickMsg struct{}

type fullscreenSpinner struct {
	title  string
	frame  int
	done   bool
	err    error
	action func() error
}

func (m fullscreenSpinner) Init() tea.Cmd {
	return tea.Batch(m.doTick(), m.doAction())
}

func (m fullscreenSpinner) doTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(_ time.Time) tea.Msg { return tickMsg{} })
}

func (m fullscreenSpinner) doAction() tea.Cmd {
	return func() tea.Msg { return doneMsg{err: m.action()} }
}

func (m fullscreenSpinner) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.frame++
		return m, m.doTick()
	case doneMsg:
		m.done = true
		m.err = msg.err
		return m, tea.Quit
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.err = fmt.Errorf("interrupted")
			return m, tea.Quit
		}
	}
	return m, nil
}

var frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (m fullscreenSpinner) View() string {
	f := lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#2563EB", Dark: "#60A5FA"}).Render(frames[m.frame%len(frames)])
	return fmt.Sprintf("%s\n%s %s", BrandHeader(), f, m.title)
}

func RunWithSpinnerFullscreen(title string, action func() error) error {
	ClearScreen()
	m := fullscreenSpinner{title: title, action: action}
	result, err := tea.NewProgram(m).Run()
	if err != nil {
		return err
	}
	if fm, ok := result.(fullscreenSpinner); ok && fm.err != nil {
		return fm.err
	}
	return nil
}
