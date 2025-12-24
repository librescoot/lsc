package models

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"librescoot/lsc/internal/redis"
	"librescoot/lsc/internal/tui/styles"
)

type menuItem struct {
	title       string
	description string
}

type Menu struct {
	redisAddr   string
	redisClient *redis.Client
	items       []menuItem
	cursor      int
	width       int
	height      int
	quitting    bool
}

func NewMenu(redisAddr string, redisClient *redis.Client) Menu {
	items := []menuItem{
		{
			title:       "Settings Browser",
			description: "Navigate and edit scooter settings",
		},
		{
			title:       "Live Dashboard",
			description: "Real-time monitoring of vehicle state",
		},
		{
			title:       "Quit",
			description: "Exit the TUI",
		},
	}

	return Menu{
		redisAddr:   redisAddr,
		redisClient: redisClient,
		items:       items,
		cursor:      0,
	}
}

func (m Menu) Init() tea.Cmd {
	return tea.ClearScreen
}

func (m Menu) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}

		case "enter":
			// Handle selection
			switch m.cursor {
			case 0: // Settings Browser
				settingsModel := NewSettings(m.redisClient)
				return settingsModel, settingsModel.Init()
			case 1: // Live Dashboard
				dashboardModel := NewDashboard(m.redisClient)
				return dashboardModel, dashboardModel.Init()
			case 2: // Quit
				m.quitting = true
				return m, tea.Quit
			}
		}
	}

	return m, nil
}

func (m Menu) View() string {
	if m.quitting {
		return ""
	}

	s := styles.Title.Render("LibreScoot Control - Main Menu")
	s += "\n\n"

	// Render menu items
	for i, item := range m.items {
		cursor := " "
		if i == m.cursor {
			cursor = "▸"
		}

		title := item.title
		if i == m.cursor {
			title = styles.Selected.Render(title)
		}

		s += fmt.Sprintf("%s %s\n", cursor, title)
		s += fmt.Sprintf("  %s\n\n", styles.Subtitle.Render(item.description))
	}

	// Help text
	help := "\nUse ↑/↓ or j/k to navigate • Enter to select • q to quit"
	s += styles.Help.Render(help)

	content := lipgloss.NewStyle().Margin(2, 4).Render(s)

	// Pad to fill screen height if we have it
	if m.height > 0 {
		contentLines := strings.Count(content, "\n") + 1
		if contentLines < m.height {
			padding := strings.Repeat("\n", m.height-contentLines)
			content += padding
		}
	}

	return content
}
