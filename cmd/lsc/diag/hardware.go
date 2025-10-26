package diag

import (
	"encoding/json"
	"fmt"
	"os"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var dashboardCmd = &cobra.Command{
	Use:       "dashboard [on|off]",
	Aliases:   []string{"dbc", "dash"},
	Short:     "Control dashboard power",
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"on", "off"},
	Run: func(cmd *cobra.Command, args []string) {
		action := args[0]

		if action != "on" && action != "off" {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "dashboard",
					"status":  "error",
					"error":   fmt.Sprintf("invalid action: %s", action),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Invalid action '%s'. Must be 'on' or 'off'\n"), action)
			}
			return
		}

		command := fmt.Sprintf("dashboard:%s", action)
		if err := RedisClient.LPush("scooter:hardware", command); err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "dashboard",
					"action":  action,
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to send dashboard command: %v\n"), err)
			}
			return
		}

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "dashboard",
				"action":  action,
				"status":  "success",
			})
			fmt.Println(string(output))
		} else {
			fmt.Printf("%s Dashboard power: %s\n", format.Success("✓"), action)
		}
	},
}

var engineCmd = &cobra.Command{
	Use:       "engine [on|off]",
	Short:     "Control engine power",
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"on", "off"},
	Run: func(cmd *cobra.Command, args []string) {
		action := args[0]

		if action != "on" && action != "off" {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "engine",
					"status":  "error",
					"error":   fmt.Sprintf("invalid action: %s", action),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Invalid action '%s'. Must be 'on' or 'off'\n"), action)
			}
			return
		}

		command := fmt.Sprintf("engine:%s", action)
		if err := RedisClient.LPush("scooter:hardware", command); err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "engine",
					"action":  action,
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to send engine command: %v\n"), err)
			}
			return
		}

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "engine",
				"action":  action,
				"status":  "success",
			})
			fmt.Println(string(output))
		} else {
			fmt.Printf("%s Engine power: %s\n", format.Success("✓"), action)
		}
	},
}

func init() {
	DiagCmd.AddCommand(dashboardCmd)
	DiagCmd.AddCommand(engineCmd)
}
