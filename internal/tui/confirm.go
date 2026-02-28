package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// ConfirmModel is a simple yes/no confirmation TUI.
type ConfirmModel struct {
	message   string
	details   []string
	confirmed bool
	answered  bool
	cursor    int // 0 = Yes, 1 = No
}

// NewConfirm creates a confirmation dialog.
func NewConfirm(message string, details []string) ConfirmModel {
	return ConfirmModel{
		message: message,
		details: details,
		cursor:  0,
	}
}

// Confirmed returns true if the user said yes.
func (m ConfirmModel) Confirmed() bool {
	return m.confirmed
}

func (m ConfirmModel) Init() tea.Cmd {
	return nil
}

func (m ConfirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c", "q"))):
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("left", "h"))):
			m.cursor = 0
		case key.Matches(msg, key.NewBinding(key.WithKeys("right", "l"))):
			m.cursor = 1

		case key.Matches(msg, key.NewBinding(key.WithKeys("y"))):
			m.confirmed = true
			m.answered = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("n"))):
			m.confirmed = false
			m.answered = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			m.confirmed = m.cursor == 0
			m.answered = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m ConfirmModel) View() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("⚡ Confirm Retry"))
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("  %s\n\n", m.message))

	for _, detail := range m.details {
		b.WriteString(fmt.Sprintf("   • %s\n", detail))
	}
	b.WriteString("\n")

	yesStyle := NormalItem
	noStyle := NormalItem
	if m.cursor == 0 {
		yesStyle = SelectedItem
	} else {
		noStyle = SelectedItem
	}

	b.WriteString(fmt.Sprintf("  %s    %s\n",
		yesStyle.Render("[ Yes ]"),
		noStyle.Render("[ No ]"),
	))

	b.WriteString(HelpStyle.Render("\n  ←/→ or y/n to choose • enter confirm"))
	return b.String()
}
