package nav

import (
	"encoding/json"
	"fmt"
	"strconv"

	"librescoot/lsc/internal/format"
	"librescoot/lsc/internal/routeplan"

	"github.com/spf13/cobra"
)

var navPlanName string

var navPlanCmd = &cobra.Command{
	Use:   "plan",
	Short: "Manage the multi-hop route plan",
	Long: `Inspect and edit the ordered stop list the dashboard guides through.

The plan is managed by settings-service and projected into the navigation hash.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return navPlanListCmd.RunE(cmd, args)
	},
}

var navPlanListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show the current route plan",
	RunE: func(cmd *cobra.Command, args []string) error {
		plan, err := routeplan.Call(RedisClient, "plan.get", routeplan.Empty{})
		if err != nil {
			return emitNavError("nav-plan-list", err)
		}
		stops, step := plan.Stops, plan.CurrentStep
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

		plan, err := routeplan.Call(RedisClient, "plan.append", routeplan.AppendRequest{
			Stop: routeplan.StopInput{Lat: lat, Lon: lon, Label: label},
		})
		if err != nil {
			return emitNavError("nav-plan-add", err)
		}
		stops, step := plan.Stops, plan.CurrentStep

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
		plan, err := routeplan.Call(RedisClient, "plan.get", routeplan.Empty{})
		if err != nil {
			return emitNavError("nav-plan-remove", err)
		}
		if index < 1 || index > len(plan.Stops) {
			return emitNavError("nav-plan-remove",
				fmt.Errorf("index %d out of range (plan has %d stops)", index, len(plan.Stops)))
		}
		plan, err = routeplan.Call(RedisClient, "plan.remove", routeplan.RemoveRequest{
			Index: index - 1, ExpectedRevision: plan.Revision,
		})
		if err != nil {
			return emitNavError("nav-plan-remove", err)
		}
		stops, step := plan.Stops, plan.CurrentStep

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
		plan, err := routeplan.Call(RedisClient, "plan.get", routeplan.Empty{})
		if err != nil {
			return emitNavError("nav-plan-skip", err)
		}
		if len(plan.Stops) == 0 {
			return emitNavError("nav-plan-skip", fmt.Errorf("no route plan set"))
		}
		if plan.CurrentStep+1 >= len(plan.Stops) {
			return emitNavError("nav-plan-skip", fmt.Errorf("already at the last stop"))
		}
		request := routeplan.ProgressRequest{ExpectedPlanID: plan.ID, ExpectedStopID: plan.Stops[plan.CurrentStep].ID}
		plan, err = routeplan.Call(RedisClient, "plan.advance", request)
		if err != nil {
			return emitNavError("nav-plan-skip", err)
		}
		step, stops := plan.CurrentStep, plan.Stops

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

func init() {
	navPlanAddCmd.Flags().StringVar(&navPlanName, "name", "", "label for the added stop")
	navPlanCmd.AddCommand(navPlanListCmd)
	navPlanCmd.AddCommand(navPlanAddCmd)
	navPlanCmd.AddCommand(navPlanRemoveCmd)
	navPlanCmd.AddCommand(navPlanSkipCmd)
	NavCmd.AddCommand(navPlanCmd)
}
