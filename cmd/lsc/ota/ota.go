package ota

import (
	"librescoot/lsc/internal/redis"

	"github.com/spf13/cobra"
)

var (
	RedisClient *redis.Client
	JSONOutput  *bool
)

var OTACmd = &cobra.Command{
	Use:   "ota",
	Short: "OTA update management",
	Long:  `Manage over-the-air (OTA) updates using Mender.`,
}

// SetRedisClient sets the Redis client for all ota commands
func SetRedisClient(client *redis.Client) {
	RedisClient = client
}

// SetJSONOutput sets the JSON output flag reference for all ota commands
func SetJSONOutput(jsonOutput *bool) {
	JSONOutput = jsonOutput
}

func init() {
}
