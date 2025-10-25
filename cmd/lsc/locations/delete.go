package locations

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:     "delete <id>",
	Aliases: []string{"rm", "remove"},
	Short:   "Delete a saved location",
	Long:    `Delete a saved location by ID.`,
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Parse ID
		id, err := strconv.Atoi(args[0])
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "locations-delete",
					"status":  "error",
					"error":   fmt.Sprintf("invalid id: %s", args[0]),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Invalid ID '%s': must be an integer\n"), args[0])
			}
			return
		}

		// Check if location exists
		location, err := loadLocation(id)
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "locations-delete",
					"status":  "error",
					"error":   fmt.Sprintf("location not found: %d", id),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Location with ID %d not found\n"), id)
			}
			return
		}

		// Delete from Redis
		if err := deleteLocation(id); err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "locations-delete",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to delete location: %v\n"), err)
			}
			return
		}

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "locations-delete",
				"status":  "success",
				"id":      id,
			})
			fmt.Println(string(output))
		} else {
			fmt.Printf("%s Deleted location %s (%s)\n",
				format.Success("✓"),
				format.Info(fmt.Sprintf("%d", id)),
				location.Label,
			)
		}
	},
}

func init() {
	LocationsCmd.AddCommand(deleteCmd)
}
