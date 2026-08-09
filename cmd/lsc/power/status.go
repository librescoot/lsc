package power

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"

	"librescoot/lsc/internal/cli"
	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var statusShowAllInhibitors bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show power management status",
	Long:  `Display current power manager state, battery levels, and inhibitor status.`,
	RunE: func(cmd *cobra.Command, args []string) error {
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
			return cli.ErrSilent
		}

		// Fetch power mux data
		pmuxData, _ := RedisClient.HGetAll("power-mux")

		// Fetch aux battery data
		auxBattery, _ := RedisClient.HGetAll("aux-battery")

		// Fetch cb battery data
		cbBattery, _ := RedisClient.HGetAll("cb-battery")

		// Fetch inhibitors from both sources
		inhibitors, err := RedisClient.HGetAll("power-manager:busy-services")
		if err != nil {
			fmt.Fprintf(os.Stderr, format.Warning("Failed to fetch inhibitors: %v\n"), err)
			inhibitors = map[string]string{}
		}

		// Fetch external inhibitors (e.g. from update-service)
		externalInhibits, _ := RedisClient.HGetAll("power:inhibits")

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
					"state":               pmData["state"],
					"power_source":        pmuxData["selected-input"],
					"inhibitors":          inhibitors,
					"external_inhibitors": externalInhibits,
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
					"present":               true,
					"charge_percent":        parseInt(cbBattery["charge"]),
					"charge_status":         cbBattery["charge-status"],
					"health_percent":        parseInt(cbBattery["state-of-health"]),
					"cycles":                parseInt(cbBattery["cycle-count"]),
					"temperature_c":         parseInt(cbBattery["temperature"]),
					"voltage_v":             parseFloat(cbBattery["cell-voltage"]) / 1000000.0,
					"current_a":             parseFloat(cbBattery["current"]) / 1000000.0,
					"remaining_capacity_wh": parseFloat(cbBattery["remaining-capacity"]) / 1000000.0,
					"full_capacity_wh":      parseFloat(cbBattery["full-capacity"]) / 1000000.0,
					"time_to_empty_s":       parseInt(cbBattery["time-to-empty"]),
					"time_to_full_s":        parseInt(cbBattery["time-to-full"]),
				}
			} else {
				output["cb_battery"] = map[string]interface{}{
					"present": false,
				}
			}

			jsonBytes, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(jsonBytes))
			return nil
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

		// Inhibitors. power-manager:busy-services is pm-service's full published
		// view and already includes anything synced in from power:inhibits, so
		// by default only that one source is shown to avoid printing the same
		// inhibitor twice. --all additionally shows the raw power:inhibits hash
		// as its own section, since it can diverge from busy-services if
		// pm-service is down or its Redis sync is stalled (busy-services then
		// keeps its last published value instead of reflecting reality).
		blockingInhibitors := make(map[string]string)
		advisoryInhibitors := make(map[string]string)
		for who, typ := range inhibitors {
			if typ == "delay" {
				advisoryInhibitors[who] = typ
			} else {
				blockingInhibitors[who] = typ
			}
		}

		sortedKeys := func(m map[string]string) []string {
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			return keys
		}

		hasBusyList := len(blockingInhibitors) > 0 || (statusShowAllInhibitors && len(advisoryInhibitors) > 0)
		hasExternalList := statusShowAllInhibitors && len(externalInhibits) > 0
		hasInhibitors := hasBusyList || hasExternalList

		if hasInhibitors {
			if hasBusyList {
				format.PrintSubsection("Active Inhibitors")
				for _, who := range sortedKeys(blockingInhibitors) {
					fmt.Printf("  %s %s %s\n", format.Warning("•"), who, format.Dim("("+blockingInhibitors[who]+")"))
				}
				if statusShowAllInhibitors {
					for _, who := range sortedKeys(advisoryInhibitors) {
						fmt.Printf("  %s %s %s\n", format.Dim("•"), who, format.Dim("(advisory, does not block)"))
					}
				}
			}

			if hasExternalList {
				format.PrintSubsection("Raw power:inhibits (as written by clients)")
				for _, name := range sortedKeys(externalInhibits) {
					fmt.Printf("  %s %s %s\n", format.Warning("•"), name, format.Dim("("+externalInhibits[name]+")"))
				}
			}
		} else {
			format.PrintKV("Inhibitors", format.Success("None"))
		}

		// Auxiliary batteries
		if len(auxBattery) > 0 {
			format.PrintSection("Auxiliary Battery")

			voltage := auxBattery["voltage"]
			if voltage != "" {
				format.PrintKV("Voltage", format.FormatVoltageColored(voltage))
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
			format.PrintSection("Connectivity Battery Box")

			charge := cbBattery["charge"]
			if charge != "" {
				chargeVal, _ := strconv.Atoi(charge)
				format.PrintKV("Charge", format.ColorizePercentage(chargeVal))
			}

			if v := cbBattery["cell-voltage"]; v != "" {
				format.PrintKV("Voltage", format.FormatCBVoltageColored(v))
			}
			if c := cbBattery["current"]; c != "" {
				format.PrintKV("Current", format.MicroampsToAmps(c))
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

			if rc := format.MicrowattHoursToWattHours(cbBattery["remaining-capacity"]); rc != "" {
				if fc := format.MicrowattHoursToWattHours(cbBattery["full-capacity"]); fc != "" {
					format.PrintKV("Capacity", rc+" / "+fc)
				} else {
					format.PrintKV("Capacity", rc)
				}
			}
			if cbBattery["charge-status"] == "charging" {
				if t := format.SecondsToHuman(cbBattery["time-to-full"]); t != "" {
					format.PrintKV("Time to full", t)
				}
			} else if t := format.SecondsToHuman(cbBattery["time-to-empty"]); t != "" {
				format.PrintKV("Time to empty", t)
			}
		}

		fmt.Println()
		return nil
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
	statusCmd.Flags().BoolVar(&statusShowAllInhibitors, "all", false, "Also show advisory (delay) inhibitors, which block nothing, and the raw power:inhibits hash")

	PowerCmd.AddCommand(statusCmd)
}
