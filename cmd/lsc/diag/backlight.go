package diag

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"librescoot/lsc/internal/cli"
	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var backlightCmd = &cobra.Command{
	Use:       "backlight [on|off]",
	Short:     "Control dashboard backlight override",
	Long:      `Turn the dashboard backlight on or off. "off" sets an override that forces brightness to 0. "on" clears the override and resumes automatic brightness.`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"on", "off"},
	RunE: func(cmd *cobra.Command, args []string) error {
		action := args[0]

		if action != "on" && action != "off" {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "backlight",
					"status":  "error",
					"error":   fmt.Sprintf("invalid action: %s", action),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Invalid action '%s'. Must be 'on' or 'off'\n"), action)
			}
			return cli.ErrSilent
		}

		value := "true"
		if action == "off" {
			value = "false"
		}

		if err := RedisClient.HSet("dashboard", "backlight-enabled", value); err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "backlight",
					"action":  action,
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to set backlight override: %v\n"), err)
			}
			return cli.ErrSilent
		}

		if err := RedisClient.Publish(context.Background(), "dashboard", "backlight-enabled"); err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "backlight",
					"action":  action,
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to set backlight override: %v\n"), err)
			}
			return cli.ErrSilent
		}

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "backlight",
				"action":  action,
				"status":  "success",
			})
			fmt.Println(string(output))
		} else {
			fmt.Printf("%s Backlight: %s\n", format.Success("✓"), action)
		}
		return nil
	},
}

func init() {
	DiagCmd.AddCommand(backlightCmd)
}
