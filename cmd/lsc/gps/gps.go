package gps

import (
	"librescoot/lsc/internal/redis"

	"github.com/spf13/cobra"
)

var RedisClient *redis.Client
var JSONOutput *bool

// SetRedisClient allows the parent command to inject the Redis client
func SetRedisClient(client *redis.Client) {
	RedisClient = client
}

// SetJSONOutput allows the parent command to inject the JSON output flag
func SetJSONOutput(jsonOutput *bool) {
	JSONOutput = jsonOutput
}

// GpsCmd represents the gps command
var GpsCmd = &cobra.Command{
	Use:   "gps",
	Short: "GPS status and tracking",
	Long:  `View GPS fix status, position, and accuracy information.`,
}
