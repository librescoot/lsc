package lsc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"librescoot/lsc/internal/cli"
	"librescoot/lsc/internal/format"
	"librescoot/lsc/internal/redis"

	"github.com/spf13/cobra"
)

var bluetoothCmd = &cobra.Command{
	Use:   "bluetooth",
	Short: "Inspect Bluetooth state and manage paired phones",
	Long:  `Show Bluetooth status and clear the phones paired with this scooter.`,
}

var bluetoothStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Bluetooth status",
	RunE: func(cmd *cobra.Command, args []string) error {
		fields := map[string]string{}
		for _, key := range []string{"status", "mac-address", "pin-code"} {
			value, err := redisClient.HGet("ble", key)
			if err != nil && !redis.IsNil(err) {
				if JSONOutput {
					output, _ := json.Marshal(map[string]interface{}{"error": err.Error()})
					fmt.Println(string(output))
				} else {
					fmt.Fprintf(os.Stderr, format.Error("Failed to read Bluetooth state: %v\n"), err)
				}
				return cli.ErrSilent
			}
			fields[key] = value
		}
		version, _ := redisClient.HGet("system", "nrf-fw-version")

		if JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"status":      fields["status"],
				"mac_address": fields["mac-address"],
				"pairing":     fields["pin-code"] != "",
				"fw_version":  version,
			})
			fmt.Println(string(output))
			return nil
		}

		format.PrintSection("Bluetooth")
		status := fields["status"]
		if status == "" {
			status = "unknown"
		}
		format.PrintKV("Status", format.ColorizeState(status))
		if fields["mac-address"] != "" {
			format.PrintKV("MAC", fields["mac-address"])
		}
		if version != "" {
			format.PrintKV("nRF firmware", version)
		}
		// The PIN is only present while a phone is partway through pairing,
		// so its presence is the useful signal rather than its value.
		if fields["pin-code"] != "" {
			format.PrintKV("Pairing", "in progress")
		}
		fmt.Println()
		return nil
	},
}

var bluetoothForgetYes bool

var bluetoothForgetCmd = &cobra.Command{
	Use:   "forget-all",
	Short: "Clear every phone paired with this scooter",
	Long: `Delete all Bluetooth bonds stored on the scooter.

Every paired phone has to pair again afterwards, including the one running
this command. Pairing needs the dashboard, which displays the passkey, so do
this while the scooter is parked rather than mid-ride.

There is no per-phone equivalent: the firmware accepts a single-bond delete
but does nothing with it, so this clears all of them or nothing.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !bluetoothForgetYes && !JSONOutput {
			fmt.Print("This unpairs every phone from this scooter. Continue? [y/N] ")
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			if !strings.EqualFold(strings.TrimSpace(answer), "y") {
				fmt.Println("Cancelled.")
				return nil
			}
		}

		if err := redisClient.LPush("scooter:bluetooth", "delete-all-bonds"); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "forget-all",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to clear bonds: %v\n"), err)
			}
			return cli.ErrSilent
		}

		if JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "forget-all",
				"status":  "sent",
			})
			fmt.Println(string(output))
			return nil
		}

		// The nRF stops advertising and drops the connected phone to do this,
		// so the result is visible on the phone rather than reported back.
		fmt.Println(format.Success("Cleared all paired phones."))
		fmt.Println("Phones will need to pair again from the dashboard.")
		return nil
	},
}

func init() {
	bluetoothForgetCmd.Flags().BoolVarP(&bluetoothForgetYes, "yes", "y", false, "Skip the confirmation prompt")

	bluetoothCmd.AddCommand(bluetoothStatusCmd)
	bluetoothCmd.AddCommand(bluetoothForgetCmd)
	rootCmd.AddCommand(bluetoothCmd)
}
