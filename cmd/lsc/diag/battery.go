package diag

import (
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

		for _, id := range batteryIDs {
			showBattery(id)
		}
	},
}

func showBattery(id string) {
	data, err := RedisClient.HGetAll(fmt.Sprintf("battery:%s", id))
	if err != nil {
		fmt.Fprintf(os.Stderr, format.Error("Failed to fetch battery:%s data: %v\n"), id, err)
		return
	}

	format.PrintSection(fmt.Sprintf("Battery %s", id))

	// Check if battery is present
	if data["present"] != "true" {
		fmt.Println(format.Dim("  Not Present\n"))
		return
	}

	// Basic status
	format.PrintKV("State", format.ColorizeState(data["state"]))
	format.PrintKV("Present", format.FormatPresence(data["present"]))

	// Charge information
	format.PrintSubsection("Charge")
	format.PrintKV("Level", format.FormatChargeColored(data["charge"]))
	format.PrintKV("Voltage", format.FormatVoltageColored(data["voltage"]))
	format.PrintKV("Current", format.MilliampsToAmps(data["current"]))

	// Temperature information
	format.PrintSubsection("Temperature")
	format.PrintKV("Sensor 0", format.FormatTemperatureColored(data["temperature:0"]))
	format.PrintKV("Sensor 1", format.FormatTemperatureColored(data["temperature:1"]))
	format.PrintKV("Sensor 2", format.FormatTemperatureColored(data["temperature:2"]))
	format.PrintKV("Sensor 3", format.FormatTemperatureColored(data["temperature:3"]))
	format.PrintKV("State", format.ColorizeState(data["temperature-state"]))

	// Health information
	format.PrintSubsection("Health")
	format.PrintKV("Cycle Count", format.SafeValueOr(data["cycle-count"], "0"))
	soh, _ := strconv.Atoi(data["state-of-health"])
	if soh > 0 {
		format.PrintKV("State of Health", format.ColorizePercentage(soh))
	} else {
		format.PrintKV("State of Health", format.Dim("N/A"))
	}

	// Identity
	format.PrintSubsection("Identity")
	format.PrintKV("Serial Number", format.SafeValueOr(data["serial-number"], "N/A"))
	format.PrintKV("Mfg Date", format.SafeValueOr(data["manufacturing-date"], "N/A"))
	format.PrintKV("Firmware", format.SafeValueOr(data["fw-version"], "N/A"))

	// Faults
	faults, err := RedisClient.SMembers(fmt.Sprintf("battery:%s:faults", id))
	if err == nil && len(faults) > 0 {
		format.PrintSubsection("Active Faults")
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
