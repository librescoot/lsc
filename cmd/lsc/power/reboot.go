package power

import (
	"encoding/json"
	"fmt"
	"os"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var rebootCmd = &cobra.Command{
	Use:   "reboot",
	Short: "Reboot the system",
	Long:  `Request the power manager to reboot the system.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := RedisClient.LPush("scooter:power", "reboot"); err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "reboot",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to send reboot command: %v\n"), err)
			}
			return
		}

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "reboot",
				"status":  "success",
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success("Reboot command sent"))
			fmt.Println(format.Warning("Warning: System will reboot"))
		}
	},
}

func init() {
	PowerCmd.AddCommand(rebootCmd)
}
