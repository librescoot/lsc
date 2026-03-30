package diag

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var batteryCmd = &cobra.Command{
	Use:   "battery [id...]",
	Short: "Show detailed battery information",
	Long:  `Display comprehensive battery information for one or more batteries. If no IDs specified, shows all batteries.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Determine which batteries to show
		batteryIDs := []string{"0", "1"}
		if len(args) > 0 {
			batteryIDs = args
		}

		if JSONOutput != nil && *JSONOutput {
			// Collect all battery data for JSON output
			batteries := make([]interface{}, 0)
			for _, id := range batteryIDs {
				batteryData := getBatteryData(id)
				if batteryData != nil {
					batteries = append(batteries, batteryData)
				}
			}
			jsonBytes, _ := json.MarshalIndent(map[string]interface{}{
				"batteries": batteries,
			}, "", "  ")
			fmt.Println(string(jsonBytes))
		} else {
			for _, id := range batteryIDs {
				showBattery(id)
			}
		}
	},
}

func getBatteryData(id string) map[string]interface{} {
	data, err := RedisClient.HGetAll(fmt.Sprintf("battery:%s", id))
	if err != nil {
		return nil
	}

	// Check if battery is present
	if data["present"] != "true" {
		return map[string]interface{}{
			"id":      id,
			"present": false,
		}
	}

	// Parse numeric values
	parseInt := func(s string) int {
		v, _ := strconv.Atoi(s)
		return v
	}
	parseFloat := func(s string) float64 {
		v, _ := strconv.ParseFloat(s, 64)
		return v
	}

	// Get faults
	faults, _ := RedisClient.SMembers(fmt.Sprintf("battery:%s:faults", id))

	return map[string]interface{}{
		"id":      id,
		"present": true,
		"state":   data["state"],
		"charge": map[string]interface{}{
			"charge_percent": parseInt(data["charge"]),
			"voltage_v":      parseFloat(data["voltage"]) / 1000.0,
			"current_a":      parseFloat(data["current"]) / 1000.0,
		},
		"temperature": map[string]interface{}{
			"sensor_0_c": parseInt(data["temperature:0"]),
			"sensor_1_c": parseInt(data["temperature:1"]),
			"sensor_2_c": parseInt(data["temperature:2"]),
			"sensor_3_c": parseInt(data["temperature:3"]),
			"state":      data["temperature-state"],
		},
		"health": map[string]interface{}{
			"cycles":         parseInt(data["cycle-count"]),
			"health_percent": parseInt(data["state-of-health"]),
		},
		"identity": map[string]interface{}{
			"serial_number":      data["serial-number"],
			"manufacturing_date": data["manufacturing-date"],
			"firmware_version":   data["fw-version"],
		},
		"faults": faults,
	}
}

func showBattery(id string) {
	data, err := RedisClient.HGetAll(fmt.Sprintf("battery:%s", id))
	if err != nil {
		fmt.Fprintf(os.Stderr, format.Error("Failed to fetch battery:%s data: %v\n"), id, err)
		return
	}

	if data["present"] != "true" {
		format.PrintSection(fmt.Sprintf("Battery %s: not present", id))
		fmt.Println()
		return
	}

	format.PrintSection(fmt.Sprintf("Battery %s: present, %s", id, data["state"]))

	// Charge
	format.PrintKV("Charge", fmt.Sprintf("%s, %s, %s",
		format.FormatChargeColored(data["charge"]),
		format.FormatVoltageColored(data["voltage"]),
		format.MilliampsToAmps(data["current"]),
	))

	// Temperature
	format.PrintKV("Temperature", fmt.Sprintf("%s (%s, %s, %s, %s)",
		format.ColorizeState(data["temperature-state"]),
		format.FormatTemperatureColored(data["temperature:0"]),
		format.FormatTemperatureColored(data["temperature:1"]),
		format.FormatTemperatureColored(data["temperature:2"]),
		format.FormatTemperatureColored(data["temperature:3"]),
	))

	// Health
	soh, _ := strconv.Atoi(data["state-of-health"])
	cycles := format.SafeValueOr(data["cycle-count"], "0")
	sohStr := format.Dim("N/A")
	if soh > 0 {
		sohStr = format.ColorizePercentage(soh)
	}
	format.PrintKV("Health", fmt.Sprintf("%s (%s cycles)", sohStr, cycles))

	// Identity
	format.PrintKV("Identity", fmt.Sprintf("%s, manufactured %s, firmware %s",
		format.SafeValueOr(data["serial-number"], "N/A"),
		format.SafeValueOr(data["manufacturing-date"], "N/A"),
		format.SafeValueOr(data["fw-version"], "N/A"),
	))

	// Faults
	faults, err := RedisClient.SMembers(fmt.Sprintf("battery:%s:faults", id))
	if err == nil && len(faults) > 0 {
		for _, fault := range faults {
			fmt.Printf("  %s %s\n", format.Error("•"), fault)
		}
	} else if err == nil {
		format.PrintKV("Faults", format.Success("None"))
	}

	fmt.Println()
}

func init() {
	DiagCmd.AddCommand(batteryCmd)
}
