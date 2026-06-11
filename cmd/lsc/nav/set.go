package nav

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"librescoot/lsc/cmd/lsc/locations"
	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var navSetCmd = &cobra.Command{
	Use:   "set <lat,lon | lat lon | saved-location>",
	Short: "Set the navigation destination",
	Long: `Set the navigation destination to coordinates or a saved location.

Examples:
  lsc nav set 52.520008,13.404954
  lsc nav set 52.520008 13.404954
  lsc nav set Home                  # label from 'lsc locations list'`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		lat, lon, address, err := resolveDestination(args)
		if err != nil {
			return emitNavError("nav-set", err)
		}

		destination := fmt.Sprintf("%.6f,%.6f", lat, lon)
		fields := map[string]string{"destination": destination}
		if address != "" {
			fields["address"] = address
		}
		if err := setNavFields(fields); err != nil {
			return emitNavError("nav-set", err)
		}

		if JSONOutput != nil && *JSONOutput {
			result := map[string]any{
				"command":     "nav-set",
				"status":      "success",
				"destination": destination,
				"latitude":    lat,
				"longitude":   lon,
			}
			if address != "" {
				result["address"] = address
			}
			output, _ := json.Marshal(result)
			fmt.Println(string(output))
		} else {
			target := destination
			if address != "" {
				target = fmt.Sprintf("%s (%s)", address, destination)
			}
			fmt.Println(format.Success("Navigation destination set: " + target))
		}
		return nil
	},
}

// resolveDestination turns command arguments into coordinates: either
// "lat,lon", "lat lon", or the label of a saved location.
func resolveDestination(args []string) (lat, lon float64, address string, err error) {
	if len(args) == 2 {
		latV, errLat := strconv.ParseFloat(args[0], 64)
		lonV, errLon := strconv.ParseFloat(strings.TrimSuffix(args[1], ","), 64)
		if errLat == nil && errLon == nil {
			return validateCoords(latV, lonV)
		}
	}

	if len(args) == 1 && strings.Contains(args[0], ",") {
		parts := strings.SplitN(args[0], ",", 2)
		latV, errLat := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		lonV, errLon := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if errLat == nil && errLon == nil {
			return validateCoords(latV, lonV)
		}
	}

	// Treat the arguments as a saved location label (labels may contain spaces)
	label := strings.Join(args, " ")
	loc, err := locations.FindByLabel(label)
	if err != nil {
		return 0, 0, "", fmt.Errorf("%v (expected coordinates or a saved location label)", err)
	}
	lat, lon, _, err = validateCoords(loc.Latitude, loc.Longitude)
	return lat, lon, loc.Label, err
}

func validateCoords(lat, lon float64) (float64, float64, string, error) {
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return 0, 0, "", fmt.Errorf("coordinates out of range: %f, %f", lat, lon)
	}
	return lat, lon, "", nil
}
