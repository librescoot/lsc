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
	Long:  `Display all active faults from vehicle, ECU and battery systems.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Fetch faults from all sources
		vehicleFaults, err := RedisClient.SMembers("vehicle:fault")
		if err != nil {
			vehicleFaults = []string{}
		}

		ecuFaults, err := RedisClient.SMembers("engine-ecu:fault")
		if err != nil {
			ecuFaults = []string{}
		}

		battery0Faults, err := RedisClient.SMembers("battery:0:fault")
		if err != nil {
			battery0Faults = []string{}
		}

		battery1Faults, err := RedisClient.SMembers("battery:1:fault")
		if err != nil {
			battery1Faults = []string{}
		}

		totalFaults := len(vehicleFaults) + len(ecuFaults) + len(battery0Faults) + len(battery1Faults)

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.MarshalIndent(map[string]interface{}{
				"total_faults": totalFaults,
				"vehicle":      vehicleFaults,
				"engine_ecu":   ecuFaults,
				"battery_0":    battery0Faults,
				"battery_1":    battery1Faults,
			}, "", "  ")
			fmt.Println(string(output))
			return nil
		}

		if totalFaults == 0 {
			fmt.Println(format.Success("No active faults"))
			return nil
		}

		format.PrintSection(fmt.Sprintf("Active Faults (%d)", totalFaults))

		headers := []string{"SOURCE", "FAULT"}
		var rows [][]string

		if len(vehicleFaults) > 0 {
			for _, fault := range vehicleFaults {
				rows = append(rows, []string{"Vehicle", format.Error(fault)})
			}
		}

		if len(ecuFaults) > 0 {
			for _, fault := range ecuFaults {
				rows = append(rows, []string{"ECU", format.Error(fault)})
			}
		}

		if len(battery0Faults) > 0 {
			for _, fault := range battery0Faults {
				rows = append(rows, []string{"Battery 0", format.Error(fault)})
			}
		}

		if len(battery1Faults) > 0 {
			for _, fault := range battery1Faults {
				rows = append(rows, []string{"Battery 1", format.Error(fault)})
			}
		}

		format.PrintTable(headers, rows)
		return nil
	},
}

func init() {
	DiagCmd.AddCommand(faultsCmd)
}
