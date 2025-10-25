package power

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

// PowerCmd represents the power command
var PowerCmd = &cobra.Command{
	Use:   "power",
	Short: "Power management and status",
	Long:  `View power manager status and control power states (run, suspend, hibernate).`,
}
