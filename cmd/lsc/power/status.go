package power

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
	Short: "Show power management status",
	Long:  `Display current power manager state, battery levels, and inhibitor status.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Fetch power manager data
		pmData, err := RedisClient.HGetAll("power-manager")
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"error": err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to fetch power-manager data: %v\n"), err)
			}
			return
		}

		// Fetch power mux data
		pmuxData, _ := RedisClient.HGetAll("power-mux")

		// Fetch aux battery data
		auxBattery, _ := RedisClient.HGetAll("aux-battery")

		// Fetch cb battery data
		cbBattery, _ := RedisClient.HGetAll("cb-battery")

		// Fetch inhibitors
		inhibitors, _ := RedisClient.SMembers("power-manager:busy-services")

		// JSON output
		if JSONOutput != nil && *JSONOutput {
			parseInt := func(s string) int {
				v, _ := strconv.Atoi(s)
				return v
			}
			parseFloat := func(s string) float64 {
				v, _ := strconv.ParseFloat(s, 64)
				return v
			}

			output := map[string]interface{}{
				"power_manager": map[string]interface{}{
					"state":        pmData["state"],
					"power_source": pmuxData["selected-input"],
					"inhibitors":   inhibitors,
				},
			}

			if len(auxBattery) > 0 {
				output["aux_battery"] = map[string]interface{}{
					"voltage_v":      parseFloat(auxBattery["voltage"]) / 1000.0,
					"charge_percent": parseInt(auxBattery["charge"]),
					"charge_status":  auxBattery["charge-status"],
				}
			}

			if len(cbBattery) > 0 && cbBattery["present"] == "true" {
				output["cb_battery"] = map[string]interface{}{
					"present":        true,
					"charge_percent": parseInt(cbBattery["charge"]),
					"charge_status":  cbBattery["charge-status"],
					"health_percent": parseInt(cbBattery["state-of-health"]),
					"cycles":         parseInt(cbBattery["cycle-count"]),
					"temperature_c":  parseInt(cbBattery["temperature"]),
				}
			} else {
				output["cb_battery"] = map[string]interface{}{
					"present": false,
				}
			}

			jsonBytes, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(jsonBytes))
			return
		}

		// Display power manager status
		format.PrintSection("Power Manager")

		state := pmData["state"]
		if state != "" {
			format.PrintKV("State", format.ColorizeState(state))
		} else {
			format.PrintKV("State", format.Warning("Unknown"))
		}

		// Power source
		if pmuxData["selected-input"] != "" {
			selectedInput := pmuxData["selected-input"]
			format.PrintKV("Power Source", formatPowerSource(selectedInput))
		}

		// Inhibitors
		if len(inhibitors) > 0 {
			format.PrintSubsection("Active Inhibitors")
			for _, inh := range inhibitors {
				fmt.Printf("  %s %s\n", format.Warning("•"), inh)
			}
		} else {
			format.PrintKV("Inhibitors", format.Success("None"))
		}

		// Auxiliary batteries
		if len(auxBattery) > 0 {
			format.PrintSection("Auxiliary Battery")

			voltage := auxBattery["voltage"]
			if voltage != "" {
				voltageVal, _ := strconv.Atoi(voltage)
				format.PrintKV("Voltage", format.FormatVoltageColored(voltage))

				// Typical 12V battery ranges
				if voltageVal > 12500 {
					// Good voltage for 12V system
				} else if voltageVal > 11000 {
					// Low voltage warning
				}
			}

			charge := auxBattery["charge"]
			if charge != "" {
				chargeVal, _ := strconv.Atoi(charge)
				format.PrintKV("Charge", format.ColorizePercentage(chargeVal))
			}

			chargeStatus := auxBattery["charge-status"]
			if chargeStatus != "" {
				format.PrintKV("Status", format.ColorizeState(chargeStatus))
			}
		}

		if len(cbBattery) > 0 && cbBattery["present"] == "true" {
			format.PrintSection("Control Board Battery")

			charge := cbBattery["charge"]
			if charge != "" {
				chargeVal, _ := strconv.Atoi(charge)
				format.PrintKV("Charge", format.ColorizePercentage(chargeVal))
			}

			chargeStatus := cbBattery["charge-status"]
			if chargeStatus != "" {
				format.PrintKV("Status", format.ColorizeState(chargeStatus))
			}

			soh := cbBattery["state-of-health"]
			if soh != "" {
				sohVal, _ := strconv.Atoi(soh)
				format.PrintKV("Health", format.ColorizePercentage(sohVal))
			}

			cycleCount := cbBattery["cycle-count"]
			if cycleCount != "" {
				format.PrintKV("Cycles", cycleCount)
			}

			temp := cbBattery["temperature"]
			if temp != "" {
				format.PrintKV("Temperature", format.FormatTemperatureColored(temp))
			}
		}

		fmt.Println()
	},
}

func formatPowerSource(source string) string {
	switch source {
	case "aux":
		return "Auxiliary Battery"
	case "main":
		return "Main Battery"
	case "external":
		return format.Success("External Power")
	default:
		return source
	}
}

func init() {
	PowerCmd.AddCommand(statusCmd)
}
