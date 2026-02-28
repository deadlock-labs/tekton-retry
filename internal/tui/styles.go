package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Color palette
	colorPrimary   = lipgloss.Color("#7C3AED") // Purple
	colorSuccess   = lipgloss.Color("#10B981") // Green
	colorDanger    = lipgloss.Color("#EF4444") // Red
	colorWarning   = lipgloss.Color("#F59E0B") // Amber
	colorMuted     = lipgloss.Color("#6B7280") // Gray
	colorHighlight = lipgloss.Color("#3B82F6") // Blue
	colorWhite     = lipgloss.Color("#FFFFFF")

	// Title bar
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorWhite).
			Background(colorPrimary).
			Padding(0, 2).
			MarginBottom(1)

	// Status badges
	StatusSucceeded = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorSuccess)

	StatusFailed = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorDanger)

	StatusRunning = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorWarning)

	StatusUnknown = lipgloss.NewStyle().
			Foreground(colorMuted)

	// List items
	SelectedItem = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorHighlight).
			PaddingLeft(2)

	NormalItem = lipgloss.NewStyle().
			PaddingLeft(2)

	// Cursor
	CursorStyle = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true)

	// Search box
	SearchBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(0, 1).
			MarginBottom(1)

	// Help bar
	HelpStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			MarginTop(1)

	// Results
	SuccessStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorSuccess).
			MarginTop(1)

	ErrorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorDanger).
			MarginTop(1)

	// Details panel
	DetailLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorMuted).
				Width(16)

	DetailValueStyle = lipgloss.NewStyle().
				Foreground(colorWhite)
)

// StatusStyle returns the appropriate style for a given status.
func StatusStyle(status string) lipgloss.Style {
	switch status {
	case "Succeeded":
		return StatusSucceeded
	case "Failed":
		return StatusFailed
	case "Running":
		return StatusRunning
	default:
		return StatusUnknown
	}
}

// StatusIcon returns a unicode icon for a status.
func StatusIcon(status string) string {
	switch status {
	case "Succeeded":
		return "✓"
	case "Failed":
		return "✗"
	case "Running":
		return "●"
	default:
		return "?"
	}
}
