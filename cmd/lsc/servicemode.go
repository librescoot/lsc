package lsc

import (
	"encoding/json"
	"fmt"
	"os"

	"librescoot/lsc/internal/cli"
	"librescoot/lsc/internal/redis"

	"github.com/spf13/cobra"
)

var serviceModeCmd = &cobra.Command{
	Use:     "service-mode",
	Short:   "Enable or disable Service mode (Servicemodus)",
	Long:    "Service mode disables auto-standby, auto-hibernate, the alarm, and handlebar auto-lock, keeps usb0 up, and shows the debug screen. It persists until disabled.",
	Aliases: []string{"servicemode", "svcmode"},
}

var serviceModeOnCmd = &cobra.Command{
	Use:   "on",
	Short: "Enable Service mode",
	RunE: func(cmd *cobra.Command, args []string) error {
		return overlayPush("apply:service", "on")
	},
}

var serviceModeOffCmd = &cobra.Command{
	Use:   "off",
	Short: "Disable Service mode",
	RunE: func(cmd *cobra.Command, args []string) error {
		return overlayPush("clear:service", "off")
	},
}

var serviceModeStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show whether Service mode is active",
	RunE: func(cmd *cobra.Command, args []string) error {
		val, err := redisClient.HGet("settings", "dashboard.service-mode-active")
		// A missing field means the overlay has never been applied: genuinely
		// off. Any other error is a transport/connection failure and must not
		// be reported as a definitive "off".
		if err != nil && !redis.IsNil(err) {
			if JSONOutput {
				out, _ := json.Marshal(map[string]any{"status": "error", "error": err.Error()})
				fmt.Println(string(out))
			} else {
				fmt.Fprintf(os.Stderr, "Failed to read Service mode status: %v\n", err)
			}
			return cli.ErrSilent
		}
		active := err == nil && val == "true"
		if JSONOutput {
			out, _ := json.Marshal(map[string]any{"active": active})
			fmt.Println(string(out))
			return nil
		}
		if active {
			fmt.Println("Service mode: ACTIVE")
		} else {
			fmt.Println("Service mode: off")
		}
		return nil
	},
}

func overlayPush(payload, verb string) error {
	if err := redisClient.LPush("settings:overlay", payload); err != nil {
		if JSONOutput {
			out, _ := json.Marshal(map[string]any{"command": verb, "status": "error", "error": err.Error()})
			fmt.Println(string(out))
		} else {
			fmt.Fprintf(os.Stderr, "Failed to send servicemode %s: %v\n", verb, err)
		}
		return cli.ErrSilent
	}
	if JSONOutput {
		out, _ := json.Marshal(map[string]any{"command": verb, "status": "success"})
		fmt.Println(string(out))
	} else {
		fmt.Printf("Service mode %s requested\n", verb)
	}
	return nil
}

func init() {
	serviceModeCmd.AddCommand(serviceModeOnCmd, serviceModeOffCmd, serviceModeStatusCmd)
	rootCmd.AddCommand(serviceModeCmd)
	serviceModeCmd.GroupID = "main"
}
