package diag

import (
	"encoding/json"
	"fmt"
	"os"

	"librescoot/lsc/internal/cli"
	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var blinkersCmd = &cobra.Command{
	Use:       "blinkers [off|left|right|both]",
	Short:     "Control blinkers",
	Long:      `Control the scooter's turn signal blinkers.`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"off", "left", "right", "both"},
	RunE: func(cmd *cobra.Command, args []string) error {
		state := args[0]

		// Validate argument
		validStates := map[string]bool{
			"off":   true,
			"left":  true,
			"right": true,
			"both":  true,
		}

		if !validStates[state] {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "blinkers",
					"status":  "error",
					"error":   fmt.Sprintf("invalid state: %s", state),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Invalid state '%s'. Must be one of: off, left, right, both\n"), state)
			}
			return cli.ErrSilent
		}

		// Send command
		if err := RedisClient.LPush("scooter:blinker", state); err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "blinkers",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to send blinker command: %v\n"), err)
			}
			return cli.ErrSilent
		}

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "blinkers",
				"status":  "success",
				"state":   state,
			})
			fmt.Println(string(output))
		} else {
			fmt.Printf("%s Blinkers set to: %s\n", format.Success("✓"), state)
		}
		return nil
	},
}

func init() {
	DiagCmd.AddCommand(blinkersCmd)
}
