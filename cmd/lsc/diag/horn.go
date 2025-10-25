package diag

import (
	"encoding/json"
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
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "horn",
					"status":  "error",
					"error":   fmt.Sprintf("invalid state: %s", state),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Invalid state '%s'. Must be 'on' or 'off'\n"), state)
			}
			return
		}

		// Send command
		if err := RedisClient.LPush("scooter:horn", state); err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "horn",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to send horn command: %v\n"), err)
			}
			return
		}

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "horn",
				"status":  "success",
				"state":   state,
			})
			fmt.Println(string(output))
		} else {
			fmt.Printf("%s Horn: %s\n", format.Success("✓"), state)
		}
	},
}

func init() {
	DiagCmd.AddCommand(hornCmd)
}
