package lsc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"librescoot/lsc/internal/cli"
	"librescoot/lsc/internal/format"
	"librescoot/lsc/internal/redis"
	"librescoot/lsc/internal/schema"

	"github.com/spf13/cobra"
)

var forceSet bool

func fetchSchema() *schema.Schema {
	raw, err := redisClient.Get("settings:schema")
	if err != nil {
		return nil
	}
	s, err := schema.Parse([]byte(raw))
	if err != nil {
		fmt.Fprintf(os.Stderr, format.Warning("Failed to parse settings schema: %v\n"), err)
		return nil
	}
	return s
}

var settingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "Manage scooter settings",
	Long:  `View and modify scooter settings stored in Redis.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// When called without subcommand, show all settings
		return settingsListCmd.RunE(cmd, args)
	},
}

var settingsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all settings",
	Long:  `Display all known settings. Shows current values from Redis, with unset settings shown as (not set).`,
	RunE: func(cmd *cobra.Command, args []string) error {
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
			return cli.ErrSilent
		}

		s := fetchSchema()

		if JSONOutput {
			result := make(map[string]interface{})
			if s != nil {
				for key := range s.Settings {
					value, exists := settings[key]
					if !exists || value == "" {
						result[key] = nil
					} else {
						result[key] = value
					}
				}
			}
			// Include any Redis keys not in schema
			for key, value := range settings {
				if _, ok := result[key]; !ok && value != "" {
					result[key] = value
				}
			}
			jsonBytes, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(jsonBytes))
			return nil
		}

		format.PrintSection("Settings")
		headers := []string{"KEY", "VALUE", "DESCRIPTION"}
		var rows [][]string

		if s == nil {
			// No schema available -- fall back to raw key-value listing
			keys := make([]string, 0, len(settings))
			for k := range settings {
				if settings[k] != "" {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			for _, k := range keys {
				rows = append(rows, []string{k, settings[k], format.Dim("-")})
			}
			format.PrintTable(headers, rows)
			fmt.Println()
			return nil
		}

		// Group schema settings by service
		type schemaEntry struct {
			key     string
			setting schema.Setting
		}
		grouped := make(map[string][]schemaEntry)
		for key, setting := range s.Settings {
			svc := setting.Service
			if svc == "" {
				svc = "(unknown)"
			}
			grouped[svc] = append(grouped[svc], schemaEntry{key, setting})
		}
		// Sort keys within each service group
		for svc := range grouped {
			entries := grouped[svc]
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].key < entries[j].key
			})
			grouped[svc] = entries
		}

		services := s.Services()
		expandedIndexed := make(map[string]bool)

		for _, service := range services {
			prettyService := strings.ReplaceAll(service, "-", " ")
			prettyService = strings.Title(prettyService)
			rows = append(rows, []string{prettyService})

			for _, entry := range grouped[service] {
				info := entry.setting
				key := entry.key

				if info.Pattern == "indexed" {
					parts := strings.SplitN(key, ".0.", 2)
					if len(parts) != 2 {
						continue
					}
					prefix := parts[0] + "."
					if expandedIndexed[prefix] {
						continue
					}
					expandedIndexed[prefix] = true

					var matchingKeys []string
					for k := range settings {
						if strings.HasPrefix(k, prefix) && settings[k] != "" {
							matchingKeys = append(matchingKeys, k)
						}
					}
					sort.Strings(matchingKeys)
					for _, k := range matchingKeys {
						rows = append(rows, []string{k, settings[k], format.Dim("-")})
					}
					continue
				}

				value, exists := settings[key]
				var displayValue string
				if !exists || value == "" {
					displayValue = format.Dim("(not set)")
				} else {
					displayValue = value
				}

				description := info.Description
				if info.ReadOnly {
					description = fmt.Sprintf("%s [read-only]", description)
				}
				possibleValues := info.PossibleValues()
				if len(possibleValues) > 0 {
					description = fmt.Sprintf("%s (%s)", description, strings.Join(possibleValues, ", "))
				} else if info.Unit != "" {
					description = fmt.Sprintf("%s [%s]", description, info.Unit)
				}

				rows = append(rows, []string{key, displayValue, format.Dim(description)})
			}
		}

		// Show unknown settings (in Redis but not in schema)
		unknownKeys := make([]string, 0)
		for key := range settings {
			if !s.IsKnown(key) && settings[key] != "" {
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
		return nil
	},
}

var settingsGetCmd = &cobra.Command{
	Use:   "get <key> [<key>...]",
	Short: "Get one or more setting values",
	Long:  `Retrieve the value of one or more settings.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if JSONOutput {
			result := make(map[string]interface{})
			hasError := false

			for _, key := range args {
				value, err := redisClient.HGet("settings", key)
				if err != nil && redis.IsNil(err) {
					err = nil
					value = ""
				}
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
				return cli.ErrSilent
			}
			jsonBytes, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(jsonBytes))
			return nil
		}

		// Non-JSON output
		var hasError bool
		for _, key := range args {
			value, err := redisClient.HGet("settings", key)
			if err != nil && redis.IsNil(err) {
				err = nil
				value = ""
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, format.Error("Failed to get setting '%s': %v\n"), key, err)
				hasError = true
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
		if hasError {
			return cli.ErrSilent
		}
		return nil
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
  pm.hibernation-timer            - Hibernation timeout in seconds
  updates.mdb.method              - Update method for MDB (delta/full)
  updates.mdb.channel             - Release channel for MDB (stable/testing/nightly)
  updates.mdb.check-interval      - Update check interval for MDB (hours, 0=never)
  updates.dbc.method              - Update method for DBC (delta/full)
  updates.dbc.channel             - Release channel for DBC (stable/testing/nightly)
  updates.dbc.check-interval      - Update check interval for DBC (hours, 0=never)
  cellular.apn                    - Cellular APN string

Use 'lsc settings list' to see all available settings and their current values.`,
	Args: settingsSetArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		schemaForSet := fetchSchema()
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
			var validationErr error
			if schemaForSet != nil {
				validationErr = schemaForSet.ValidateValue(key, value)
			}
			if validationErr != nil && !forceSet {
				// Without --force, a failed validation skips this pair
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
		if hasError {
			return cli.ErrSilent
		}
		return nil
	},
}

// settingsSetArgs validates key-value pair arguments; shared with the 'set'
// shortcut so both accept the same argument shapes.
var settingsSetArgs = func(cmd *cobra.Command, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("requires at least one key-value pair")
	}
	if len(args)%2 != 0 {
		return fmt.Errorf("requires even number of arguments (key-value pairs)")
	}
	return nil
}

var settingsDelCmd = &cobra.Command{
	Use:   "del <key>",
	Short: "Delete a setting key",
	Long:  `Delete a setting key from the settings hash and publish the change.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
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
			return cli.ErrSilent
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
			return cli.ErrSilent
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
		return nil
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
	settingsCmd.GroupID = "main"
}
