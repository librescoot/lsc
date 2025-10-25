package locations

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add <latitude> <longitude> <label>",
	Short: "Add a new saved location",
	Long:  `Add a new saved location with coordinates and label.`,
	Args:  cobra.MinimumNArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		// Parse latitude
		lat, err := strconv.ParseFloat(args[0], 64)
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "locations-add",
					"status":  "error",
					"error":   fmt.Sprintf("invalid latitude: %s", args[0]),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Invalid latitude '%s': must be a number\n"), args[0])
			}
			return
		}

		// Parse longitude
		lon, err := strconv.ParseFloat(args[1], 64)
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "locations-add",
					"status":  "error",
					"error":   fmt.Sprintf("invalid longitude: %s", args[1]),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Invalid longitude '%s': must be a number\n"), args[1])
			}
			return
		}

		// Validate coordinates
		if err := validateCoordinates(lat, lon); err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "locations-add",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("%v\n"), err)
			}
			return
		}

		// Join remaining args as label
		label := strings.Join(args[2:], " ")

		// Find next available ID
		id, err := findNextAvailableID()
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "locations-add",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to find available ID: %v\n"), err)
			}
			return
		}

		// Create location
		now := time.Now()
		location := SavedLocation{
			ID:         id,
			Latitude:   lat,
			Longitude:  lon,
			Label:      label,
			CreatedAt:  now,
			LastUsedAt: now,
		}

		// Save to Redis
		if err := saveLocation(location); err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "locations-add",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to save location: %v\n"), err)
			}
			return
		}

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command":   "locations-add",
				"status":    "success",
				"id":        id,
				"latitude":  lat,
				"longitude": lon,
				"label":     label,
			})
			fmt.Println(string(output))
		} else {
			fmt.Printf("%s Location '%s' saved with ID %s\n",
				format.Success("✓"),
				label,
				format.Info(fmt.Sprintf("%d", id)),
			)
		}
	},
}

func init() {
	LocationsCmd.AddCommand(addCmd)
}
