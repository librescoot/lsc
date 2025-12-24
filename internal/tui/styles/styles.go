package styles

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	PrimaryColor   = lipgloss.Color("39")  // Blue
	SuccessColor   = lipgloss.Color("42")  // Green
	WarningColor   = lipgloss.Color("214") // Orange
	ErrorColor     = lipgloss.Color("196") // Red
	DimColor       = lipgloss.Color("240") // Gray
	HighlightColor = lipgloss.Color("205") // Pink

	// Text styles
	Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(PrimaryColor).
		MarginBottom(1)

	Subtitle = lipgloss.NewStyle().
		Foreground(DimColor).
		Italic(true)

	Selected = lipgloss.NewStyle().
		Foreground(HighlightColor).
		Bold(true)

	Help = lipgloss.NewStyle().
		Foreground(DimColor).
		Italic(true)

	// Status styles
	Success = lipgloss.NewStyle().
		Foreground(SuccessColor)

	Warning = lipgloss.NewStyle().
		Foreground(WarningColor)

	Error = lipgloss.NewStyle().
		Foreground(ErrorColor)

	// UI elements
	Border = lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(DimColor).
		Padding(1, 2)

	StatusBar = lipgloss.NewStyle().
		Foreground(DimColor).
		Background(lipgloss.Color("235"))
)
