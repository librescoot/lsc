package diag

import (
	"encoding/json"
	"fmt"
	"os"

	"librescoot/lsc/internal/cli"
	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

// boardVersion picks the human-readable version from a version:<board> hash,
// falling back to the lowercase version_id when the full string is absent.
func boardVersion(ver map[string]string) string {
	if v := ver["version"]; v != "" {
		return v
	}
	return ver["version_id"]
}

// kernelReport summarizes what version-service publishes about each board's own
// kernel: the release it runs, whether it agrees with /lib/modules and
// /boot/zImage, and what disagrees when it does not.
func kernelReport(versions map[string]map[string]string) map[string]map[string]string {
	out := make(map[string]map[string]string)
	for board, ver := range versions {
		if ver["kernel_check"] == "" && ver["kernel_release"] == "" {
			continue
		}
		out[board] = map[string]string{
			"release": ver["kernel_release"],
			"check":   ver["kernel_check"],
			"detail":  ver["kernel_check_detail"],
		}
	}
	return out
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show firmware versions",
	Long:  `Display firmware versions for all system components.`,
	RunE: func(cmd *cobra.Command, args []string) error {
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
			return cli.ErrSilent
		}

		// Board versions come from version-service, one hash per board. The
		// system hash carries mdb-version/dbc-version/environment too, but
		// those are written by radio-gaga, which is not part of the platform.
		mdbVer, _ := RedisClient.HGetAll("version:mdb")
		dbcVer, _ := RedisClient.HGetAll("version:dbc")

		ecuData, _ := RedisClient.HGetAll("engine-ecu")
		battery0Data, _ := RedisClient.HGetAll("battery:0")
		battery1Data, _ := RedisClient.HGetAll("battery:1")
		otaData, _ := RedisClient.HGetAll("ota")

		if JSONOutput != nil && *JSONOutput {
			// Build JSON output
			output := map[string]interface{}{
				"system": map[string]interface{}{
					"mdb": boardVersion(mdbVer),
					"dbc": boardVersion(dbcVer),
					"nrf": system["nrf-fw-version"],
				},
				"kernel": kernelReport(map[string]map[string]string{"mdb": mdbVer, "dbc": dbcVer}),
				"components": map[string]interface{}{
					"ecu": ecuData["fw-version"],
				},
				"ota": map[string]interface{}{
					"system":       otaData["system"],
					"status":       otaData["status"],
					"fresh_update": otaData["fresh-update"] == "true",
				},
			}

			// Add battery info
			batteries := make(map[string]interface{})
			if battery0Data["present"] == "true" {
				batteries["0"] = map[string]interface{}{
					"present":       true,
					"version":       battery0Data["fw-version"],
					"serial_number": battery0Data["serial-number"],
				}
			} else {
				batteries["0"] = map[string]interface{}{"present": false}
			}
			if battery1Data["present"] == "true" {
				batteries["1"] = map[string]interface{}{
					"present":       true,
					"version":       battery1Data["fw-version"],
					"serial_number": battery1Data["serial-number"],
				}
			} else {
				batteries["1"] = map[string]interface{}{"present": false}
			}
			output["batteries"] = batteries

			jsonBytes, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(jsonBytes))
			return nil
		}

		// Display system versions
		format.PrintSection("System Versions")
		format.PrintKV("MDB", format.SafeValueOr(boardVersion(mdbVer), "N/A"))
		format.PrintKV("DBC", format.SafeValueOr(boardVersion(dbcVer), "N/A"))
		format.PrintKV("nRF", format.SafeValueOr(system["nrf-fw-version"], "N/A"))

		// What each board reports about its own kernel, published by
		// version-service. Absent fields mean an older writer, not a fault.
		for _, board := range []struct {
			name     string
			versions map[string]string
		}{{"MDB", mdbVer}, {"DBC", dbcVer}} {
			check := board.versions["kernel_check"]
			if check == "" && board.versions["kernel_release"] == "" {
				continue
			}
			value := fmt.Sprintf("%s (%s)",
				format.SafeValueOr(board.versions["kernel_release"], "unknown release"), check)
			switch check {
			case "ok":
			case "skew":
				value = format.Error(value)
			default:
				value = format.Dim(value)
			}
			format.PrintKV(board.name+" kernel", value)
			if detail := board.versions["kernel_check_detail"]; detail != "" {
				format.PrintKV("", format.Dim(detail))
			}
		}

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
		return nil
	},
}

func init() {
	DiagCmd.AddCommand(versionCmd)
}
