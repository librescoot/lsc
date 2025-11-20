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
	Long:  `Display current OTA update status and configuration.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Get settings from Redis
		settings, err := RedisClient.HGetAll("settings")
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "ota-status",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to get settings: %v\n"), err)
			}
			return
		}

		// Get runtime status from ota hash
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

		components := []string{"mdb", "dbc"}

		// Configuration keys from settings hash
		configKeys := []string{"method", "channel", "check-interval", "last-check-time"}

		// Runtime status keys from ota hash
		statusKeys := []string{"status", "update-version", "error", "error-message", "download-progress"}

		if JSONOutput != nil && *JSONOutput {
			result := make(map[string]map[string]interface{})

			for _, component := range components {
				result[component] = make(map[string]interface{})

				// Add configuration from settings
				for _, key := range configKeys {
					settingKey := fmt.Sprintf("updates.%s.%s", component, key)
					if val, exists := settings[settingKey]; exists && val != "" {
						result[component][key] = val
					} else {
						result[component][key] = nil
					}
				}

				// Add runtime status from ota hash
				for _, key := range statusKeys {
					otaKey := fmt.Sprintf("%s:%s", key, component)
					if val, exists := updateData[otaKey]; exists && val != "" {
						result[component][key] = val
					} else {
						result[component][key] = nil
					}
				}
			}

			jsonBytes, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(jsonBytes))
		} else {
			format.PrintSection("OTA Update Status")
			fmt.Println()

			for _, component := range components {
				fmt.Printf("%s:\n", format.Info(component))

				// Show configuration from settings
				for _, key := range configKeys {
					settingKey := fmt.Sprintf("updates.%s.%s", component, key)
					if val, exists := settings[settingKey]; exists && val != "" {
						format.PrintKV(fmt.Sprintf("  %s", key), val)
					} else {
						format.PrintKV(fmt.Sprintf("  %s", key), format.Dim("(not set)"))
					}
				}

				// Show runtime status from ota hash
				for _, key := range statusKeys {
					otaKey := fmt.Sprintf("%s:%s", key, component)
					if val, exists := updateData[otaKey]; exists && val != "" {
						format.PrintKV(fmt.Sprintf("  %s", key), val)
					} else {
						format.PrintKV(fmt.Sprintf("  %s", key), format.Dim("(not set)"))
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
