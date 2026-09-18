package nav

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

// planStop mirrors the JSON shape the dashboard reads from the navigation
// hash's "waypoints" field.
type planStop struct {
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
	Label string  `json:"label,omitempty"`
}

var navPlanName string

var navPlanCmd = &cobra.Command{
	Use:     "plan",
	Short:   "Manage the multi-hop route plan",
	Long: `Inspect and edit the ordered stop list the dashboard guides through.

The plan lives in the navigation hash's "waypoints" (JSON) together with
"current-step". Without a plan the destination fields describe a single trip.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return navPlanListCmd.RunE(cmd, args)
	},
}

var navPlanListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show the current route plan",
	RunE: func(cmd *cobra.Command, args []string) error {
		stops, step, err := readPlan()
		if err != nil {
			return emitNavError("nav-plan-list", err)
		}
		if JSONOutput != nil && *JSONOutput {
			output, _ := json.MarshalIndent(map[string]any{
				"command":      "nav-plan-list",
				"status":       "success",
				"stops":        stops,
				"current_step": step,
			}, "", "  ")
			fmt.Println(string(output))
			return nil
		}

		if len(stops) == 0 {
			fmt.Println(format.Dim("No route plan set."))
			return nil
		}
		format.PrintSection(fmt.Sprintf("Route plan (%d stops, at stop %d)", len(stops), step+1))
		for i, stop := range stops {
			marker := "  "
			if i == step {
				marker = "> "
			}
			label := stop.Label
			if label == "" {
				label = fmt.Sprintf("%.5f, %.5f", stop.Lat, stop.Lon)
			}
			format.PrintKV(fmt.Sprintf("%s%d", marker, i+1), label)
		}
		fmt.Println()
		return nil
	},
}

var navPlanAddCmd = &cobra.Command{
	Use:   "add <lat,lon | lat lon | saved-location>",
	Short: "Append a stop to the route plan",
	Long: `Append a stop to the route plan, creating the plan if none exists.

Examples:
  lsc nav plan add 52.520008,13.404954
  lsc nav plan add 52.520008 13.404954 --name "Alexanderplatz"
  lsc nav plan add Home`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		lat, lon, address, err := resolveDestination(args)
		if err != nil {
			return emitNavError("nav-plan-add", err)
		}
		label := address
		if navPlanName != "" {
			label = navPlanName
		}

		stops, step, err := readPlan()
		if err != nil {
			return emitNavError("nav-plan-add", err)
		}
		stops = append(stops, planStop{Lat: lat, Lon: lon, Label: label})
		if err := writePlan(stops, step); err != nil {
			return emitNavError("nav-plan-add", err)
		}

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]any{
				"command":      "nav-plan-add",
				"status":       "success",
				"stops":        len(stops),
				"current_step": step,
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success(fmt.Sprintf("Added stop %d of %d", len(stops), len(stops))))
		}
		return nil
	},
}

var navPlanRemoveCmd = &cobra.Command{
	Use:   "remove <index>",
	Short: "Remove a stop from the route plan",
	Long:  `Remove a stop by its 1-based position. Removing every stop clears the plan.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		index, err := strconv.Atoi(args[0])
		if err != nil {
			return emitNavError("nav-plan-remove", fmt.Errorf("index must be a number"))
		}
		stops, step, err := readPlan()
		if err != nil {
			return emitNavError("nav-plan-remove", err)
		}
		if index < 1 || index > len(stops) {
			return emitNavError("nav-plan-remove",
				fmt.Errorf("index %d out of range (plan has %d stops)", index, len(stops)))
		}
		zero := index - 1
		stops = append(stops[:zero], stops[zero+1:]...)
		if zero < step {
			step--
		}
		if err := writePlan(stops, step); err != nil {
			return emitNavError("nav-plan-remove", err)
		}

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]any{
				"command":      "nav-plan-remove",
				"status":       "success",
				"stops":        len(stops),
				"current_step": step,
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success(fmt.Sprintf("Removed stop %d; %d remain", index, len(stops))))
		}
		return nil
	},
}

var navPlanSkipCmd = &cobra.Command{
	Use:   "skip",
	Short: "Advance the plan to the next stop",
	RunE: func(cmd *cobra.Command, args []string) error {
		stops, step, err := readPlan()
		if err != nil {
			return emitNavError("nav-plan-skip", err)
		}
		if len(stops) == 0 {
			return emitNavError("nav-plan-skip", fmt.Errorf("no route plan set"))
		}
		if step+1 >= len(stops) {
			return emitNavError("nav-plan-skip", fmt.Errorf("already at the last stop"))
		}
		step++
		if err := writePlan(stops, step); err != nil {
			return emitNavError("nav-plan-skip", err)
		}

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]any{
				"command":      "nav-plan-skip",
				"status":       "success",
				"current_step": step,
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success(fmt.Sprintf("Now guiding to stop %d of %d", step+1, len(stops))))
		}
		return nil
	},
}

// readPlan reads the waypoints JSON and current step from the navigation hash.
func readPlan() ([]planStop, int, error) {
	data, err := RedisClient.HGetAll("navigation")
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read navigation hash: %w", err)
	}
	var stops []planStop
	if raw := data["waypoints"]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &stops); err != nil {
			return nil, 0, fmt.Errorf("navigation waypoints are not valid JSON: %w", err)
		}
	}
	step, _ := strconv.Atoi(data["current-step"])
	return stops, step, nil
}

// writePlan stores the stop list and current step, plus the target fields the
// dashboard reads for the current hop. An empty list clears the plan.
func writePlan(stops []planStop, step int) error {
	if len(stops) == 0 {
		fields := map[string]string{
			"destination":  "",
			"latitude":     "",
			"longitude":    "",
			"address":      "",
			"timestamp":    "",
			"waypoints":    "",
			"current-step": "",
		}
		return setNavFields(fields)
	}
	if step < 0 {
		step = 0
	}
	if step >= len(stops) {
		step = len(stops) - 1
	}

	encoded, err := json.Marshal(stops)
	if err != nil {
		return fmt.Errorf("failed to encode waypoints: %w", err)
	}
	target := stops[step]
	fields := map[string]string{
		"waypoints":    string(encoded),
		"current-step": strconv.Itoa(step),
		"latitude":     fmt.Sprintf("%.6f", target.Lat),
		"longitude":    fmt.Sprintf("%.6f", target.Lon),
		"destination":  fmt.Sprintf("%.6f,%.6f", target.Lat, target.Lon),
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
	}
	if target.Label != "" {
		fields["address"] = target.Label
	}
	return setNavFields(fields)
}

func init() {
	navPlanAddCmd.Flags().StringVar(&navPlanName, "name", "", "label for the added stop")
	navPlanCmd.AddCommand(navPlanListCmd)
	navPlanCmd.AddCommand(navPlanAddCmd)
	navPlanCmd.AddCommand(navPlanRemoveCmd)
	navPlanCmd.AddCommand(navPlanSkipCmd)
	NavCmd.AddCommand(navPlanCmd)
}