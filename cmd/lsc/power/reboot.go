package power

import (
	"encoding/json"
	"fmt"
	"os"

	"librescoot/lsc/internal/cli"
	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var rebootCmd = &cobra.Command{
	Use:   "reboot",
	Short: "Reboot the system",
	Long:  `Request the power manager to reboot the system.`,
	RunE: func(cmd *cobra.Command, args []string) error {
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
			return cli.ErrSilent
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
		return nil
	},
}

func init() {
	PowerCmd.AddCommand(rebootCmd)
}
