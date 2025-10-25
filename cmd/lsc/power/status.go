package power

import (
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
			fmt.Fprintf(os.Stderr, format.Error("Failed to fetch power-manager data: %v\n"), err)
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
