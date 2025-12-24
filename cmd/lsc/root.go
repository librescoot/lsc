package lsc

import (
	"fmt"
	"io"
	"log"
	"os"

	"librescoot/lsc/cmd/lsc/diag"
	"librescoot/lsc/cmd/lsc/gps"
	"librescoot/lsc/cmd/lsc/keycard"
	"librescoot/lsc/cmd/lsc/locations"
	"librescoot/lsc/cmd/lsc/logs"
	"librescoot/lsc/cmd/lsc/monitor"
	"librescoot/lsc/cmd/lsc/ota"
	"librescoot/lsc/cmd/lsc/power"
	"librescoot/lsc/cmd/lsc/service"
	"librescoot/lsc/internal/redis"
	"librescoot/lsc/internal/tui"

	"github.com/spf13/cobra"
)

var (
	version     string
	redisClient *redis.Client
	redisAddr   string
	JSONOutput  bool // Global flag for JSON output mode
	Verbose     bool // Global flag for verbose logging
)

func init() {
	// Suppress all default log output (Redis client uses this)
	log.SetOutput(io.Discard)

	rootCmd.PersistentFlags().StringVar(&redisAddr, "redis-addr", "192.168.7.1:6379", "Redis server address (host:port)")
	rootCmd.PersistentFlags().BoolVar(&JSONOutput, "json", false, "Output in JSON format")
	rootCmd.PersistentFlags().BoolVarP(&Verbose, "verbose", "v", false, "Enable verbose logging")

	// Add subcommands
	rootCmd.AddCommand(diag.DiagCmd)
	rootCmd.AddCommand(gps.GpsCmd)
	rootCmd.AddCommand(keycard.KeycardCmd)
	rootCmd.AddCommand(locations.LocationsCmd)
	rootCmd.AddCommand(logs.LogsCmd)
	rootCmd.AddCommand(monitor.MonitorCmd)
	rootCmd.AddCommand(ota.OTACmd)
	rootCmd.AddCommand(power.PowerCmd)
	rootCmd.AddCommand(service.ServiceCmd)
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "lsc",
	Short: "lsc - librescoot control CLI",
	Long: `lsc is a command-line interface for controlling and monitoring LibreScoot
electric scooters via Redis.

It provides convenient access to:
  • Vehicle state management (lock/unlock, hibernate, force-lock)
  • LED control (cues and fade animations)
  • Power management (run/suspend/hibernate/reboot states)
  • Service management (start/stop/restart/enable/disable services, view logs)
  • OTA updates (status and installation)
  • GPS tracking and monitoring
  • Battery diagnostics and status
  • Alarm system control
  • Hardware control (dashboard, engine, handlebar, seatbox)
  • Settings management
  • Fault monitoring and event streaming

All commands support JSON output mode (--json) for automation and scripting.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if Verbose {
			fmt.Fprintf(os.Stderr, "lsc version %s starting\n", version)
			fmt.Fprintf(os.Stderr, "Connecting to Redis at %s\n", redisAddr)
		}

		// Temporarily suppress stderr to hide redis library warnings
		oldStderr := os.Stderr
		devNull, _ := os.Open(os.DevNull)
		os.Stderr = devNull

		redisClient = redis.NewClient(redisAddr)
		err := redisClient.Connect()

		// Restore stderr
		os.Stderr = oldStderr
		devNull.Close()

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error connecting to Redis: %v\n", err)
			return err
		}

		if Verbose {
			fmt.Fprintf(os.Stderr, "Successfully connected to Redis\n")
		}

		// Make Redis client available to subcommands
		diag.SetRedisClient(redisClient)
		gps.SetRedisClient(redisClient)
		keycard.SetRedisClient(redisClient)
		locations.SetRedisClient(redisClient)
		logs.SetRedisClient(redisClient)
		monitor.SetRedisClient(redisClient)
		ota.SetRedisClient(redisClient)
		power.SetRedisClient(redisClient)
		service.SetRedisClient(redisClient)

		// Make JSONOutput flag available to subcommands
		diag.SetJSONOutput(&JSONOutput)
		gps.SetJSONOutput(&JSONOutput)
		keycard.SetJSONOutput(&JSONOutput)
		locations.SetJSONOutput(&JSONOutput)
		logs.SetJSONOutput(&JSONOutput)
		monitor.SetJSONOutput(&JSONOutput)
		ota.SetJSONOutput(&JSONOutput)
		power.SetJSONOutput(&JSONOutput)
		service.SetJSONOutput(&JSONOutput)

		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		if redisClient != nil {
			redisClient.Close()
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute(v string) {
	version = v
	rootCmd.Version = version

	// Check if TUI should launch (no args + TTY + no special env vars)
	if tui.ShouldLaunchTUI(os.Args[1:]) {
		// Connect to Redis for TUI
		redisClient = redis.NewClient(redisAddr)
		if err := redisClient.Connect(); err != nil {
			fmt.Fprintf(os.Stderr, "Error connecting to Redis: %v\n", err)
			os.Exit(1)
		}
		defer redisClient.Close()

		// Launch TUI
		if err := tui.Launch(redisAddr, redisClient); err != nil {
			fmt.Fprintf(os.Stderr, "Error running TUI: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Otherwise, run normal Cobra CLI
	cobra.CheckErr(rootCmd.Execute())
}
