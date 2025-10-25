package lsc

import (
	"fmt"
	"io"
	"log"
	"os"

	"librescoot/lsc/cmd/lsc/diag"
	"librescoot/lsc/cmd/lsc/gps"
	"librescoot/lsc/cmd/lsc/ota"
	"librescoot/lsc/cmd/lsc/power"
	"librescoot/lsc/internal/redis"

	"github.com/spf13/cobra"
)

var (
	redisClient *redis.Client
	redisAddr   string
	JSONOutput  bool // Global flag for JSON output mode
)

func init() {
	// Suppress all default log output (Redis client uses this)
	log.SetOutput(io.Discard)

	rootCmd.PersistentFlags().StringVar(&redisAddr, "redis-addr", "192.168.7.1:6379", "Redis server address (host:port)")
	rootCmd.PersistentFlags().BoolVar(&JSONOutput, "json", false, "Output in JSON format")

	// Add subcommands
	rootCmd.AddCommand(diag.DiagCmd)
	rootCmd.AddCommand(gps.GpsCmd)
	rootCmd.AddCommand(ota.OTACmd)
	rootCmd.AddCommand(power.PowerCmd)
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
  • OTA updates (status and installation)
  • GPS tracking and monitoring
  • Battery diagnostics and status
  • Alarm system control
  • Hardware control (dashboard, engine, handlebar, seatbox)
  • Settings management
  • Fault monitoring and event streaming

All commands support JSON output mode (--json) for automation and scripting.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
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

		// Make Redis client available to subcommands
		diag.SetRedisClient(redisClient)
		gps.SetRedisClient(redisClient)
		ota.SetRedisClient(redisClient)
		power.SetRedisClient(redisClient)

		// Make JSONOutput flag available to subcommands
		diag.SetJSONOutput(&JSONOutput)
		gps.SetJSONOutput(&JSONOutput)
		ota.SetJSONOutput(&JSONOutput)
		power.SetJSONOutput(&JSONOutput)

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
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}
