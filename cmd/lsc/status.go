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

		// If JSON output is requested, output structured JSON
		if JSONOutput {
			outputStatusJSON(vehicleData, ecuData, battery0Data, battery1Data)
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
		format.PrintSection("Battery 0")
		if battery0Data["present"] == "true" {
			format.PrintKV("State", format.ColorizeState(battery0Data["state"]))
			format.PrintKV("Charge", format.FormatChargeColored(battery0Data["charge"]))
			format.PrintKV("Voltage", format.FormatVoltageColored(battery0Data["voltage"]))
			format.PrintKV("Current", format.MilliampsToAmps(battery0Data["current"]))
			format.PrintKV("Temperature", format.FormatTemperatureColored(battery0Data["temperature:0"]))
			format.PrintKV("Temp State", format.ColorizeState(battery0Data["temperature-state"]))
			format.PrintKV("Cycles", format.SafeValueOr(battery0Data["cycle-count"], "0"))
			format.PrintKV("Health", format.FormatPercentage(battery0Data["state-of-health"]))
		} else {
			fmt.Println(format.Dim("  Not Present"))
		}

		// Display Battery 1 Status
		format.PrintSection("Battery 1")
		if battery1Data["present"] == "true" {
			format.PrintKV("State", format.ColorizeState(battery1Data["state"]))
			format.PrintKV("Charge", format.FormatChargeColored(battery1Data["charge"]))
			format.PrintKV("Voltage", format.FormatVoltageColored(battery1Data["voltage"]))
			format.PrintKV("Current", format.MilliampsToAmps(battery1Data["current"]))
			format.PrintKV("Temperature", format.FormatTemperatureColored(battery1Data["temperature:0"]))
			format.PrintKV("Temp State", format.ColorizeState(battery1Data["temperature-state"]))
			format.PrintKV("Cycles", format.SafeValueOr(battery1Data["cycle-count"], "0"))
			format.PrintKV("Health", format.FormatPercentage(battery1Data["state-of-health"]))
		} else {
			fmt.Println(format.Dim("  Not Present"))
		}

		fmt.Println() // Trailing newline
	},
}

func outputStatusJSON(vehicleData, ecuData, battery0Data, battery1Data map[string]string) {
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
			"state":      vehicleData["state"],
			"kickstand":  vehicleData["kickstand"],
			"brakes": map[string]string{
				"left":  vehicleData["brake:left"],
				"right": vehicleData["brake:right"],
			},
			"blinker": format.SafeValueOr(vehicleData["blinker:switch"], "off"),
			"seatbox": format.SafeValueOr(vehicleData["seatbox:lock"], "closed"),
		},
		"motor": map[string]interface{}{
			"speed_kph":       parseFloat(ecuData["speed"]),
			"rpm":             parseInt(ecuData["rpm"]),
			"throttle":        ecuData["throttle"] == "true",
			"odometer_km":     parseFloat(ecuData["odometer"]) / 1000.0,
			"voltage_v":       parseFloat(ecuData["motor:voltage"]) / 1000.0,
			"current_a":       parseFloat(ecuData["motor:current"]) / 1000.0,
			"temperature_c":   parseInt(ecuData["temperature"]),
			"kers":            ecuData["kers"] == "true",
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

	jsonBytes, _ := json.MarshalIndent(output, "", "  ")
	fmt.Println(string(jsonBytes))
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
