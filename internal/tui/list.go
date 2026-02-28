package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ford-mstech/pipeline-retry/internal/tekton"
)

// PipelineRunListModel is the TUI model for listing pipeline runs.
type PipelineRunListModel struct {
	runs     []tekton.PipelineRunInfo
	cursor   int
	selected *tekton.PipelineRunInfo
	quitting bool
	width    int
	height   int
}

// NewPipelineRunList creates a new list model.
func NewPipelineRunList(runs []tekton.PipelineRunInfo) PipelineRunListModel {
	return PipelineRunListModel{
		runs:   runs,
		cursor: 0,
	}
}

// Selected returns the user's selected pipeline run (nil if cancelled).
func (m PipelineRunListModel) Selected() *tekton.PipelineRunInfo {
	return m.selected
}

func (m PipelineRunListModel) Init() tea.Cmd {
	return nil
}

func (m PipelineRunListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("q", "esc", "ctrl+c"))):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("up", "k"))):
			if m.cursor > 0 {
				m.cursor--
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("down", "j"))):
			if m.cursor < len(m.runs)-1 {
				m.cursor++
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			if len(m.runs) > 0 {
				selected := m.runs[m.cursor]
				m.selected = &selected
			}
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m PipelineRunListModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	b.WriteString(TitleStyle.Render("🔧 Pipeline Runs"))
	b.WriteString("\n")

	if len(m.runs) == 0 {
		b.WriteString(HelpStyle.Render("  No pipeline runs found in this namespace."))
		b.WriteString("\n")
		return b.String()
	}

	// Header
	header := fmt.Sprintf("  %-3s %-40s %-14s %-12s %-20s",
		"", "NAME", "PIPELINE", "STATUS", "STARTED")
	b.WriteString(HelpStyle.Render(header))
	b.WriteString("\n")

	// Show runs with scroll window
	visibleCount := m.height - 6 // reserve space for header/footer
	if visibleCount < 5 {
		visibleCount = 5
	}
	if visibleCount > len(m.runs) {
		visibleCount = len(m.runs)
	}

	start := 0
	if m.cursor >= visibleCount {
		start = m.cursor - visibleCount + 1
	}
	end := start + visibleCount
	if end > len(m.runs) {
		end = len(m.runs)
	}

	for i := start; i < end; i++ {
		run := m.runs[i]
		cursor := "  "
		style := NormalItem
		if i == m.cursor {
			cursor = CursorStyle.Render("▸ ")
			style = SelectedItem
		}

		icon := StatusIcon(run.Status)
		statusStr := StatusStyle(run.Status).Render(fmt.Sprintf("%s %s", icon, run.Status))

		startedAgo := ""
		if !run.StartTime.IsZero() {
			startedAgo = formatTimeAgo(run.StartTime)
		}

		line := fmt.Sprintf("%-40s %-14s %s  %s",
			truncate(run.Name, 38),
			truncate(run.Pipeline, 12),
			statusStr,
			startedAgo,
		)
		b.WriteString(cursor + style.Render(line))
		b.WriteString("\n")
	}

	b.WriteString(HelpStyle.Render("\n  ↑/↓ navigate • enter select • q quit"))
	return b.String()
}
