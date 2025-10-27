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

// SettingInfo describes a known setting key
type SettingInfo struct {
	Key         string
	Description string
	Default     string
	Service     string
}

// knownSettings is a registry of all LibreScoot settings (using dot notation)
var knownSettings = []SettingInfo{
	// Alarm settings (alarm-service)
	{Key: "alarm.enabled", Description: "Enable/disable alarm system", Default: "false", Service: "alarm-service"},
	{Key: "alarm.honk", Description: "Enable horn during alarm trigger", Default: "false", Service: "alarm-service"},
	{Key: "alarm.duration", Description: "Duration in seconds for alarm sound", Default: "60", Service: "alarm-service"},

	// Power management settings (pm-service)
	{Key: "hibernation-timer", Description: "Hibernation timeout in seconds", Default: "900", Service: "pm-service"},

	// Update service settings (update-service)
	{Key: "updates.mdb.method", Description: "Update method for MDB (delta or full)", Default: "full", Service: "update-service"},
	{Key: "updates.mdb.channel", Description: "Release channel for MDB (stable/testing/nightly)", Default: "nightly", Service: "update-service"},
	{Key: "updates.mdb.check-interval", Description: "Time between update checks for MDB (hours, 0=never)", Default: "6", Service: "update-service"},
	{Key: "updates.mdb.github-releases-url", Description: "GitHub Releases API endpoint for MDB", Default: "https://api.github.com/repos/librescoot/librescoot/releases", Service: "update-service"},
	{Key: "updates.mdb.dry-run", Description: "Enable dry-run mode for MDB updates (no reboot)", Default: "false", Service: "update-service"},
	{Key: "updates.dbc.method", Description: "Update method for DBC (delta or full)", Default: "full", Service: "update-service"},
	{Key: "updates.dbc.channel", Description: "Release channel for DBC (stable/testing/nightly)", Default: "nightly", Service: "update-service"},
	{Key: "updates.dbc.check-interval", Description: "Time between update checks for DBC (hours, 0=never)", Default: "6", Service: "update-service"},
	{Key: "updates.dbc.github-releases-url", Description: "GitHub Releases API endpoint for DBC", Default: "https://api.github.com/repos/librescoot/librescoot/releases", Service: "update-service"},
	{Key: "updates.dbc.dry-run", Description: "Enable dry-run mode for DBC updates (no reboot)", Default: "false", Service: "update-service"},

	// Network settings
	{Key: "cellular.apn", Description: "Cellular APN string", Default: "", Service: "modem-service"},

	// Dashboard settings (scootui)
	{Key: "dashboard.show-raw-speed", Description: "Show raw uncorrected speed from ECU", Default: "false", Service: "scootui"},
	{Key: "dashboard.show-gps", Description: "GPS indicator visibility (always/active-or-error/error/never)", Default: "error", Service: "scootui"},
	{Key: "dashboard.show-bluetooth", Description: "Bluetooth indicator visibility (always/active-or-error/error/never)", Default: "active-or-error", Service: "scootui"},
	{Key: "dashboard.show-cloud", Description: "Cloud indicator visibility (always/active-or-error/error/never)", Default: "error", Service: "scootui"},
	{Key: "dashboard.show-internet", Description: "Internet indicator visibility (always/active-or-error/error/never)", Default: "always", Service: "scootui"},
	{Key: "dashboard.map.type", Description: "Map tile source (online/offline)", Default: "offline", Service: "scootui"},
	{Key: "dashboard.map.render-mode", Description: "Map rendering mode (vector/raster)", Default: "raster", Service: "scootui"},
	{Key: "dashboard.theme", Description: "UI theme (light/dark/auto)", Default: "dark", Service: "scootui"},
	{Key: "dashboard.mode", Description: "Default screen mode (speedometer/navigation)", Default: "speedometer", Service: "scootui"},
	{Key: "dashboard.valhalla-url", Description: "Valhalla routing service endpoint", Default: "http://localhost:8002/", Service: "scootui"},
}

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
			for _, info := range knownSettings {
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

		// Show LibreScoot settings
		format.PrintSection("Settings")
		for _, info := range knownSettings {
			value, exists := settings[info.Key]
			if !exists || value == "" {
				format.PrintKV(info.Key, format.Dim("(not set)"))
			} else {
				format.PrintKV(info.Key, value)
			}
		}

		// Show any unknown settings that exist in Redis but aren't in our known list
		unknownKeys := make([]string, 0)
		for key := range settings {
			known := false
			for _, info := range knownSettings {
				if info.Key == key {
					known = true
					break
				}
			}
			if !known && settings[key] != "" {
				unknownKeys = append(unknownKeys, key)
			}
		}

		if len(unknownKeys) > 0 {
			sort.Strings(unknownKeys)
			fmt.Println()
			format.PrintSection("Unknown Settings")
			for _, key := range unknownKeys {
				format.PrintKV(key, settings[key])
			}
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
