package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/ford-mstech/pipeline-retry/internal/tekton"
)

// TaskSelectorModel is the TUI model for fuzzy-searching and multi-selecting failed tasks.
type TaskSelectorModel struct {
	tasks       []tekton.TaskRunInfo
	filtered    []int // indices into tasks
	selected    map[int]bool
	cursor      int
	search      textinput.Model
	confirmed   bool
	quitting    bool
	pipelineRun string
	width       int
	height      int
}

// NewTaskSelector creates a new task selector TUI.
func NewTaskSelector(tasks []tekton.TaskRunInfo, pipelineRunName string) TaskSelectorModel {
	ti := textinput.New()
	ti.Placeholder = "Type to filter tasks..."
	ti.Focus()
	ti.CharLimit = 100
	ti.Width = 50

	indices := make([]int, len(tasks))
	for i := range tasks {
		indices[i] = i
	}

	return TaskSelectorModel{
		tasks:       tasks,
		filtered:    indices,
		selected:    make(map[int]bool),
		cursor:      0,
		search:      ti,
		pipelineRun: pipelineRunName,
	}
}

// SelectedTasks returns the task names that were selected by the user.
func (m TaskSelectorModel) SelectedTasks() []string {
	if !m.confirmed {
		return nil
	}
	var names []string
	for idx := range m.selected {
		names = append(names, m.tasks[idx].TaskName)
	}
	return names
}

// Confirmed returns true if the user confirmed the selection.
func (m TaskSelectorModel) Confirmed() bool {
	return m.confirmed
}

func (m TaskSelectorModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m TaskSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			if m.search.Value() != "" {
				m.search.SetValue("")
				m.filterTasks()
			} else {
				m.quitting = true
				return m, tea.Quit
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("up"))):
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("down"))):
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("tab", " "))):
			if len(m.filtered) > 0 {
				realIdx := m.filtered[m.cursor]
				m.selected[realIdx] = !m.selected[realIdx]
				if !m.selected[realIdx] {
					delete(m.selected, realIdx)
				}
			}
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+a"))):
			// Toggle select all filtered
			allSelected := true
			for _, idx := range m.filtered {
				if !m.selected[idx] {
					allSelected = false
					break
				}
			}
			for _, idx := range m.filtered {
				if allSelected {
					delete(m.selected, idx)
				} else {
					m.selected[idx] = true
				}
			}
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			// If nothing explicitly selected, auto-select the current item
			if len(m.selected) == 0 && len(m.filtered) > 0 {
				realIdx := m.filtered[m.cursor]
				m.selected[realIdx] = true
			}
			if len(m.selected) > 0 {
				m.confirmed = true
			}
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.search, cmd = m.search.Update(msg)
	m.filterTasks()
	return m, cmd
}

func (m *TaskSelectorModel) filterTasks() {
	query := strings.ToLower(m.search.Value())
	if query == "" {
		m.filtered = make([]int, len(m.tasks))
		for i := range m.tasks {
			m.filtered[i] = i
		}
	} else {
		m.filtered = m.filtered[:0]
		for i, task := range m.tasks {
			if fuzzyMatch(strings.ToLower(task.TaskName), query) ||
				fuzzyMatch(strings.ToLower(task.Name), query) ||
				fuzzyMatch(strings.ToLower(task.FailureReason), query) {
				m.filtered = append(m.filtered, i)
			}
		}
	}

	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func (m TaskSelectorModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	b.WriteString(TitleStyle.Render(fmt.Sprintf("🔍 Failed Tasks — %s", m.pipelineRun)))
	b.WriteString("\n")

	// Search input
	b.WriteString(SearchBoxStyle.Render(m.search.View()))
	b.WriteString("\n")

	if len(m.tasks) == 0 {
		b.WriteString(SuccessStyle.Render("  No failed tasks found! 🎉"))
		b.WriteString("\n")
		return b.String()
	}

	// Info line
	info := fmt.Sprintf("  %d/%d tasks shown • %d selected",
		len(m.filtered), len(m.tasks), len(m.selected))
	b.WriteString(HelpStyle.Render(info))
	b.WriteString("\n\n")

	// Task list
	visibleCount := m.height - 10
	if visibleCount < 5 {
		visibleCount = 5
	}
	if visibleCount > len(m.filtered) {
		visibleCount = len(m.filtered)
	}

	start := 0
	if m.cursor >= visibleCount {
		start = m.cursor - visibleCount + 1
	}
	end := start + visibleCount
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	for vi := start; vi < end; vi++ {
		realIdx := m.filtered[vi]
		task := m.tasks[realIdx]

		cursor := "  "
		if vi == m.cursor {
			cursor = CursorStyle.Render("▸ ")
		}

		checkbox := "○"
		if m.selected[realIdx] {
			checkbox = StatusFailed.Render("●")
		}

		icon := StatusIcon(task.Status)
		statusStr := StatusStyle(task.Status).Render(fmt.Sprintf("%s %s", icon, task.Status))

		line := fmt.Sprintf("%s %s %-30s %s",
			checkbox,
			truncate(task.TaskName, 28),
			statusStr,
			HelpStyle.Render(truncate(task.FailureReason, 30)),
		)

		if vi == m.cursor {
			b.WriteString(cursor + SelectedItem.Render(line))
		} else {
			b.WriteString(cursor + NormalItem.Render(line))
		}
		b.WriteString("\n")
	}

	b.WriteString(HelpStyle.Render("\n  ↑/↓ navigate • space/tab toggle • ctrl+a all • enter confirm • esc back"))
	return b.String()
}

// fuzzyMatch implements simple substring + fuzzy character matching.
func fuzzyMatch(text, pattern string) bool {
	// First try simple substring
	if strings.Contains(text, pattern) {
		return true
	}

	// Fuzzy: all pattern chars must appear in order
	pi := 0
	for ti := 0; ti < len(text) && pi < len(pattern); ti++ {
		if text[ti] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}
