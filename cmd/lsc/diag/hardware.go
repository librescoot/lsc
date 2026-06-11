package diag

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"librescoot/lsc/internal/cli"
	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var dashboardCmd = &cobra.Command{
	Use:     "dashboard [on|off]",
	Aliases: []string{"dbc", "dash"},
	Short:   "Control dashboard power and connectivity",
	Long:    `Control dashboard power (on/off) and check connectivity (ping, on-wait).`,
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// If no args, show help
		if len(args) == 0 {
			return cmd.Help()
		}

		action := args[0]

		if action != "on" && action != "off" {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "dashboard",
					"status":  "error",
					"error":   fmt.Sprintf("invalid action: %s", action),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Invalid action '%s'. Must be 'on' or 'off'\n"), action)
			}
			return cli.ErrSilent
		}

		// Build command with optional :force suffix
		command := fmt.Sprintf("dashboard:%s", action)
		if action == "off" && forceFlag {
			command = fmt.Sprintf("dashboard:%s:force", action)
		}
		if err := RedisClient.LPush("scooter:hardware", command); err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "dashboard",
					"action":  action,
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to send dashboard command: %v\n"), err)
			}
			return cli.ErrSilent
		}

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "dashboard",
				"action":  action,
				"status":  "success",
			})
			fmt.Println(string(output))
		} else {
			fmt.Printf("%s Dashboard power: %s\n", format.Success("✓"), action)
		}
		return nil
	},
}

var engineCmd = &cobra.Command{
	Use:       "engine [on|off]",
	Short:     "Control engine power",
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"on", "off"},
	RunE: func(cmd *cobra.Command, args []string) error {
		action := args[0]

		if action != "on" && action != "off" {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "engine",
					"status":  "error",
					"error":   fmt.Sprintf("invalid action: %s", action),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Invalid action '%s'. Must be 'on' or 'off'\n"), action)
			}
			return cli.ErrSilent
		}

		command := fmt.Sprintf("engine:%s", action)
		if err := RedisClient.LPush("scooter:hardware", command); err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "engine",
					"action":  action,
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to send engine command: %v\n"), err)
			}
			return cli.ErrSilent
		}

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "engine",
				"action":  action,
				"status":  "success",
			})
			fmt.Println(string(output))
		} else {
			fmt.Printf("%s Engine power: %s\n", format.Success("✓"), action)
		}
		return nil
	},
}

var (
	onWaitTimeout int
	forceFlag     bool
	pingFlag      bool
)

// startTimer starts a background goroutine that prints elapsed time every second.
// Call the returned stop function to clear the timer line and stop updates.
func startTimer(label string) func() {
	start := time.Now()
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				elapsed := time.Since(start).Truncate(time.Second)
				fmt.Printf("\r%s [%s]", label, elapsed)
			}
		}
	}()
	return func() {
		close(done)
		fmt.Print("\r\033[2K") // clear the timer line
	}
}

var dbcStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show DBC status (power, ready, display, features)",
	Long:  `Display dashboard power, ready state, serial, backlight, ambient brightness, and feature availability. Use --ping to also verify network reachability.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		dash, dashErr := RedisClient.HGetAll("dashboard")
		if dashErr != nil {
			dash = map[string]string{}
		}
		vehicle, vehErr := RedisClient.HGetAll("vehicle")
		if vehErr != nil {
			vehicle = map[string]string{}
		}

		power := vehicle["dashboard:power"]
		if power == "" {
			power = "unknown"
		}
		ready := dash["ready"] == "true"
		serial := dash["serial-number"]
		backlightEnabled := dash["backlight-enabled"] == "true"
		backlightRaw := dash["backlight"]
		brightnessLux, hasBrightness := parseFloat(dash["brightness"])
		mapsAvail := dash["maps-available"] == "true"
		navAvail := dash["navigation-available"] == "true"

		var pingOK bool
		var pingLatency time.Duration
		var pingChecked bool
		if pingFlag {
			pingChecked = true
			pingStart := time.Now()
			if err := exec.Command("ping", "-c", "1", "-W", "1", "192.168.7.2").Run(); err == nil {
				pingOK = true
				pingLatency = time.Since(pingStart)
			}
		}

		if JSONOutput != nil && *JSONOutput {
			output := map[string]interface{}{
				"power":                power,
				"ready":                ready,
				"serial_number":        serial,
				"backlight_enabled":    backlightEnabled,
				"backlight_pwm":        parseIntOrNil(backlightRaw),
				"maps_available":       mapsAvail,
				"navigation_available": navAvail,
			}
			if hasBrightness {
				output["ambient_lux"] = brightnessLux
			} else {
				output["ambient_lux"] = nil
			}
			if pingChecked {
				output["reachable"] = pingOK
				if pingOK {
					output["ping_ms"] = float64(pingLatency.Microseconds()) / 1000.0
				}
			}
			data, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(data))
			return nil
		}

		format.PrintSection("Dashboard Status")
		format.PrintKV("Power", format.ColorizeState(power))
		if ready {
			format.PrintKV("Ready", format.Success("yes"))
		} else {
			format.PrintKV("Ready", format.Dim("no"))
		}
		if pingChecked {
			if pingOK {
				format.PrintKV("Reachable", fmt.Sprintf("%s (%s)", format.Success("yes"), pingLatency.Round(time.Millisecond)))
			} else {
				format.PrintKV("Reachable", format.Error("no"))
			}
		}
		format.PrintKV("Serial", format.SafeValueOr(serial, "unknown"))

		format.PrintSection("Display")
		if backlightEnabled {
			if backlightRaw != "" {
				format.PrintKV("Backlight", fmt.Sprintf("%s (PWM %s)", format.Success("enabled"), backlightRaw))
			} else {
				format.PrintKV("Backlight", format.Success("enabled"))
			}
		} else {
			format.PrintKV("Backlight", format.Dim("disabled"))
		}
		if hasBrightness {
			format.PrintKV("Ambient", fmt.Sprintf("%.1f lux", brightnessLux))
		} else {
			format.PrintKV("Ambient", format.Dim("unknown"))
		}

		format.PrintSection("Features")
		if mapsAvail {
			format.PrintKV("Maps", format.Success("available"))
		} else {
			format.PrintKV("Maps", format.Dim("unavailable"))
		}
		if navAvail {
			format.PrintKV("Navigation", format.Success("available"))
		} else {
			format.PrintKV("Navigation", format.Dim("unavailable"))
		}
		fmt.Println()
		return nil
	},
}

func parseFloat(s string) (float64, bool) {
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseIntOrNil(s string) interface{} {
	if s == "" {
		return nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return v
}

var dbcPingCmd = &cobra.Command{
	Use:   "ping",
	Short: "Ping the DBC to check connectivity",
	Long:  `Ping the Dashboard Computer at 192.168.7.2 to verify network connectivity.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		pingCmd := exec.Command("ping", "192.168.7.2")
		pingCmd.Stdout = os.Stdout
		pingCmd.Stderr = os.Stderr
		pingCmd.Stdin = os.Stdin
		if err := pingCmd.Run(); err != nil {
			return cli.ErrSilent
		}
		return nil
	},
}

