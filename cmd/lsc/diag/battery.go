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
	Long:  `Display comprehensive battery information for one or more batteries. IDs can be numeric (0, 1) or named (aux, cb). If no IDs specified, shows all batteries.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Determine which batteries to show
		batteryIDs := []string{"0", "1", "aux", "cb"}
		if len(args) > 0 {
			batteryIDs = args
		}

		if JSONOutput != nil && *JSONOutput {
			batteries := make([]interface{}, 0)
			for _, id := range batteryIDs {
				switch id {
				case "aux":
					if data := getAuxBatteryData(); data != nil {
						batteries = append(batteries, data)
					}
				case "cb":
					if data := getCBBatteryData(); data != nil {
						batteries = append(batteries, data)
					}
				default:
					if data := getBatteryData(id); data != nil {
						batteries = append(batteries, data)
					}
				}
			}
			jsonBytes, _ := json.MarshalIndent(map[string]interface{}{
				"batteries": batteries,
			}, "", "  ")
			fmt.Println(string(jsonBytes))
		} else {
			for _, id := range batteryIDs {
				switch id {
				case "aux":
					showAuxBattery()
				case "cb":
					showCBBattery()
				default:
					showBattery(id)
				}
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
		fmt.Printf("\n%s\n\n", format.LightGray(fmt.Sprintf("=== Battery %s: not present ===", id)))
		return
	}

	prefix := format.LightGray(fmt.Sprintf("=== Battery %s: ", id))
	suffix := format.LightGray(" ===")
	fmt.Printf("\n%s%s, %s%s\n", prefix, format.ColorizeBatteryState("present"), format.ColorizeBatteryState(data["state"]), suffix)

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

func getAuxBatteryData() map[string]interface{} {
	data, err := RedisClient.HGetAll("aux-battery")
	if err != nil || len(data) == 0 {
		return nil
	}
	parseInt := func(s string) int {
		v, _ := strconv.Atoi(s)
		return v
	}
	parseFloat := func(s string) float64 {
		v, _ := strconv.ParseFloat(s, 64)
		return v
	}
	return map[string]interface{}{
		"id":             "aux",
		"voltage_v":      parseFloat(data["voltage"]) / 1000.0,
		"charge_percent": parseInt(data["charge"]),
		"charge_status":  data["charge-status"],
	}
}

func getCBBatteryData() map[string]interface{} {
	data, err := RedisClient.HGetAll("cb-battery")
	if err != nil || len(data) == 0 {
		return nil
	}
	parseInt := func(s string) int {
		v, _ := strconv.Atoi(s)
		return v
	}
	parseFloat := func(s string) float64 {
		v, _ := strconv.ParseFloat(s, 64)
		return v
	}
	present := data["present"] == "true"
	result := map[string]interface{}{
		"id":      "cb",
		"present": present,
	}
	if present {
		result["charge_percent"] = parseInt(data["charge"])
		result["charge_status"] = data["charge-status"]
		result["health_percent"] = parseInt(data["state-of-health"])
		result["cycles"] = parseInt(data["cycle-count"])
		result["temperature_c"] = parseInt(data["temperature"])
		result["voltage_v"] = parseFloat(data["cell-voltage"]) / 1000000.0
		result["current_a"] = parseFloat(data["current"]) / 1000000.0
	}
	return result
}

func showAuxBattery() {
	data, err := RedisClient.HGetAll("aux-battery")
	if err != nil {
		fmt.Fprintf(os.Stderr, format.Error("Failed to fetch aux-battery data: %v\n"), err)
		return
	}
	if len(data) == 0 {
		fmt.Printf("\n%s\n\n", format.LightGray("=== Aux Battery: no data ==="))
		return
	}

	fmt.Printf("\n%s\n", format.LightGray("=== Aux Battery ==="))
	if v := data["voltage"]; v != "" {
		format.PrintKV("Voltage", format.FormatAuxVoltageColored(v))
	}
	if c := data["charge"]; c != "" {
		chargeVal, _ := strconv.Atoi(c)
		format.PrintKV("Charge", format.ColorizePercentage(chargeVal))
	}
	if s := data["charge-status"]; s != "" {
		format.PrintKV("Status", format.ColorizeState(s))
	}
	fmt.Println()
}

func showCBBattery() {
	data, err := RedisClient.HGetAll("cb-battery")
	if err != nil {
		fmt.Fprintf(os.Stderr, format.Error("Failed to fetch cb-battery data: %v\n"), err)
		return
	}
	if data["present"] != "true" {
		fmt.Printf("\n%s\n\n", format.LightGray("=== CBB: not present ==="))
		return
	}

	fmt.Printf("\n%s\n", format.LightGray("=== CBB ==="))

	// Charge, voltage, current on one line
	chargeVal, _ := strconv.Atoi(data["charge"])
	chargeParts := format.ColorizePercentage(chargeVal)
	if v := data["cell-voltage"]; v != "" {
		chargeParts += ", " + format.FormatCBVoltageColored(v)
	}
	if c := data["current"]; c != "" {
		chargeParts += ", " + format.MicroampsToAmps(c)
	}
	format.PrintKV("Charge", chargeParts)

	// Health
	soh, _ := strconv.Atoi(data["state-of-health"])
	cycles := format.SafeValueOr(data["cycle-count"], "0")
	sohStr := format.Dim("N/A")
	if soh > 0 {
		sohStr = format.ColorizePercentage(soh)
	}
	format.PrintKV("Health", fmt.Sprintf("%s (%s cycles)", sohStr, cycles))

	if temp := data["temperature"]; temp != "" {
		format.PrintKV("Temperature", format.FormatTemperatureColored(temp))
	}
	if s := data["charge-status"]; s != "" {
		format.PrintKV("Status", format.ColorizeState(s))
	}
	fmt.Println()
}

func init() {
	DiagCmd.AddCommand(batteryCmd)
}
