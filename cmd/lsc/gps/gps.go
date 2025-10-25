package gps

import (
	"librescoot/lsc/internal/redis"

	"github.com/spf13/cobra"
)

var RedisClient *redis.Client

// SetRedisClient allows the parent command to inject the Redis client
func SetRedisClient(client *redis.Client) {
	RedisClient = client
}

// GpsCmd represents the gps command
var GpsCmd = &cobra.Command{
	Use:   "gps",
	Short: "GPS status and tracking",
	Long:  `View GPS fix status, position, and accuracy information.`,
}
