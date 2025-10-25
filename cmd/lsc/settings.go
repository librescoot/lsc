package lsc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var settingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "Manage scooter settings",
	Long:  `View and modify scooter settings stored in Redis.`,
}

var settingsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all settings",
	Long:  `Display all settings from the Redis settings hash.`,
	Run: func(cmd *cobra.Command, args []string) {
		settings, err := redisClient.HGetAll("settings")
		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"error": err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to fetch settings: %v\n"), err)
			}
			return
		}

		if JSONOutput {
			jsonBytes, _ := json.MarshalIndent(settings, "", "  ")
			fmt.Println(string(jsonBytes))
			return
		}

		if len(settings) == 0 {
			fmt.Println(format.Dim("No settings found"))
			return
		}

		format.PrintSection("Settings")

		// Sort keys for consistent output
		keys := make([]string, 0, len(settings))
		for key := range settings {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		// Print key-value pairs
		for _, key := range keys {
			format.PrintKV(key, settings[key])
		}

		fmt.Println()
	},
}

var settingsGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a setting value",
	Long:  `Retrieve the value of a specific setting.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]

		value, err := redisClient.HGet("settings", key)
		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"error": err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to get setting '%s': %v\n"), key, err)
			}
			return
		}

		if JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"key":   key,
				"value": value,
			})
			fmt.Println(string(output))
			return
		}

		if value == "" {
			fmt.Println(format.Dim("(not set)"))
			return
		}

		fmt.Println(value)
	},
}

var settingsSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a setting value",
	Long: `Set the value of a specific setting and publish the change.

Common settings:
  alarm.enabled       - Enable/disable alarm (true/false)
  alarm.honk          - Enable horn during alarm (true/false)
  alarm.duration      - Alarm duration in seconds (integer)
  scooter.speed_limit - Speed limit in km/h (integer)
  scooter.mode        - Driving mode (eco/normal/sport)
  cellular.apn        - Cellular APN string`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		value := args[1]

		// Set the value in Redis hash
		if err := redisClient.HSet("settings", key, value); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"error": err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to set setting '%s': %v\n"), key, err)
			}
			return
		}

		// Publish the change so services can react
		ctx := context.Background()
		if err := redisClient.Publish(ctx, "settings", key); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"key":     key,
					"value":   value,
					"status":  "warning",
					"message": "Setting updated but publish failed",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Warning("Setting updated but publish failed: %v\n"), err)
			}
			return
		}

		if JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"key":    key,
				"value":  value,
				"status": "success",
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success(fmt.Sprintf("Setting '%s' = '%s'", key, value)))
		}
	},
}

func init() {
	settingsCmd.AddCommand(settingsListCmd)
	settingsCmd.AddCommand(settingsGetCmd)
	settingsCmd.AddCommand(settingsSetCmd)
	rootCmd.AddCommand(settingsCmd)
}
