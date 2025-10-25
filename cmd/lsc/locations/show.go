package locations

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:     "show <id>",
	Aliases: []string{"get"},
	Short:   "Show details of a saved location",
	Long:    `Display detailed information about a specific saved location.`,
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Parse ID
		id, err := strconv.Atoi(args[0])
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "locations-show",
					"status":  "error",
					"error":   fmt.Sprintf("invalid id: %s", args[0]),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Invalid ID '%s': must be an integer\n"), args[0])
			}
			return
		}

		// Load location
		location, err := loadLocation(id)
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "locations-show",
					"status":  "error",
					"error":   fmt.Sprintf("location not found: %d", id),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Location with ID %d not found\n"), id)
			}
			return
		}

		if JSONOutput != nil && *JSONOutput {
			output := map[string]interface{}{
				"command":   "locations-show",
				"status":    "success",
				"id":        location.ID,
				"latitude":  location.Latitude,
				"longitude": location.Longitude,
				"label":     location.Label,
				"created_at":  location.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
				"last_used_at": location.LastUsedAt.Format("2006-01-02T15:04:05Z07:00"),
			}
			jsonBytes, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(jsonBytes))
		} else {
			format.PrintSection(fmt.Sprintf("Location %d", id))
			fmt.Println()
			format.PrintKV("Label", location.Label)
			format.PrintKV("Latitude", fmt.Sprintf("%.6f", location.Latitude))
			format.PrintKV("Longitude", fmt.Sprintf("%.6f", location.Longitude))
			format.PrintKV("Coordinates", fmt.Sprintf("%.6f, %.6f", location.Latitude, location.Longitude))
			format.PrintKV("Created", location.CreatedAt.Format("2006-01-02 15:04:05"))
			format.PrintKV("Last used", formatRelativeTime(location.LastUsedAt))
			fmt.Println()
		}
	},
}

func init() {
	LocationsCmd.AddCommand(showCmd)
}
