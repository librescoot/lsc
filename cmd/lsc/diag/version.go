package diag

import (
	"encoding/json"
	"fmt"
	"os"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show firmware versions",
	Long:  `Display firmware versions for all system components.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Fetch version data from various sources
		system, err := RedisClient.HGetAll("system")
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"error": err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to fetch system data: %v\n"), err)
			}
			return
		}

		ecuData, _ := RedisClient.HGetAll("engine-ecu")
		battery0Data, _ := RedisClient.HGetAll("battery:0")
		battery1Data, _ := RedisClient.HGetAll("battery:1")
		otaData, _ := RedisClient.HGetAll("ota")

		if JSONOutput != nil && *JSONOutput {
			// Build JSON output
			output := map[string]interface{}{
				"system": map[string]interface{}{
					"mdb":         format.SafeValueOr(system["mdb-version"], ""),
					"dbc":         format.SafeValueOr(system["dbc-version"], ""),
					"nrf":         format.SafeValueOr(system["nrf-fw-version"], ""),
					"environment": format.SafeValueOr(system["environment"], ""),
				},
				"components": map[string]interface{}{
					"ecu": format.SafeValueOr(ecuData["fw-version"], ""),
				},
				"ota": map[string]interface{}{
					"system":       format.SafeValueOr(otaData["system"], ""),
					"status":       format.SafeValueOr(otaData["status"], ""),
					"fresh_update": otaData["fresh-update"] == "true",
				},
			}

			// Add battery info
			batteries := make(map[string]interface{})
			if battery0Data["present"] == "true" {
				batteries["0"] = map[string]interface{}{
					"present":       true,
					"version":       format.SafeValueOr(battery0Data["fw-version"], ""),
					"serial_number": format.SafeValueOr(battery0Data["serial-number"], ""),
				}
			} else {
				batteries["0"] = map[string]interface{}{"present": false}
			}
			if battery1Data["present"] == "true" {
				batteries["1"] = map[string]interface{}{
					"present":       true,
					"version":       format.SafeValueOr(battery1Data["fw-version"], ""),
					"serial_number": format.SafeValueOr(battery1Data["serial-number"], ""),
				}
			} else {
				batteries["1"] = map[string]interface{}{"present": false}
			}
			output["batteries"] = batteries

			jsonBytes, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(jsonBytes))
			return
		}

		// Display system versions
		format.PrintSection("System Versions")
		format.PrintKV("MDB", format.SafeValueOr(system["mdb-version"], "N/A"))
		format.PrintKV("DBC", format.SafeValueOr(system["dbc-version"], "N/A"))
		format.PrintKV("nRF", format.SafeValueOr(system["nrf-fw-version"], "N/A"))
		format.PrintKV("Environment", format.SafeValueOr(system["environment"], "N/A"))

		// Display component versions
		format.PrintSection("Component Versions")
		format.PrintKV("ECU", format.SafeValueOr(ecuData["fw-version"], "N/A"))

		if battery0Data["present"] == "true" {
			serial := format.SafeValueOr(battery0Data["serial-number"], "")
			version := format.SafeValueOr(battery0Data["fw-version"], "N/A")
			if serial != "" {
				format.PrintKV("Battery 0", fmt.Sprintf("%s (S/N: %s)", version, serial))
			} else {
				format.PrintKV("Battery 0", version)
			}
		} else {
			format.PrintKV("Battery 0", format.Dim("Not Present"))
		}

		if battery1Data["present"] == "true" {
			serial := format.SafeValueOr(battery1Data["serial-number"], "")
			version := format.SafeValueOr(battery1Data["fw-version"], "N/A")
			if serial != "" {
				format.PrintKV("Battery 1", fmt.Sprintf("%s (S/N: %s)", version, serial))
			} else {
				format.PrintKV("Battery 1", version)
			}
		} else {
			format.PrintKV("Battery 1", format.Dim("Not Present"))
		}

		// Display OTA info
		format.PrintSection("OTA System")
		format.PrintKV("System", format.SafeValueOr(otaData["system"], "N/A"))
		format.PrintKV("Status", format.SafeValueOr(otaData["status"], "N/A"))
		if otaData["fresh-update"] == "true" {
			format.PrintKV("Fresh Update", format.Success("Yes"))
		}

		fmt.Println()
	},
}

func init() {
	DiagCmd.AddCommand(versionCmd)
}
