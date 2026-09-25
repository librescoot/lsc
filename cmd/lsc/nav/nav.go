package nav

import (
	"encoding/json"
	"fmt"
	"os"

	"librescoot/lsc/internal/cli"
	"librescoot/lsc/internal/format"
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

// NavCmd represents the nav command
var NavCmd = &cobra.Command{
	Use:     "nav",
	Aliases: []string{"navigation"},
	Short:   "Control dashboard navigation",
	Long: `Set, clear, and inspect the navigation destination shown on the dashboard.

The route plan is managed by settings-service and projected into the navigation hash.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return navStatusCmd.RunE(cmd, args)
	},
}

func emitNavError(command string, err error) error {
	if JSONOutput != nil && *JSONOutput {
		output, _ := json.Marshal(map[string]any{
			"command": command,
			"status":  "error",
			"error":   err.Error(),
		})
		fmt.Println(string(output))
	} else {
		fmt.Fprintf(os.Stderr, format.Error("%v\n"), err)
	}
	return cli.ErrSilent
}

func init() {
	NavCmd.AddCommand(navStatusCmd)
	NavCmd.AddCommand(navSetCmd)
	NavCmd.AddCommand(navClearCmd)
}
