package diag

import (
	"fmt"
	"os"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var hornCmd = &cobra.Command{
	Use:       "horn [on|off]",
	Short:     "Control horn",
	Long:      `Control the scooter's horn.`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"on", "off"},
	Run: func(cmd *cobra.Command, args []string) {
		state := args[0]

		// Validate argument
		if state != "on" && state != "off" {
			fmt.Fprintf(os.Stderr, format.Error("Invalid state '%s'. Must be 'on' or 'off'\n"), state)
			return
		}

		// Send command
		if err := RedisClient.LPush("scooter:horn", state); err != nil {
			fmt.Fprintf(os.Stderr, format.Error("Failed to send horn command: %v\n"), err)
			return
		}

		fmt.Printf("%s Horn: %s\n", format.Success("✓"), state)
	},
}

func init() {
	DiagCmd.AddCommand(hornCmd)
}
