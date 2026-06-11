package modem

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"librescoot/lsc/internal/cli"
	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var modemStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show modem and internet connectivity status",
	Long:  `Display modem power/SIM state and internet connectivity details.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		modemData, err := RedisClient.HGetAll("modem")
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]any{"error": err.Error()})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to fetch modem data: %v\n"), err)
			}
			return cli.ErrSilent
		}

		internetData, _ := RedisClient.HGetAll("internet")

		if JSONOutput != nil && *JSONOutput {
			signalQuality, _ := strconv.Atoi(internetData["signal-quality"])
			output := map[string]any{
				"modem": map[string]any{
					"power_state":       modemData["power-state"],
					"sim_state":         modemData["sim-state"],
					"sim_lock":          modemData["sim-lock"],
					"operator_name":     modemData["operator-name"],
					"operator_code":     modemData["operator-code"],
					"is_roaming":        modemData["is-roaming"] == "true",
					"registration_fail": modemData["registration-fail"],
				},
				"internet": map[string]any{
					"modem_state":    internetData["modem-state"],
					"modem_health":   internetData["modem-health"],
					"status":         internetData["status"],
					"ip_address":     internetData["ip-address"],
					"access_tech":    internetData["access-tech"],
					"signal_quality": signalQuality,
					"sim_imei":       internetData["sim-imei"],
					"sim_iccid":      internetData["sim-iccid"],
					"sim_imsi":       internetData["sim-imsi"],
				},
			}
			jsonBytes, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(jsonBytes))
			return nil
		}

		format.PrintSection("Modem")
		format.PrintKV("Power", format.ColorizeState(format.SafeValueOr(modemData["power-state"], "unknown")))
		if v := internetData["modem-health"]; v != "" && v != "normal" {
			format.PrintKV("Health", format.Warning(v))
		}
		format.PrintKV("SIM", format.SafeValueOr(modemData["sim-state"], "unknown"))
		if v := modemData["sim-lock"]; v != "" && v != "disabled" {
			format.PrintKV("SIM Lock", format.Warning(v))
		}
		if v := modemData["operator-name"]; v != "" {
			operator := v
			if modemData["is-roaming"] == "true" {
				operator += " " + format.Warning("(roaming)")
			}
			format.PrintKV("Operator", operator)
		}
		if v := modemData["registration-fail"]; v != "" {
			format.PrintKV("Registration", format.Error(v))
		}

		format.PrintSection("Internet")
		format.PrintKV("Status", format.ColorizeState(format.SafeValueOr(internetData["status"], "unknown")))
		if v := internetData["access-tech"]; v != "" {
			format.PrintKV("Access Tech", v)
		}
		if v := internetData["signal-quality"]; v != "" {
			quality, _ := strconv.Atoi(v)
			format.PrintKV("Signal", format.ColorizePercentage(quality))
		}
		if v := internetData["ip-address"]; v != "" {
			format.PrintKV("IP Address", v)
		}
		if v := internetData["sim-imei"]; v != "" {
			format.PrintKV("IMEI", v)
		}
		if v := internetData["sim-iccid"]; v != "" {
			format.PrintKV("ICCID", v)
		}
		fmt.Println()
		return nil
	},
}
