package lsc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"librescoot/lsc/internal/format"
	"librescoot/lsc/internal/registry"

	"github.com/spf13/cobra"
)

// Note: Settings registry moved to internal/registry package

var forceSet bool

var settingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "Manage scooter settings",
	Long:  `View and modify scooter settings stored in Redis.`,
	Run: func(cmd *cobra.Command, args []string) {
		// When called without subcommand, show all settings
		settingsListCmd.Run(cmd, args)
	},
}

var settingsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all settings",
	Long:  `Display all known settings. Shows current values from Redis, with unset settings shown as (not set).`,
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
			// For JSON output, merge known settings with current values
			result := make(map[string]interface{})
			for _, info := range registry.Settings {
				value, exists := settings[info.Key]
				if !exists || value == "" {
					result[info.Key] = nil
				} else {
					result[info.Key] = value
				}
			}
			jsonBytes, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(jsonBytes))
			return
		}

		// Show LibreScoot settings grouped by Service
		format.PrintSection("Settings")

		// Group settings by Service
		groupedSettings := make(map[string][]registry.Setting)
		services := registry.GetServices()

		for _, info := range registry.Settings {
			groupedSettings[info.Service] = append(groupedSettings[info.Service], info)
		}

		headers := []string{"KEY", "VALUE", "DESCRIPTION"}
		var rows [][]string

		for _, service := range services {
			// Prettify service name
			prettyService := strings.ReplaceAll(service, "-", " ")
			prettyService = strings.Title(prettyService)

			// Add section header
			rows = append(rows, []string{prettyService})

			for _, info := range groupedSettings[service] {
				value, exists := settings[info.Key]
				var displayValue string
				if !exists || value == "" {
					displayValue = format.Dim("(not set)")
				} else {
					displayValue = value
				}

				// Build description with possible values/units
				description := info.Description
				if len(info.PossibleValues) > 0 {
					description = fmt.Sprintf("%s (%s)", description, strings.Join(info.PossibleValues, ", "))
				} else if info.Unit != "" {
					description = fmt.Sprintf("%s [%s]", description, info.Unit)
				}

				rows = append(rows, []string{info.Key, displayValue, format.Dim(description)})
			}
		}

		// Show any unknown settings that exist in Redis but aren't in our known list
		unknownKeys := make([]string, 0)
		for key := range settings {
			if !registry.IsKnownSetting(key) && settings[key] != "" {
				unknownKeys = append(unknownKeys, key)
			}
		}

		if len(unknownKeys) > 0 {
			sort.Strings(unknownKeys)
			rows = append(rows, []string{"Unknown Settings"})
			for _, key := range unknownKeys {
				rows = append(rows, []string{key, settings[key], format.Dim("-")})
			}
		}

		format.PrintTable(headers, rows)

		fmt.Println()
	},
}

var settingsGetCmd = &cobra.Command{
	Use:   "get <key> [<key>...]",
	Short: "Get one or more setting values",
	Long:  `Retrieve the value of one or more settings.`,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if JSONOutput {
			result := make(map[string]interface{})
			hasError := false

			for _, key := range args {
				value, err := redisClient.HGet("settings", key)
				if err != nil {
					result[key] = map[string]interface{}{
						"error": err.Error(),
					}
					hasError = true
				} else if value == "" {
					result[key] = nil
				} else {
					result[key] = value
				}
			}

			if hasError {
				output, _ := json.Marshal(map[string]interface{}{
					"error":  "one or more keys failed",
					"values": result,
				})
				fmt.Println(string(output))
			} else {
				jsonBytes, _ := json.MarshalIndent(result, "", "  ")
				fmt.Println(string(jsonBytes))
			}
			return
		}

		// Non-JSON output
		for _, key := range args {
			value, err := redisClient.HGet("settings", key)
			if err != nil {
				fmt.Fprintf(os.Stderr, format.Error("Failed to get setting '%s': %v\n"), key, err)
				continue
			}

			if len(args) > 1 {
				// Multiple keys - show key=value format
				if value == "" {
					fmt.Printf("%s=%s\n", key, format.Dim("(not set)"))
				} else {
					fmt.Printf("%s=%s\n", key, value)
				}
			} else {
				// Single key - just show value
				if value == "" {
					fmt.Println(format.Dim("(not set)"))
				} else {
					fmt.Println(value)
				}
			}
		}
	},
}

