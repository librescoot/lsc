package ota

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
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
	// checkDefaultTimeout is the wait for a board that consumes check-now as
	// soon as the command lands, and for a DBC nothing is going to power on.
	checkDefaultTimeout = 15 * time.Second
	// checkDBCOrchestratedTimeout covers a DBC that the MDB will power on for
	// the check: the dashboard has to boot before its update-service starts
	// listening for the queued command (systemd reaches update-service about
	// ten seconds into boot) and then still do the GitHub lookup.
	checkDBCOrchestratedTimeout = 30 * time.Second
)

// dbcOrchestrationEnabled reads updates.mdb.orchestrate-dbc. update-service
// defaults this to on when the setting has not been seeded yet, so an absent
// or unreadable value is treated as on to match it.
func dbcOrchestrationEnabled() bool {
	val, err := RedisClient.HGet("settings", "updates.mdb.orchestrate-dbc")
	if err != nil || val == "" {
		return true
	}
	return val == "true"
}

// dbcCheckWillBeOrchestrated reports whether this request is one the MDB will
// chase the DBC for. Orchestration runs inside the MDB's own update check, so
// only a request that checks the MDB too gets the DBC powered on; a DBC-only
// request must not claim the longer budget for a boot nothing will trigger.
func dbcCheckWillBeOrchestrated(targets []string, orchestrationEnabled bool) bool {
	return orchestrationEnabled && slices.Contains(targets, "mdb")
}

// checkQueueTargets returns the services that should receive the initial
// check-now. An MDB-led DBC request is deliberately not sent directly to the
// DBC: the MDB preflight decides whether to wake it and queues its command.
func checkQueueTargets(targets []string, orchestrateDBC bool) []string {
	if orchestrateDBC && slices.Contains(targets, "mdb") && slices.Contains(targets, "dbc") {
		return []string{"mdb"}
	}
	return targets
}

// effectiveCheckTimeout resolves the wait budget for a set of targets. The
// waits run in parallel, so a single budget covering the slowest board is
// enough. --timeout overrides the per-board defaults when set.
func effectiveCheckTimeout(targets []string, orchestrateDBC bool) time.Duration {
	if checkTimeout > 0 {
		return checkTimeout
	}
	if !slices.Contains(targets, "dbc") {
		return checkDefaultTimeout
	}
	if orchestrateDBC {
		return checkDBCOrchestratedTimeout
	}
	return checkDefaultTimeout
}

// waitNotice describes the wait, calling out what has to happen to the DBC so a
// long wait does not look like a hang.
func waitNotice(targets []string, timeout time.Duration, orchestrateDBC bool) string {
	text := fmt.Sprintf("Waiting up to %s for the check result", timeout)
	if slices.Contains(targets, "dbc") {
		if orchestrateDBC {
			text += " (orchestration powers the DBC on to run it)"
		} else {
			text += " (the DBC must already be on)"
		}
	}
	return text + "..."
}

// checkOutcome is what waiting on one component's check produced.
type checkOutcome struct {
	component string
	kind      string // "update", "error", "no-update", "pending"
	status    componentStatus
	// orchestrated is true when this request will have the MDB power the DBC
	// on for the check, which is what makes a slow DBC response expected
	// rather than a sign that nothing is going to run it.
	orchestrated bool
	// preflight identifies an MDB-side DBC assessment. It is early and useful,
	// but advisory: the DBC confirms it after boot before installing anything.
	preflight bool
}

func (o checkOutcome) print() {
	label := colorizeComponent(o.component)
	switch o.kind {
	case "update":
		if o.preflight {
			text := format.Info("update available (MDB preflight)")
			if o.status.UpdateVersion != "" {
				text += " target " + o.status.UpdateVersion
			}
			fmt.Printf("%s: %s\n", label, text)
			return
		}
		fmt.Printf("%s: %s\n", label, o.status.summary())
	case "error":
		fmt.Printf("%s: %s\n", label, o.status.summary())
	case "no-update":
		text := format.Dim("no update available")
		if o.preflight {
			text += format.Dim(" (MDB preflight)")
		}
		if o.status.RunningVersion != "" {
			text += " (running " + o.status.RunningVersion + ")"
		}
		fmt.Printf("%s: %s\n", label, text)
	default:
		fmt.Printf("%s: %s\n", label, o.pendingMessage())
	}
}

// pendingMessage explains a wait that expired without any response. The queued
// check-now survives in Redis until the DBC's update-service pops it, so a DBC
// that nothing powered on is deferred rather than lost.
func (o checkOutcome) pendingMessage() string {
	if o.component == "dbc" && !o.orchestrated {
		return format.Warning("no response — the DBC will run the queued check the next time it is on") +
			format.Dim(" ('lsc dbc on-wait' to power it on now)")
	}
	return format.Warning("no response yet — run 'lsc ota watch' to follow the check")
}

