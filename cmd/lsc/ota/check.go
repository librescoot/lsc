package ota

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var (
	checkTimeout time.Duration
	checkNoWait  bool
)

const (
	// checkPollInterval is how often the wait re-reads the OTA hash.
	checkPollInterval = 250 * time.Millisecond
	// checkGrace is how long to keep watching after the update service records
	// a fresh last-check-time. That timestamp is written before the GitHub
	// lookup, so a plan can still appear once the lookup returns; without the
	// grace a fast no-op check and a slow one look identical.
	checkGrace = 6 * time.Second
)

// checkOutcome is what waiting on one component's check produced.
type checkOutcome struct {
	component string
	kind      string // "update", "error", "no-update", "pending"
	status    componentStatus
}

func (o checkOutcome) print() {
	label := colorizeComponent(o.component)
	switch o.kind {
	case "update", "error":
		fmt.Printf("%s: %s\n", label, o.status.summary())
	case "no-update":
		text := format.Dim("no update available")
		if o.status.RunningVersion != "" {
			text += " (running " + o.status.RunningVersion + ")"
		}
		fmt.Printf("%s: %s\n", label, text)
	default:
		fmt.Printf("%s: %s\n", label, format.Warning("no response yet — run 'lsc ota watch' to follow the check"))
	}
}

func (o checkOutcome) json() map[string]any {
	m := map[string]any{"component": o.component}
	if o.status.RunningVersion != "" {
		m["running-version"] = o.status.RunningVersion
	}
	if o.status.Status != "" {
		m["status"] = o.status.Status
	}
	switch o.kind {
	case "update":
		m["outcome"] = "update-available"
		if o.status.UpdateVersion != "" {
			m["update-version"] = o.status.UpdateVersion
		}
		if o.status.UpdateMethod != "" {
			m["update-method"] = o.status.UpdateMethod
		}
	case "error":
		m["outcome"] = "error"
		if o.status.Error != "" {
			m["error"] = o.status.Error
		}
		if o.status.ErrorMessage != "" {
			m["error-message"] = o.status.ErrorMessage
		}
	case "no-update":
		m["outcome"] = "no-update"
	default:
		m["outcome"] = "pending"
	}
	return m
}

// planKind classifies an OTA status as a plan the update service has
// committed to, so the wait can stop as soon as one appears.
func planKind(status string) (string, bool) {
	switch status {
	case "downloading", "preparing", "installing", "pending-reboot":
		return "update", true
	case "error":
		return "error", true
	default:
		return "", false
	}
}

func snapshotComponent(component string) componentStatus {
	otaData, err := RedisClient.HGetAll("ota")
	if err != nil {
		return componentStatus{}
	}
	s := readComponentStatus(otaData, component)
	if running, err := RedisClient.HGet(fmt.Sprintf("version:%s", component), "version_id"); err == nil {
		s.RunningVersion = running
	}
	return s
}

// waitForCheckResult watches one component until the update service reports a
// plan, or until the check is known to have found nothing to install.
func waitForCheckResult(component, lastCheckBefore string, timeout time.Duration) checkOutcome {
	settingsKey := fmt.Sprintf("updates.%s.last-check-time", component)
	deadline := time.Now().Add(timeout)
	var checkSeenAt time.Time

	for {
		s := snapshotComponent(component)
		if kind, ok := planKind(s.Status); ok {
			return checkOutcome{component: component, kind: kind, status: s}
		}

		if checkSeenAt.IsZero() {
			if lastCheck, err := RedisClient.HGet("settings", settingsKey); err == nil && lastCheck != "" && lastCheck != lastCheckBefore {
				checkSeenAt = time.Now()
			}
		}

		now := time.Now()
		// A fresh last-check-time with no plan means the service consumed the
		// command and did not come back with an install.
		if !checkSeenAt.IsZero() && now.Sub(checkSeenAt) >= checkGrace {
			return checkOutcome{component: component, kind: "no-update", status: s}
		}
		if now.After(deadline) {
			if !checkSeenAt.IsZero() {
				return checkOutcome{component: component, kind: "no-update", status: s}
			}
			return checkOutcome{component: component, kind: "pending", status: s}
		}
		time.Sleep(checkPollInterval)
	}
}

