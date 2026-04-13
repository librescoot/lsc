package lsc

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show overall scooter status",
	Long:  `Displays a dashboard of key metrics from various scooter services.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Fetch data from Redis
		vehicleData, err := redisClient.HGetAll("vehicle")
		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"error": err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Error fetching vehicle data: %v\n"), err)
			}
			return
		}

		ecuData, err := redisClient.HGetAll("engine-ecu")
		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"error": err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Error fetching ECU data: %v\n"), err)
			}
			return
		}

		battery0Data, err := redisClient.HGetAll("battery:0")
		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"error": err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Error fetching battery:0 data: %v\n"), err)
			}
			return
		}

		battery1Data, err := redisClient.HGetAll("battery:1")
		if err != nil {
			// Battery 1 might not exist, ignore error
			battery1Data = make(map[string]string)
		}

		auxBatteryData, _ := redisClient.HGetAll("aux-battery")
		cbBatteryData, _ := redisClient.HGetAll("cb-battery")

		// If JSON output is requested, output structured JSON
		if JSONOutput {
			outputStatusJSON(vehicleData, ecuData, battery0Data, battery1Data, auxBatteryData, cbBatteryData)
			return
		}

		// Display Vehicle Status
		format.PrintSection("Vehicle Status")
		format.PrintKV("State", format.ColorizeState(vehicleData["state"]))
		format.PrintKV("Kickstand", format.ColorizeState(vehicleData["kickstand"]))
		format.PrintKV("Brakes", fmt.Sprintf("L:%s R:%s",
			format.FormatOnOff(vehicleData["brake:left"]),
			format.FormatOnOff(vehicleData["brake:right"])))
		format.PrintKV("Blinker", format.SafeValueOr(vehicleData["blinker:switch"], "off"))
		format.PrintKV("Seatbox", format.SafeValueOr(vehicleData["seatbox:lock"], "closed"))

		// Display Motor Status
		format.PrintSection("Motor Status")
		format.PrintKV("Speed", format.FormatSpeed(ecuData["speed"]))
		format.PrintKV("RPM", format.FormatRPM(ecuData["rpm"]))
		format.PrintKV("Throttle", format.FormatOnOff(ecuData["throttle"]))
		format.PrintKV("Odometer", format.MetersToKilometers(ecuData["odometer"]))
		format.PrintKV("Voltage", format.MillivoltsToVolts(ecuData["motor:voltage"]))
		format.PrintKV("Current", format.MilliampsToAmps(ecuData["motor:current"]))
		format.PrintKV("Temperature", format.FormatTemperatureColored(ecuData["temperature"]))
		format.PrintKV("KERS", format.FormatOnOff(ecuData["kers"]))

		// Display Battery 0 Status
		prefix := format.LightGray("=== Battery 0: ")
		suffix := format.LightGray(" ===")
		if battery0Data["present"] == "true" {
			fmt.Printf("\n%s%s, %s%s\n", prefix, format.ColorizeBatteryState("present"), format.ColorizeBatteryState(battery0Data["state"]), suffix)
			format.PrintKV("Charge", fmt.Sprintf("%s, %s, %s",
				format.FormatChargeColored(battery0Data["charge"]),
				format.FormatVoltageColored(battery0Data["voltage"]),
				format.MilliampsToAmps(battery0Data["current"]),
			))
			format.PrintKV("Temperature", fmt.Sprintf("%s (%s)",
				format.ColorizeState(battery0Data["temperature-state"]),
				format.FormatTemperatureColored(battery0Data["temperature:0"]),
			))
			soh, _ := strconv.Atoi(battery0Data["state-of-health"])
			sohStr := format.Dim("N/A")
			if soh > 0 {
				sohStr = format.ColorizePercentage(soh)
			}
			format.PrintKV("Health", fmt.Sprintf("%s (%s cycles)", sohStr, format.SafeValueOr(battery0Data["cycle-count"], "0")))
		} else {
			fmt.Printf("\n%s%s%s\n", prefix, format.Dim("not present"), suffix)
		}

		// Display Battery 1 Status
		prefix = format.LightGray("=== Battery 1: ")
		if battery1Data["present"] == "true" {
			fmt.Printf("\n%s%s, %s%s\n", prefix, format.ColorizeBatteryState("present"), format.ColorizeBatteryState(battery1Data["state"]), suffix)
			format.PrintKV("Charge", fmt.Sprintf("%s, %s, %s",
				format.FormatChargeColored(battery1Data["charge"]),
				format.FormatVoltageColored(battery1Data["voltage"]),
				format.MilliampsToAmps(battery1Data["current"]),
			))
			format.PrintKV("Temperature", fmt.Sprintf("%s (%s)",
				format.ColorizeState(battery1Data["temperature-state"]),
				format.FormatTemperatureColored(battery1Data["temperature:0"]),
			))
			soh, _ := strconv.Atoi(battery1Data["state-of-health"])
			sohStr := format.Dim("N/A")
			if soh > 0 {
				sohStr = format.ColorizePercentage(soh)
			}
			format.PrintKV("Health", fmt.Sprintf("%s (%s cycles)", sohStr, format.SafeValueOr(battery1Data["cycle-count"], "0")))
		} else {
			fmt.Printf("\n%s%s%s\n", prefix, format.Dim("not present"), suffix)
		}

		// Display Auxiliary Battery
		if len(auxBatteryData) > 0 {
			fmt.Printf("\n%s\n", format.LightGray("=== Aux Battery ==="))
			chargeParts := ""
			if v := auxBatteryData["voltage"]; v != "" {
				chargeParts = format.FormatAuxVoltageColored(v)
			}
			if c := auxBatteryData["charge"]; c != "" {
				chargeVal, _ := strconv.Atoi(c)
				if chargeParts != "" {
					chargeParts = format.ColorizePercentage(chargeVal) + ", " + chargeParts
				} else {
					chargeParts = format.ColorizePercentage(chargeVal)
				}
			}
			if chargeParts != "" {
				format.PrintKV("Charge", chargeParts)
			}
			if s := auxBatteryData["charge-status"]; s != "" {
				format.PrintKV("Status", format.ColorizeState(s))
			}
		}

		// Display Control Board Battery
		if cbBatteryData["present"] == "true" {
			fmt.Printf("\n%s\n", format.LightGray("=== CB Battery ==="))
			chargeVal, _ := strconv.Atoi(cbBatteryData["charge"])
			chargeParts := format.ColorizePercentage(chargeVal)
			if v := cbBatteryData["cell-voltage"]; v != "" {
				chargeParts += ", " + format.FormatCBVoltageColored(v)
			}
			if c := cbBatteryData["current"]; c != "" {
				chargeParts += ", " + format.MicroampsToAmps(c)
			}
			format.PrintKV("Charge", chargeParts)
			soh, _ := strconv.Atoi(cbBatteryData["state-of-health"])
			sohStr := format.Dim("N/A")
			if soh > 0 {
				sohStr = format.ColorizePercentage(soh)
			}
			format.PrintKV("Health", fmt.Sprintf("%s (%s cycles)", sohStr, format.SafeValueOr(cbBatteryData["cycle-count"], "0")))
			if temp := cbBatteryData["temperature"]; temp != "" {
				format.PrintKV("Temperature", format.FormatTemperatureColored(temp))
			}
			if s := cbBatteryData["charge-status"]; s != "" {
				format.PrintKV("Status", format.ColorizeState(s))
			}
		}

		fmt.Println() // Trailing newline
	},
}

func outputStatusJSON(vehicleData, ecuData, battery0Data, battery1Data, auxBatteryData, cbBatteryData map[string]string) {
	// Helper function to parse int
	parseInt := func(s string) int {
		v, _ := strconv.Atoi(s)
		return v
	}

	// Helper function to parse float
	parseFloat := func(s string) float64 {
		v, _ := strconv.ParseFloat(s, 64)
		return v
	}

	// Build structured JSON output
	output := map[string]interface{}{
		"vehicle": map[string]interface{}{
			"state":     vehicleData["state"],
			"kickstand": vehicleData["kickstand"],
			"brakes": map[string]string{
				"left":  vehicleData["brake:left"],
				"right": vehicleData["brake:right"],
			},
			"blinker": func() string {
				if vehicleData["blinker:switch"] == "" {
					return "off"
				}
				return vehicleData["blinker:switch"]
			}(),
			"seatbox": func() string {
				if vehicleData["seatbox:lock"] == "" {
					return "closed"
				}
				return vehicleData["seatbox:lock"]
			}(),
		},
		"motor": map[string]interface{}{
			"speed_kph":     parseFloat(ecuData["speed"]),
			"rpm":           parseInt(ecuData["rpm"]),
			"throttle":      ecuData["throttle"] == "on",
			"odometer_km":   parseFloat(ecuData["odometer"]) / 1000.0,
			"voltage_v":     parseFloat(ecuData["motor:voltage"]) / 1000.0,
			"current_a":     parseFloat(ecuData["motor:current"]) / 1000.0,
			"temperature_c": parseInt(ecuData["temperature"]),
			"kers":          ecuData["kers"] == "on",
		},
	}

	// Add battery 0
	if battery0Data["present"] == "true" {
		output["battery_0"] = map[string]interface{}{
			"present":           true,
			"state":             battery0Data["state"],
			"charge_percent":    parseInt(battery0Data["charge"]),
			"voltage_v":         parseFloat(battery0Data["voltage"]) / 1000.0,
			"current_a":         parseFloat(battery0Data["current"]) / 1000.0,
			"temperature_c":     parseInt(battery0Data["temperature:0"]),
			"temperature_state": battery0Data["temperature-state"],
			"cycles":            parseInt(battery0Data["cycle-count"]),
			"health_percent":    parseInt(battery0Data["state-of-health"]),
		}
	} else {
		output["battery_0"] = map[string]interface{}{
			"present": false,
		}
	}

	// Add battery 1
	if battery1Data["present"] == "true" {
		output["battery_1"] = map[string]interface{}{
			"present":           true,
			"state":             battery1Data["state"],
			"charge_percent":    parseInt(battery1Data["charge"]),
			"voltage_v":         parseFloat(battery1Data["voltage"]) / 1000.0,
			"current_a":         parseFloat(battery1Data["current"]) / 1000.0,
			"temperature_c":     parseInt(battery1Data["temperature:0"]),
			"temperature_state": battery1Data["temperature-state"],
			"cycles":            parseInt(battery1Data["cycle-count"]),
			"health_percent":    parseInt(battery1Data["state-of-health"]),
		}
	} else {
		output["battery_1"] = map[string]interface{}{
			"present": false,
		}
	}

	// Add aux battery
	if len(auxBatteryData) > 0 {
		output["aux_battery"] = map[string]interface{}{
			"voltage_v":      parseFloat(auxBatteryData["voltage"]) / 1000.0,
			"charge_percent": parseInt(auxBatteryData["charge"]),
			"charge_status":  auxBatteryData["charge-status"],
		}
	}

	// Add cb battery
	if cbBatteryData["present"] == "true" {
		output["cb_battery"] = map[string]interface{}{
			"present":        true,
			"charge_percent": parseInt(cbBatteryData["charge"]),
			"charge_status":  cbBatteryData["charge-status"],
			"temperature_c":  parseInt(cbBatteryData["temperature"]),
			"voltage_v":      parseFloat(cbBatteryData["cell-voltage"]) / 1000000.0,
			"current_a":      parseFloat(cbBatteryData["current"]) / 1000000.0,
			"health_percent": parseInt(cbBatteryData["state-of-health"]),
			"cycles":         parseInt(cbBatteryData["cycle-count"]),
		}
	} else if len(cbBatteryData) > 0 {
		output["cb_battery"] = map[string]interface{}{
			"present": false,
		}
	}

	jsonBytes, _ := json.MarshalIndent(output, "", "  ")
	fmt.Println(string(jsonBytes))
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.GroupID = "main"
}