func (o checkOutcome) json() map[string]any {
	m := map[string]any{"component": o.component}
	if o.preflight {
		m["source"] = "mdb-preflight"
	}
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

// freshDBCPreflightOutcome converts a preflight from this invocation into a
// user-facing answer. Unknown stays non-terminal so the DBC can settle it.
func freshDBCPreflightOutcome(s componentStatus, preflightBefore string, orchestrated bool) (checkOutcome, bool) {
	if !orchestrated || s.PreflightTime == "" || s.PreflightTime == preflightBefore {
		return checkOutcome{}, false
	}
	s.UpdateVersion = s.PreflightVersion
	switch s.PreflightResult {
	case "available":
		return checkOutcome{component: "dbc", kind: "update", status: s, orchestrated: true, preflight: true}, true
	case "up-to-date", "no-release":
		return checkOutcome{component: "dbc", kind: "no-update", status: s, orchestrated: true, preflight: true}, true
	default:
		return checkOutcome{}, false
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
func waitForCheckResult(component, lastCheckBefore, preflightBefore string, timeout time.Duration, orchestrated bool) checkOutcome {
	settingsKey := fmt.Sprintf("updates.%s.last-check-time", component)
	deadline := time.Now().Add(timeout)
	var checkSeenAt time.Time

	for {
		s := snapshotComponent(component)
		if kind, ok := planKind(s.Status); ok {
			return checkOutcome{component: component, kind: kind, status: s, orchestrated: orchestrated}
		}
		// An orchestrated DBC request is answered first by the MDB's preflight.
		// Only accept a result newer than the command, otherwise a periodic
		// check from hours ago could be presented as this command's answer.
		if component == "dbc" {
			if outcome, ok := freshDBCPreflightOutcome(s, preflightBefore, orchestrated); ok {
				return outcome
			}
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
			return checkOutcome{component: component, kind: "no-update", status: s, orchestrated: orchestrated}
		}
		if now.After(deadline) {
			if !checkSeenAt.IsZero() {
				return checkOutcome{component: component, kind: "no-update", status: s, orchestrated: orchestrated}
			}
			return checkOutcome{component: component, kind: "pending", status: s, orchestrated: orchestrated}
		}
		time.Sleep(checkPollInterval)
	}
}

// waitForCheckResults waits for all components in parallel so the total wait
// is bounded by timeout rather than the number of boards.
func waitForCheckResults(components []string, lastCheckBefore, preflightBefore map[string]string, timeout time.Duration, orchestrated bool) map[string]checkOutcome {
	results := make(map[string]checkOutcome, len(components))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, component := range components {
		wg.Add(1)
		go func(component string) {
			defer wg.Done()
			outcome := waitForCheckResult(component, lastCheckBefore[component], preflightBefore[component], timeout, orchestrated)
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
The wait is 15s for the MDB. A DBC check gets 30s only when the MDB is
checked too and orchestration is on, because the MDB then powers the
dashboard on for the check; a DBC-only check gets 15s, since nothing in
that request will power the DBC on. Use --timeout to override, or --no-wait
to return as soon as the command is queued.`,
	Example: `  lsc ota check             # Check both MDB and DBC
  lsc ota check mdb         # Check MDB only
  lsc ota check dbc         # Check DBC only (15s; the DBC must already be on)
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

		// When checking both boards, the MDB owns the DBC check: it publishes a
		// fast preflight result, wakes the DBC only when useful, then asks the
		// DBC service to make the final decision. Do not also queue the DBC here,
		// or a powered-off dashboard would consume a stale direct request later.
		orchestrateDBC := dbcCheckWillBeOrchestrated(targets, dbcOrchestrationEnabled())
		queueTargets := checkQueueTargets(targets, orchestrateDBC)
		orchestratedDBCTarget := len(queueTargets) != len(targets)
		if orchestratedDBCTarget {
			successMsg = "Update check triggered for MDB; DBC will be preflighted and awakened if needed"
		}

		// Remember each board's last check time before triggering, so the wait
		// can tell the update service actually consumed the command. The MDB's
		// preflight timestamp similarly distinguishes this request from stale
		// availability information left by an earlier periodic check.
		lastCheckBefore := make(map[string]string, len(targets))
		preflightBefore := make(map[string]string, len(targets))
		for _, target := range targets {
			lastCheckBefore[target], _ = RedisClient.HGet("settings", fmt.Sprintf("updates.%s.last-check-time", target))
			if target == "dbc" && orchestratedDBCTarget {
				preflightBefore[target], _ = RedisClient.HGet("ota", "preflight-time:dbc")
			}
		}

		// Send check-now to the components that own this request. The MDB queues
		// the DBC command itself after a positive or inconclusive preflight.
		for _, target := range queueTargets {
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

		timeout := time.Duration(0)
		wait := !checkNoWait
		if wait {
			timeout = effectiveCheckTimeout(targets, orchestrateDBC)
			wait = timeout > 0
		}

		// Announce the trigger before blocking on the result. In JSON mode the
		// progress goes to stderr so stdout stays a single parseable object.
		if JSONOutput != nil && *JSONOutput {
			fmt.Fprintln(os.Stderr, successMsg)
			if wait {
				fmt.Fprintln(os.Stderr, waitNotice(targets, timeout, orchestrateDBC))
			}
		} else {
			fmt.Println(format.Success(successMsg))
			if wait {
				fmt.Println(format.Info(waitNotice(targets, timeout, orchestrateDBC)))
				fmt.Println()
			}
		}

		var outcomes map[string]checkOutcome
		if wait {
			outcomes = waitForCheckResults(targets, lastCheckBefore, preflightBefore, timeout, orchestrateDBC)
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
			if wait {
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
	checkCmd.Flags().DurationVar(&checkTimeout, "timeout", 0, "How long to wait for the update service to report a plan (0 picks a per-board default: 15s, or 30s when the MDB is checked too and DBC orchestration is on)")
	checkCmd.Flags().BoolVar(&checkNoWait, "no-wait", false, "Don't wait for the check result")
	OTACmd.AddCommand(checkCmd)
}
