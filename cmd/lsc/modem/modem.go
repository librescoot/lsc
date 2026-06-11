package modem

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

// ModemCmd represents the modem command
var ModemCmd = &cobra.Command{
	Use:   "modem",
	Short: "Modem status and power control",
	Long: `View modem and internet connectivity status and control modem power.

Note: modem-service auto-enables the modem in driving states, so 'modem off'
only sticks while the vehicle stays locked.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return modemStatusCmd.RunE(cmd, args)
	},
}

func sendModemCommand(name, payload, successMsg string) error {
	if err := RedisClient.LPush("scooter:modem", payload); err != nil {
		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]any{
				"command": name,
				"status":  "error",
				"error":   err.Error(),
			})
			fmt.Println(string(output))
		} else {
			fmt.Fprintf(os.Stderr, format.Error("Failed to send modem %s command: %v\n"), payload, err)
		}
		return cli.ErrSilent
	}

	if JSONOutput != nil && *JSONOutput {
		output, _ := json.Marshal(map[string]any{
			"command": name,
			"status":  "sent",
		})
		fmt.Println(string(output))
	} else {
		fmt.Println(format.Success(successMsg))
	}
	return nil
}

var modemOnCmd = &cobra.Command{
	Use:     "on",
	Aliases: []string{"enable"},
	Short:   "Enable the modem",
	Long:    `Request modem-service to power the modem on.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return sendModemCommand("modem-on", "enable", "Modem enable command sent")
	},
}

var modemOffCmd = &cobra.Command{
	Use:     "off",
	Aliases: []string{"disable"},
	Short:   "Disable the modem",
	Long: `Request modem-service to power the modem off. The modem is re-enabled
automatically when the vehicle enters a driving state.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return sendModemCommand("modem-off", "disable", "Modem disable command sent")
	},
}

func init() {
	ModemCmd.AddCommand(modemStatusCmd)
	ModemCmd.AddCommand(modemOnCmd)
	ModemCmd.AddCommand(modemOffCmd)
}
