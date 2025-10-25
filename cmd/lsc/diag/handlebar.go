package diag

import (
	"fmt"
	"os"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var handlebarCmd = &cobra.Command{
	Use:       "handlebar [lock|unlock]",
	Short:     "Control handlebar lock",
	Long:      `Manually control the handlebar lock mechanism. Use with caution - normally handled automatically by vehicle state.`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"lock", "unlock"},
	Run: func(cmd *cobra.Command, args []string) {
		action := args[0]

		// Validate argument
		if action != "lock" && action != "unlock" {
			fmt.Fprintf(os.Stderr, format.Error("Invalid action '%s'. Must be 'lock' or 'unlock'\n"), action)
			return
		}

		// Send command
		command := fmt.Sprintf("handlebar:%s", action)
		if err := RedisClient.LPush("scooter:hardware", command); err != nil {
			fmt.Fprintf(os.Stderr, format.Error("Failed to send handlebar command: %v\n"), err)
			return
		}

		fmt.Printf("%s Handlebar %s command sent\n", format.Success("✓"), action)
		fmt.Println(format.Dim("Note: This bypasses the automatic handlebar control"))
	},
}

func init() {
	DiagCmd.AddCommand(handlebarCmd)
}
