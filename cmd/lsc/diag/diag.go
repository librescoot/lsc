package diag

import (
	"librescoot/lsc/internal/redis"

	"github.com/spf13/cobra"
)

var DiagCmd = &cobra.Command{
	Use:   "diag",
	Short: "Diagnostic commands",
	Long:  `Diagnostic and detailed information about the scooter.`,
}

// Package-level variable to hold the Redis client reference
var RedisClient *redis.Client

// SetRedisClient sets the Redis client for all diag commands
func SetRedisClient(client *redis.Client) {
	RedisClient = client
}