// waitForCheckResults waits for all components in parallel so the total wait
// is bounded by timeout rather than the number of boards.
func waitForCheckResults(components []string, lastCheckBefore map[string]string, timeout time.Duration) map[string]checkOutcome {
	results := make(map[string]checkOutcome, len(components))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, component := range components {
		wg.Add(1)
		go func(component string) {
			defer wg.Done()
			outcome := waitForCheckResult(component, lastCheckBefore[component], timeout)
			mu.Lock()
			results[component] = outcome
			mu.Unlock()
		}(component)
	}
	wg.Wait()
	return results
}

var checkCmd = &cobra.Command{
	Use:   "check [mdb|dbc]",
	Short: "Trigger immediate update check",
	Long: `Trigger an immediate update check by sending a check-now command to the update service.

By default the command then waits briefly for the service to report what it
found, printing the planned update (or that there is nothing to install).
Use --no-wait to return as soon as the command is queued.

Examples:
  lsc ota check             # Check both MDB and DBC
  lsc ota check mdb         # Check MDB only
  lsc ota check dbc         # Check DBC only
  lsc ota check --no-wait   # Queue the check and return immediately`,
	Args:      cobra.MaximumNArgs(1),
	ValidArgs: []string{"mdb", "dbc"},
	RunE: func(cmd *cobra.Command, args []string) error {
		var targets []string
		var successMsg string

		// Determine which components to trigger
		if len(args) == 0 {
			targets = []string{"mdb", "dbc"}
			successMsg = "Update check triggered for MDB and DBC"
		} else {
			component := args[0]
			if component != "mdb" && component != "dbc" {
				if JSONOutput != nil && *JSONOutput {
					output, _ := json.Marshal(map[string]interface{}{
						"command": "ota-check",
						"status":  "error",
						"error":   fmt.Sprintf("Invalid component '%s'. Must be 'mdb' or 'dbc'", component),
					})
					fmt.Println(string(output))
				} else {
					fmt.Fprintf(os.Stderr, format.Error("Invalid component '%s'. Must be 'mdb' or 'dbc'\n"), component)
				}
				return fmt.Errorf("invalid component '%s'", component)
			}
			targets = []string{component}
			successMsg = fmt.Sprintf("Update check triggered for %s", strings.ToUpper(component))
		}

		// Remember each board's last check time before triggering, so the wait
		// can tell the update service actually consumed the command.
		lastCheckBefore := make(map[string]string, len(targets))
		for _, target := range targets {
			lastCheckBefore[target], _ = RedisClient.HGet("settings", fmt.Sprintf("updates.%s.last-check-time", target))
		}

		// Send check-now command to each target component
		for _, target := range targets {
			channel := fmt.Sprintf("scooter:update:%s", target)
			err := RedisClient.LPush(channel, "check-now")
			if err != nil {
				if JSONOutput != nil && *JSONOutput {
					output, _ := json.Marshal(map[string]interface{}{
						"command":   "ota-check",
						"component": target,
						"status":    "error",
						"error":     err.Error(),
					})
					fmt.Println(string(output))
				} else {
					fmt.Fprintf(os.Stderr, format.Error("Failed to trigger update check for %s: %v\n"), strings.ToUpper(target), err)
				}
				return err
			}
		}

		wait := !checkNoWait && checkTimeout > 0
		var outcomes map[string]checkOutcome
		if wait {
			outcomes = waitForCheckResults(targets, lastCheckBefore, checkTimeout)
		}

		if JSONOutput != nil && *JSONOutput {
			result := map[string]interface{}{
				"command":    "ota-check",
				"components": targets,
				"status":     "success",
				"message":    successMsg,
			}
			if wait {
				results := make([]map[string]any, 0, len(targets))
				for _, target := range targets {
					results = append(results, outcomes[target].json())
				}
				result["results"] = results
			}
			output, _ := json.Marshal(result)
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success(successMsg))
			if wait {
				fmt.Println(format.Info(fmt.Sprintf("Waiting up to %s for the check result...", checkTimeout)))
				fmt.Println()
				for _, target := range targets {
					outcomes[target].print()
				}
				fmt.Println()
			} else {
				fmt.Println(format.Info("The update service will check for available updates immediately"))
			}
			fmt.Println(format.Dim("Use 'lsc ota status' to see current status"))
			fmt.Println(format.Dim("Use 'lsc ota watch' to monitor progress"))
		}
		return nil
	},
}

func init() {
	checkCmd.Flags().DurationVar(&checkTimeout, "timeout", 15*time.Second, "How long to wait for the update service to report a plan")
	checkCmd.Flags().BoolVar(&checkNoWait, "no-wait", false, "Don't wait for the check result")
	OTACmd.AddCommand(checkCmd)
}
