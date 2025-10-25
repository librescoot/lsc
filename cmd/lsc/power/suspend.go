package power

import (
	"encoding/json"
	"fmt"
	"os"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var suspendCmd = &cobra.Command{
	Use:   "suspend",
	Short: "Set power state to suspend",
	Long:  `Request the power manager to transition to suspend (low power) state.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := RedisClient.LPush("scooter:power", "suspend"); err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "suspend",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to send suspend command: %v\n"), err)
			}
			return
		}

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "suspend",
				"status":  "success",
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success("Power state set to: suspend"))
			fmt.Println(format.Dim("Note: System will enter low power mode"))
		}
	},
}

func init() {
	PowerCmd.AddCommand(suspendCmd)
}
