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

var editCmd = &cobra.Command{
	Use:   "edit <id> <field> <value> [<field> <value> ...]",
	Short: "Edit a saved location",
	Long: `Edit one or more fields of a saved location.

Valid fields: label, lat, lon

Examples:
  lsc loc edit 0 label "New Home"
  lsc loc edit 0 lat 52.5 lon 13.4
  lsc loc edit 0 label "Office" lat 52.5235 lon 13.4115`,
	Args: cobra.MinimumNArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		// Parse ID
		id, err := strconv.Atoi(args[0])
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "locations-edit",
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
					"command": "locations-edit",
					"status":  "error",
					"error":   fmt.Sprintf("location not found: %d", id),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Location with ID %d not found\n"), id)
			}
			return
		}

		// Parse field-value pairs
		updates, err := parseFieldValuePairs(args[1:])
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "locations-edit",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("%v\n"), err)
			}
			return
		}

		// Apply updates
		modified := false
		for field, value := range updates {
			switch field {
			case "label":
				location.Label = value
				modified = true
			case "latitude":
				lat, err := strconv.ParseFloat(value, 64)
				if err != nil {
					if JSONOutput != nil && *JSONOutput {
						output, _ := json.Marshal(map[string]interface{}{
							"command": "locations-edit",
							"status":  "error",
							"error":   fmt.Sprintf("invalid latitude: %s", value),
						})
						fmt.Println(string(output))
					} else {
						fmt.Fprintf(os.Stderr, format.Error("Invalid latitude '%s': must be a number\n"), value)
					}
					return
				}
				location.Latitude = lat
				modified = true
			case "longitude":
				lon, err := strconv.ParseFloat(value, 64)
				if err != nil {
					if JSONOutput != nil && *JSONOutput {
						output, _ := json.Marshal(map[string]interface{}{
							"command": "locations-edit",
							"status":  "error",
							"error":   fmt.Sprintf("invalid longitude: %s", value),
						})
						fmt.Println(string(output))
					} else {
						fmt.Fprintf(os.Stderr, format.Error("Invalid longitude '%s': must be a number\n"), value)
					}
					return
				}
				location.Longitude = lon
				modified = true
			}
		}

		if !modified {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "locations-edit",
					"status":  "error",
					"error":   "no valid fields to update",
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("No valid fields to update\n"))
			}
			return
		}

		// Validate coordinates if changed
		if err := validateCoordinates(location.Latitude, location.Longitude); err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "locations-edit",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("%v\n"), err)
			}
			return
		}

		// Update last-used-at timestamp
		location.LastUsedAt = time.Now()

		// Save to Redis
		if err := saveLocation(*location); err != nil {
			if JSONOutput != nil && *JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "locations-edit",
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
				"command":   "locations-edit",
				"status":    "success",
				"id":        id,
				"latitude":  location.Latitude,
				"longitude": location.Longitude,
				"label":     location.Label,
			})
			fmt.Println(string(output))
		} else {
			fmt.Printf("%s Location %s updated\n",
				format.Success("✓"),
				format.Info(fmt.Sprintf("%d", id)),
			)
		}
	},
}

func init() {
	LocationsCmd.AddCommand(editCmd)
}
