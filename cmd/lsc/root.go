package lsc

import (
	"fmt"
	"io"
	"log"
	"os"

	"librescoot/lsc/cmd/lsc/diag"
	"librescoot/lsc/internal/redis"

	"github.com/spf13/cobra"
)

var (
	redisClient *redis.Client
	redisAddr   string
)

func init() {
	// Suppress all default log output (Redis client uses this)
	log.SetOutput(io.Discard)

	rootCmd.PersistentFlags().StringVar(&redisAddr, "redis-addr", "192.168.7.1:6379", "Redis server address (host:port)")

	// Add diag subcommand
	rootCmd.AddCommand(diag.DiagCmd)
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "lsc",
	Short: "lsc - librescoot control CLI",
	Long: `A command-line interface for controlling and monitoring
librescoot ECUs and services via Redis.

It abstracts away the direct Redis commands, providing a user-friendly
interface for common operations.`,
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

		// Make Redis client available to diag subcommands
		diag.SetRedisClient(redisClient)

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