var settingsSetCmd = &cobra.Command{
	Use:   "set <key> <value> [<key> <value>...]",
	Short: "Set one or more setting values",
	Long: `Set the value of one or more settings and publish the changes.

Examples:
  lsc settings set alarm.enabled true
  lsc settings set alarm.enabled true alarm.honk false alarm.duration 60

Common Settings:
  alarm.enabled                   - Enable/disable alarm (true/false)
  alarm.honk                      - Enable horn during alarm (true/false)
  alarm.duration                  - Alarm duration in seconds
  hibernation-timer               - Hibernation timeout in seconds
  updates.mdb.method              - Update method for MDB (delta/full)
  updates.mdb.channel             - Release channel for MDB (stable/testing/nightly)
  updates.mdb.check-interval      - Update check interval for MDB (hours, 0=never)
  updates.dbc.method              - Update method for DBC (delta/full)
  updates.dbc.channel             - Release channel for DBC (stable/testing/nightly)
  updates.dbc.check-interval      - Update check interval for DBC (hours, 0=never)
  cellular.apn                    - Cellular APN string

Use 'lsc settings list' to see all available settings and their current values.`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 2 {
			return fmt.Errorf("requires at least one key-value pair")
		}
		if len(args)%2 != 0 {
			return fmt.Errorf("requires even number of arguments (key-value pairs)")
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()
		type setResult struct {
			Key    string `json:"key"`
			Value  string `json:"value"`
			Status string `json:"status"`
			Error  string `json:"error,omitempty"`
		}

		var results []setResult
		hasError := false

		// Process key-value pairs
		for i := 0; i < len(args); i += 2 {
			key := args[i]
			value := args[i+1]

			// Validate the value
			validationErr := registry.ValidateValue(key, value)
			if validationErr != nil && !forceSet {
				// Without --force, validation errors are fatal
				hasError = true
				if JSONOutput {
					results = append(results, setResult{
						Key:    key,
						Value:  value,
						Status: "error",
						Error:  fmt.Sprintf("validation failed: %v", validationErr),
					})
				} else {
					fmt.Fprintf(os.Stderr, format.Error("Invalid value for '%s': %v\n"), key, validationErr)
				}
				continue
			} else if validationErr != nil && forceSet {
				// With --force, show warning but continue
				if !JSONOutput {
					fmt.Fprintf(os.Stderr, format.Warning("Validation would fail for '%s' (--force used): %v\n"), key, validationErr)
				}
			}

			// Set the value in Redis hash
			if err := redisClient.HSet("settings", key, value); err != nil {
				hasError = true
				if JSONOutput {
					results = append(results, setResult{
						Key:    key,
						Value:  value,
						Status: "error",
						Error:  fmt.Sprintf("failed to set: %v", err),
					})
				} else {
					fmt.Fprintf(os.Stderr, format.Error("Failed to set setting '%s': %v\n"), key, err)
				}
				continue
			}

			// Publish the change so services can react
			if err := redisClient.Publish(ctx, "settings", key); err != nil {
				if JSONOutput {
					results = append(results, setResult{
						Key:    key,
						Value:  value,
						Status: "warning",
						Error:  "setting updated but publish failed",
					})
				} else {
					fmt.Fprintf(os.Stderr, format.Warning("Setting '%s' updated but publish failed: %v\n"), key, err)
				}
			} else {
				if JSONOutput {
					results = append(results, setResult{
						Key:    key,
						Value:  value,
						Status: "success",
					})
				} else {
					fmt.Println(format.Success(fmt.Sprintf("Setting '%s' = '%s'", key, value)))
				}
			}
		}

		if JSONOutput {
			if hasError {
				output, _ := json.Marshal(map[string]interface{}{
					"status":  "partial",
					"results": results,
				})
				fmt.Println(string(output))
			} else {
				output, _ := json.Marshal(map[string]interface{}{
					"status":  "success",
					"results": results,
				})
				fmt.Println(string(output))
			}
		}
	},
}

var settingsDelCmd = &cobra.Command{
	Use:   "del <key>",
	Short: "Delete a setting key",
	Long:  `Delete a setting key from the settings hash and publish the change.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]

		// Delete the key from Redis hash
		if err := redisClient.HDel("settings", key); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"error": err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to delete setting '%s': %v\n"), key, err)
			}
			return
		}

		// Publish the change so services can react
		ctx := context.Background()
		if err := redisClient.Publish(ctx, "settings", key); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"key":     key,
					"status":  "warning",
					"message": "Setting deleted but publish failed",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Warning("Setting deleted but publish failed: %v\n"), err)
			}
			return
		}

		if JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"key":    key,
				"status": "success",
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success(fmt.Sprintf("Setting '%s' deleted", key)))
		}
	},
}

func init() {
	settingsCmd.AddCommand(settingsListCmd)
	settingsCmd.AddCommand(settingsGetCmd)
	settingsCmd.AddCommand(settingsSetCmd)
	settingsCmd.AddCommand(settingsDelCmd)

	// Add flags
	settingsSetCmd.Flags().BoolVar(&forceSet, "force", false, "Skip validation and force set the value")

	rootCmd.AddCommand(settingsCmd)
}
