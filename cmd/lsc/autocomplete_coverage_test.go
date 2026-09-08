package lsc

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func commandAtPath(t *testing.T, path string) *cobra.Command {
	t.Helper()
	cmd := rootCmd
	for _, name := range strings.Fields(path) {
		var next *cobra.Command
		for _, child := range cmd.Commands() {
			if child.Name() == name {
				next = child
				break
			}
		}
		if next == nil {
			t.Fatalf("command %q not found at %q", name, path)
		}
		cmd = next
	}
	return cmd
}

func TestArgumentCompletionCoverage(t *testing.T) {
	paths := []string{
		"settings get", "settings set", "settings del", "get", "set", "del",
		"ota check", "ota channel", "diag dashboard", "diag battery", "boot set",
		"led cue", "led fade", "watch", "logs",
		"service start", "service stop", "service restart", "service enable",
		"service disable", "service status", "service logs",
		"locations show", "locations delete", "locations edit", "locations touch",
		"nav set", "keycard remove", "keycard remove-master",
		"ext show", "ext enable", "ext disable", "ext test",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			cmd := commandAtPath(t, path)
			if cmd.ValidArgsFunction == nil && len(cmd.ValidArgs) == 0 {
				t.Fatalf("%s has no argument completion", path)
			}
		})
	}
}
