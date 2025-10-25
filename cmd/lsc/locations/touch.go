package locations

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var touchCmd = &cobra.Command{
	Use:   "touch <id>",
	Short: "Update last-used timestamp",
	Long:  `Update the last-used timestamp for a location (affects sort order).`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Parse ID
		id, err := strconv.Atoi(args[0])
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "locations-touch",
					"status":  "error",
					"error":   fmt.Sprintf("invalid id: %s", args[0]),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Invalid ID '%s': must be an integer\n"), args[0])
			}
			return
		}

		// Load existing location
		location, err := loadLocation(id)
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "locations-touch",
					"status":  "error",
					"error":   fmt.Sprintf("location not found: %d", id),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Location with ID %d not found\n"), id)
			}
			return
		}

		// Update last-used timestamp
		location.LastUsedAt = time.Now()

		// Save to Redis
		if err := saveLocation(*location); err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "locations-touch",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to update location: %v\n"), err)
			}
			return
		}

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command":      "locations-touch",
				"status":       "success",
				"id":           id,
				"last_used_at": location.LastUsedAt.Format("2006-01-02T15:04:05Z07:00"),
			})
			fmt.Println(string(output))
		} else {
			fmt.Printf("%s Updated last-used timestamp for location %s (%s)\n",
				format.Success("✓"),
				format.Info(fmt.Sprintf("%d", id)),
				location.Label,
			)
		}
	},
}

func init() {
	LocationsCmd.AddCommand(touchCmd)
}
