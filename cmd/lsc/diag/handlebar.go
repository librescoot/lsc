package diag

import (
	"encoding/json"
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
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "handlebar",
					"status":  "error",
					"error":   fmt.Sprintf("invalid action: %s", action),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Invalid action '%s'. Must be 'lock' or 'unlock'\n"), action)
			}
			return
		}

		// Send command
		command := fmt.Sprintf("handlebar:%s", action)
		if err := RedisClient.LPush("scooter:hardware", command); err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "handlebar",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to send handlebar command: %v\n"), err)
			}
			return
		}

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "handlebar",
				"status":  "success",
				"action":  action,
			})
			fmt.Println(string(output))
		} else {
			fmt.Printf("%s Handlebar %s command sent\n", format.Success("✓"), action)
			fmt.Println(format.Dim("Note: This bypasses the automatic handlebar control"))
		}
	},
}

func init() {
	DiagCmd.AddCommand(handlebarCmd)
}
