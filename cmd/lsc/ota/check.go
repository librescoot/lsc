package ota

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check [mdb|dbc]",
	Short: "Trigger immediate update check",
	Long: `Trigger an immediate update check by sending a check-now command to the update service.

Examples:
  lsc ota check       # Check both MDB and DBC
  lsc ota check mdb   # Check MDB only
  lsc ota check dbc   # Check DBC only`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var targets []string
		var successMsg string

		// Determine which components to trigger
		if len(args) == 0 {
			// No args = trigger both
			targets = []string{"mdb", "dbc"}
			successMsg = "Update check triggered for MDB and DBC"
		} else {
			component := args[0]
			if component != "mdb" && component != "dbc" {
				if JSONOutput != nil && *JSONOutput {
					output, _ := json.Marshal(map[string]interface{}{
						"command": "ota-check",
						"status":  "error",
						"error":   fmt.Sprintf("Invalid component '%s'. Must be 'mdb' or 'dbc'", component),
					})
					fmt.Println(string(output))
				} else {
					fmt.Fprintf(os.Stderr, format.Error("Invalid component '%s'. Must be 'mdb' or 'dbc'\n"), component)
				}
				os.Exit(1)
				return
			}
			targets = []string{component}
			successMsg = fmt.Sprintf("Update check triggered for %s", strings.ToUpper(component))
		}

		// Send check-now command to each target component
		for _, target := range targets {
			channel := fmt.Sprintf("scooter:update:%s", target)
			err := RedisClient.LPush(channel, "check-now")
			if err != nil {
				if JSONOutput != nil && *JSONOutput {
					output, _ := json.Marshal(map[string]interface{}{
						"command":   "ota-check",
						"component": target,
						"status":    "error",
						"error":     err.Error(),
					})
					fmt.Println(string(output))
				} else {
					fmt.Fprintf(os.Stderr, format.Error("Failed to trigger update check for %s: %v\n"), strings.ToUpper(target), err)
				}
				os.Exit(1)
				return
			}
		}

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command":    "ota-check",
				"components": targets,
				"status":     "success",
				"message":    successMsg,
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success(successMsg))
			fmt.Println(format.Info("The update service will check for available updates immediately"))
			fmt.Println(format.Dim("Use 'lsc ota status' to monitor update progress"))
		}
	},
}

func init() {
	OTACmd.AddCommand(checkCmd)
}
