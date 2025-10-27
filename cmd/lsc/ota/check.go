package ota

import (
	"encoding/json"
	"fmt"
	"os"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Trigger immediate update check",
	Long: `Trigger an immediate update check by sending a check-now command to the update service.

This bypasses the configured check interval and causes both MDB and DBC update services
to check for available updates immediately.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Send check-now command to scooter:update
		err := RedisClient.LPush("scooter:update", "check-now")
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "ota-check",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to trigger update check: %v\n"), err)
			}
			os.Exit(1)
			return
		}

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "ota-check",
				"status":  "success",
				"message": "Update check triggered",
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success("Update check triggered"))
			fmt.Println(format.Info("The update service will check for available updates immediately"))
			fmt.Println(format.Dim("Use 'lsc ota status' to monitor update progress"))
		}
	},
}

func init() {
	OTACmd.AddCommand(checkCmd)
}
