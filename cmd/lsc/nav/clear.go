package nav

import (
	"encoding/json"
	"fmt"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var navClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear the navigation destination",
	Long:  `Clear the current navigation destination and stop navigation on the dashboard.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fields := map[string]string{
			"destination": "",
			"latitude":    "",
			"longitude":   "",
			"address":     "",
			"timestamp":   "",
		}
		if err := setNavFields(fields); err != nil {
			return emitNavError("nav-clear", err)
		}

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]any{
				"command": "nav-clear",
				"status":  "success",
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success("Navigation destination cleared"))
		}
		return nil
	},
}
