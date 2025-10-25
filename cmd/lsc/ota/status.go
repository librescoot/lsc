package ota

import (
	"encoding/json"
	"fmt"
	"os"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show OTA update status",
	Long:  `Display current OTA update status and information.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Get update status from Redis hash
		updateData, err := RedisClient.HGetAll("update")
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "ota-status",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to get OTA status: %v\n"), err)
			}
			return
		}

		if len(updateData) == 0 {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "ota-status",
					"status":  "no-data",
				})
				fmt.Println(string(output))
			} else {
				fmt.Println(format.Dim("No OTA update information available"))
			}
			return
		}

		if JSONOutput != nil && *JSONOutput {
			output := map[string]interface{}{
				"command": "ota-status",
				"status":  "success",
			}
			for k, v := range updateData {
				output[k] = v
			}
			jsonBytes, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(jsonBytes))
		} else {
			format.PrintSection("OTA Update Status")
			fmt.Println()

			for k, v := range updateData {
				format.PrintKV(k, v)
			}
		}
	},
}

func init() {
	OTACmd.AddCommand(statusCmd)
}
