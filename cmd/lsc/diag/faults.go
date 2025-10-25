package diag

import (
	"encoding/json"
	"fmt"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var faultsCmd = &cobra.Command{
	Use:   "faults",
	Short: "Show active faults",
	Long:  `Display all active faults from vehicle and battery systems.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Fetch faults from all sources
		vehicleFaults, err := RedisClient.SMembers("vehicle:fault")
		if err != nil {
			vehicleFaults = []string{}
		}

		battery0Faults, err := RedisClient.SMembers("battery:0:faults")
		if err != nil {
			battery0Faults = []string{}
		}

		battery1Faults, err := RedisClient.SMembers("battery:1:faults")
		if err != nil {
			battery1Faults = []string{}
		}

		totalFaults := len(vehicleFaults) + len(battery0Faults) + len(battery1Faults)

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.MarshalIndent(map[string]interface{}{
				"total_faults": totalFaults,
				"vehicle":      vehicleFaults,
				"battery_0":    battery0Faults,
				"battery_1":    battery1Faults,
			}, "", "  ")
			fmt.Println(string(output))
			return
		}

		if totalFaults == 0 {
			fmt.Println(format.Success("No active faults"))
			return
		}

		format.PrintSection(fmt.Sprintf("Active Faults (%d)", totalFaults))

		if len(vehicleFaults) > 0 {
			fmt.Println(format.Warning("\nVehicle Faults:"))
			for _, fault := range vehicleFaults {
				fmt.Printf("  %s %s\n", format.Error("•"), fault)
			}
		}

		if len(battery0Faults) > 0 {
			fmt.Println(format.Warning("\nBattery 0 Faults:"))
			for _, fault := range battery0Faults {
				fmt.Printf("  %s %s\n", format.Error("•"), fault)
			}
		}

		if len(battery1Faults) > 0 {
			fmt.Println(format.Warning("\nBattery 1 Faults:"))
			for _, fault := range battery1Faults {
				fmt.Printf("  %s %s\n", format.Error("•"), fault)
			}
		}

		fmt.Println()
	},
}

func init() {
	DiagCmd.AddCommand(faultsCmd)
}
