package locations

import (
	"encoding/json"
	"fmt"
	"os"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all saved locations",
	Long:  `Display all saved locations.`,
	Run: func(cmd *cobra.Command, args []string) {
		locations, err := loadAllLocations()
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "locations-list",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to load locations: %v\n"), err)
			}
			return
		}

		if JSONOutput != nil && *JSONOutput {
			type LocationJSON struct {
				ID         int     `json:"id"`
				Latitude   float64 `json:"latitude"`
				Longitude  float64 `json:"longitude"`
				Label      string  `json:"label"`
				CreatedAt  string  `json:"created_at"`
				LastUsedAt string  `json:"last_used_at"`
			}

			jsonLocations := make([]LocationJSON, len(locations))
			for i, loc := range locations {
				jsonLocations[i] = LocationJSON{
					ID:         loc.ID,
					Latitude:   loc.Latitude,
					Longitude:  loc.Longitude,
					Label:      loc.Label,
					CreatedAt:  loc.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
					LastUsedAt: loc.LastUsedAt.Format("2006-01-02T15:04:05Z07:00"),
				}
			}

			output := map[string]interface{}{
				"command":   "locations-list",
				"status":    "success",
				"count":     len(locations),
				"locations": jsonLocations,
			}
			jsonBytes, _ := json.MarshalIndent(output, "", "  ")
			fmt.Println(string(jsonBytes))
			return
		}

		// Human-readable output
		if len(locations) == 0 {
			fmt.Println(format.Dim("No saved locations"))
			return
		}

		format.PrintSection("Saved Locations")
		fmt.Println()

		for _, loc := range locations {
			fmt.Printf("[%s] %s %s\n",
				format.Info(fmt.Sprintf("%d", loc.ID)),
				format.Success(loc.Label),
				format.Dim(fmt.Sprintf("(%.6f, %.6f)", loc.Latitude, loc.Longitude)),
			)
			fmt.Printf("    Last used: %s\n", formatRelativeTime(loc.LastUsedAt))
			if !loc.CreatedAt.IsZero() {
				fmt.Printf("    Created: %s\n", loc.CreatedAt.Format("2006-01-02"))
			}
			fmt.Println()
		}
	},
}

func init() {
	LocationsCmd.AddCommand(listCmd)
}
