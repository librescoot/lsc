package power

import (
	"encoding/json"
	"fmt"
	"os"

	"librescoot/lsc/internal/cli"
	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Set power state to run",
	Long:  `Request the power manager to transition to run (normal operation) state.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := RedisClient.LPush("scooter:power", "run"); err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "run",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to send run command: %v\n"), err)
			}
			return cli.ErrSilent
		}

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "run",
				"status":  "success",
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success("Power state set to: run"))
		}
		return nil
	},
}

func init() {
	PowerCmd.AddCommand(runCmd)
}
