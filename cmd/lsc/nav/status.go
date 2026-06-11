package nav

import (
	"encoding/json"
	"fmt"
	"os"

	"librescoot/lsc/internal/cli"
	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var navStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the current navigation destination",
	Long:  `Display the navigation hash contents and dashboard navigation availability.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		navData, err := RedisClient.HGetAll("navigation")
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]any{"error": err.Error()})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to fetch navigation data: %v\n"), err)
			}
			return cli.ErrSilent
		}

		dashData, _ := RedisClient.HGetAll("dashboard")
		active := navData["destination"] != ""

		if JSONOutput != nil && *JSONOutput {
			output := map[string]any{
				"active":               active,
				"destination":          navData["destination"],
				"address":              navData["address"],
				"navigation_available": dashData["navigation-available"] == "true",
			}
			jsonBytes, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(jsonBytes))
			return nil
		}

		format.PrintSection("Navigation")
		if !active {
			format.PrintKV("Destination", format.Dim("(not set)"))
		} else if addr := navData["address"]; addr != "" {
			format.PrintKV("Destination", fmt.Sprintf("%s (%s)", addr, navData["destination"]))
		} else {
			format.PrintKV("Destination", navData["destination"])
		}
		if v := dashData["navigation-available"]; v != "" {
			format.PrintKV("Routing Available", format.ColorizeState(v))
		}
		fmt.Println()
		return nil
	},
}