var dbcOnWaitCmd = &cobra.Command{
	Use:   "on-wait",
	Short: "Turn on DBC and wait until ready",
	Long:  `Send dashboard:on command and wait for the dashboard to publish 'ready' state, then verify with ping.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Check if DBC is already ready
		ready, err := RedisClient.HGet("dashboard", "ready")
		if err == nil && ready == "true" {
			fmt.Println("Dashboard already marked as ready, verifying with ping...")
			// Verify with ping
			pingCmd := exec.Command("ping", "-c", "1", "-W", "2", "192.168.7.2")
			if err := pingCmd.Run(); err == nil {
				fmt.Println("Dashboard is ready and reachable!")
				return nil
			}
			fmt.Println("Dashboard marked ready but not reachable, turning on...")
		}

		// Subscribe to dashboard channel before sending command
		pubsub := RedisClient.Subscribe(ctx, "dashboard")
		defer pubsub.Close()

		ch := pubsub.Channel()

		// Allow subscription to establish
		time.Sleep(100 * time.Millisecond)

		// Send dashboard:on command
		fmt.Println("Turning on dashboard...")
		err = RedisClient.LPush("scooter:hardware", "dashboard:on")
		if err != nil {
			fmt.Printf("Error sending dashboard:on command: %v\n", err)
			return cli.ErrSilent
		}

		// Wait for ready notification
		stopTimer := startTimer("Waiting for dashboard")
		timeoutChan := time.After(time.Duration(onWaitTimeout) * time.Second)
		startTime := time.Now()

		for {
			select {
			case msg := <-ch:
				// Check if it's a ready notification
				if msg.Payload == "ready" {
					// Verify ready state
					ready, err := RedisClient.HGet("dashboard", "ready")
					if err == nil && ready == "true" {
						stopTimer()
						elapsed := time.Since(startTime).Truncate(time.Millisecond)
						// Double-check with ping
						fmt.Println("Dashboard ready notification received, verifying with ping...")
						pingCmd := exec.Command("ping", "-c", "1", "-W", "2", "192.168.7.2")
						if err := pingCmd.Run(); err != nil {
							fmt.Println("Warning: Dashboard marked ready but not reachable via ping")
							stopTimer = startTimer("Waiting for dashboard")
							continue // Keep waiting
						}
						fmt.Printf("Dashboard is ready and reachable! (took %s)\n", elapsed)
						return nil
					}
				}
			case <-timeoutChan:
				stopTimer()
				fmt.Printf("Timeout: Dashboard did not become ready within %d seconds\n", onWaitTimeout)
				// Try one last ping check
				fmt.Println("Attempting final ping check...")
				pingCmd := exec.Command("ping", "-c", "1", "-W", "2", "192.168.7.2")
				if err := pingCmd.Run(); err == nil {
					ready, _ := RedisClient.HGet("dashboard", "ready")
					if ready == "true" {
						elapsed := time.Since(startTime).Truncate(time.Millisecond)
						fmt.Printf("Dashboard is ready and reachable (took %s, ready notification may have been missed)\n", elapsed)
						return nil
					}
				}
				return cli.ErrSilent
			}
		}
	},
}

var dbcOffWaitCmd = &cobra.Command{
	Use:   "off-wait",
	Short: "Turn off DBC and wait until unreachable",
	Long:  `Send dashboard:off command and wait for the DBC to become unreachable via ping.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Build command with optional :force suffix
		command := "dashboard:off"
		if forceFlag {
			command = "dashboard:off:force"
		}

		// Send dashboard:off command
		fmt.Println("Turning off dashboard...")
		err := RedisClient.LPush("scooter:hardware", command)
		if err != nil {
			fmt.Printf("Error sending dashboard:off command: %v\n", err)
			return cli.ErrSilent
		}

		// Wait for DBC to become unreachable
		startTime := time.Now()
		timeout := time.Duration(onWaitTimeout) * time.Second
		stopTimer := startTimer("Waiting for dashboard off")

		// Give it a moment to start shutting down
		time.Sleep(2 * time.Second)

		for {
			// Check if timeout exceeded
			if time.Since(startTime) > timeout {
				stopTimer()
				fmt.Printf("Timeout waiting for dashboard off after %d seconds\n", onWaitTimeout)
				return cli.ErrSilent
			}

			// Try to ping DBC
			pingCmd := exec.Command("ping", "-c", "1", "-W", "1", "192.168.7.2")
			err := pingCmd.Run()

			// If ping fails, DBC is unreachable (off)
			if err != nil {
				stopTimer()
				elapsed := time.Since(startTime).Truncate(time.Millisecond)
				fmt.Printf("Dashboard is off! (took %s)\n", elapsed)
				return nil
			}

			// Wait a bit before trying again
			time.Sleep(1 * time.Second)
		}
	},
}

func init() {
	dbcOnWaitCmd.Flags().IntVarP(&onWaitTimeout, "timeout", "t", 60, "Timeout in seconds to wait for DBC ready")
	dbcOffWaitCmd.Flags().IntVarP(&onWaitTimeout, "timeout", "t", 60, "Timeout in seconds to wait for DBC off")
	dbcStatusCmd.Flags().BoolVarP(&pingFlag, "ping", "p", false, "Also check network reachability via ping")

	// Add --force flag to dashboard command and subcommands that need it
	dashboardCmd.PersistentFlags().BoolVarP(&forceFlag, "force", "f", false, "Force dashboard off even if DBC update is in progress")

	dashboardCmd.AddCommand(dbcStatusCmd)
	dashboardCmd.AddCommand(dbcPingCmd)
	dashboardCmd.AddCommand(dbcOnWaitCmd)
	dashboardCmd.AddCommand(dbcOffWaitCmd)

	DiagCmd.AddCommand(dashboardCmd)
	DiagCmd.AddCommand(engineCmd)
}
