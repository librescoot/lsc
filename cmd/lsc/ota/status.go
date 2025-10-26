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
		updateData, err := RedisClient.HGetAll("ota")
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

		// Define all possible OTA keys per component
		components := []string{"mdb", "dbc"}
		allKeys := []string{
			"status",
			"update-version",
			"error",
			"error-message",
			"download-progress",
			"download-bytes",
			"download-total",
			"update-method",
		}

		// Build complete status map with all possible keys
		status := make(map[string]map[string]string)
		statusForJSON := make(map[string]map[string]interface{})
		for _, component := range components {
			status[component] = make(map[string]string)
			statusForJSON[component] = make(map[string]interface{})

			// Check if this component has a status key (indicates update service is running)
			statusKey := fmt.Sprintf("status:%s", component)
			componentStatus, hasStatus := updateData[statusKey]

			if hasStatus {
				status[component]["status"] = componentStatus
				statusForJSON[component]["status"] = componentStatus

				// For active components, show all keys (even if missing)
				for _, key := range allKeys {
					if key == "status" {
						continue // Already handled above
					}
					fullKey := fmt.Sprintf("%s:%s", key, component)
					if val, exists := updateData[fullKey]; exists && val != "" {
						status[component][key] = val
						statusForJSON[component][key] = val
					} else {
						status[component][key] = format.Dim("(not set)")
						statusForJSON[component][key] = nil // Use null for unset values in JSON
					}
				}
			} else {
				// Component has no status key - update service not running
				status[component]["status"] = format.Dim("(no update service)")
				statusForJSON[component]["status"] = nil
			}
		}

		if JSONOutput != nil && *JSONOutput {
			output := map[string]interface{}{
				"command": "ota-status",
				"status":  "success",
			}

			// For JSON, include raw updateData and structured status (without color codes)
			output["raw"] = updateData
			output["components"] = statusForJSON

			jsonBytes, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(jsonBytes))
		} else {
			format.PrintSection("OTA Update Status")
			fmt.Println()

			// Display each component
			for _, component := range components {
				componentStatus := status[component]

				fmt.Printf("%s:\n", format.Info(component))

				// Always show status first
				format.PrintKV("  status", componentStatus["status"])

				// If component is active, show all other keys
				if _, hasStatus := updateData[fmt.Sprintf("status:%s", component)]; hasStatus {
					for _, key := range allKeys {
						if key == "status" {
							continue
						}
						if val, ok := componentStatus[key]; ok {
							format.PrintKV(fmt.Sprintf("  %s", key), val)
						}
					}
				}

				fmt.Println()
			}
		}
	},
}

func init() {
	OTACmd.AddCommand(statusCmd)
}
