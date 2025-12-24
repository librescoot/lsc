package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"librescoot/lsc/internal/redis"
	"librescoot/lsc/internal/tui/models"
)

// ShouldLaunchTUI determines if TUI mode should activate
func ShouldLaunchTUI(args []string) bool {
	// Don't launch TUI if:
	// - Args provided (user wants specific CLI command)
	// - Not a TTY (piped/redirected)
	// - NO_COLOR or TERM=dumb set
	// - --json flag present in args

	if len(args) > 0 {
		// Check for --json flag
		for _, arg := range args {
			if arg == "--json" {
				return false
			}
		}
		// Other args mean user wants CLI
		return false
	}

	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return false
	}

	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}

	return true
}

// Launch starts the TUI main menu
func Launch(redisAddr string, redisClient *redis.Client) error {
	// Create main menu model
	m := models.NewMenu(redisAddr, redisClient)

	// Create Bubble Tea program
	p := tea.NewProgram(m, tea.WithAltScreen())

	// Run the program
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running TUI: %w", err)
	}

	return nil
}
