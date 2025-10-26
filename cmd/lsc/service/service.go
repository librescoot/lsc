package service

import (
	"librescoot/lsc/internal/redis"
	"strings"

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

// ensureServiceSuffix adds .service suffix if not present
func ensureServiceSuffix(name string) string {
	if strings.HasSuffix(name, ".service") {
		return name
	}
	return name + ".service"
}

// ServiceCmd represents the service command
var ServiceCmd = &cobra.Command{
	Use:   "service",
	Short: "Manage systemd services",
	Long:  `Start, stop, restart, enable, disable, and view logs of LibreScoot systemd services.`,
	Aliases: []string{"svc"},
}
