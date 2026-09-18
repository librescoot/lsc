package ota

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"librescoot/lsc/internal/cli"
	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

type componentStatus struct {
	Status                string
	StateOrigin           string
	RunningVersion        string
	UpdateVersion         string
	UpdateMethod          string
	DownloadProgress      string
	DownloadBytes         string
	DownloadTotal         string
	InstallProgress       string
	Error                 string
	ErrorMessage          string
	VehicleState          string
	VehicleStateTimestamp string
}

func readComponentStatus(otaData map[string]string, component string) componentStatus {
	get := func(field string) string {
		return otaData[fmt.Sprintf("%s:%s", field, component)]
	}
	return componentStatus{
		Status:           get("status"),
		StateOrigin:      get("state-origin"),
		UpdateVersion:    get("update-version"),
		UpdateMethod:     get("update-method"),
		DownloadProgress: get("download-progress"),
		DownloadBytes:    get("download-bytes"),
		DownloadTotal:    get("download-total"),
		InstallProgress:  get("install-progress"),
		Error:            get("error"),
		ErrorMessage:     get("error-message"),
	}
}

func (s componentStatus) equals(other componentStatus) bool {
	s.VehicleState = ""
	s.VehicleStateTimestamp = ""
	other.VehicleState = ""
	other.VehicleStateTimestamp = ""
	return s == other
}

func (s componentStatus) withOrigin(text string) string {
	if s.StateOrigin == "cached" {
		return text + " " + format.Dim("(cached)")
	}
	return text
}

func (s componentStatus) summary() string {
	switch s.Status {
	case "idle", "":
		text := format.Dim("idle")
		if s.RunningVersion != "" {
			text += " running " + s.RunningVersion
		}
		return s.withOrigin(text)
	case "downloading":
		text := colorizeOTAStatus("downloading")
		if s.UpdateVersion != "" {
			text += " target " + s.UpdateVersion
		}
		if s.DownloadProgress != "" {
			text += " " + formatProgress(s.DownloadProgress, s.DownloadBytes, s.DownloadTotal)
		}
		if s.UpdateMethod != "" {
			text += fmt.Sprintf(" [%s]", s.UpdateMethod)
		}
		return s.withOrigin(text)
	case "preparing":
		text := colorizeOTAStatus("preparing")
		if s.UpdateVersion != "" {
			text += " target " + s.UpdateVersion
		}
		if s.InstallProgress != "" {
			text += fmt.Sprintf(" %s%%", s.InstallProgress)
		}
		return s.withOrigin(text)
	case "installing":
		text := colorizeOTAStatus("installing")
		if s.UpdateVersion != "" {
			text += " target " + s.UpdateVersion
		}
		if s.InstallProgress != "" {
			text += fmt.Sprintf(" %s%%", s.InstallProgress)
		}
		return s.withOrigin(text)
	case "pending-reboot":
		text := colorizeOTAStatus("pending-reboot")
		if s.UpdateVersion != "" {
			text += " pending " + s.UpdateVersion
		}
		if info := standbyTimerSummary(s.VehicleState, s.VehicleStateTimestamp); info != "" {
			text += fmt.Sprintf(" (%s)", info)
		}
		return s.withOrigin(text)
	case "error":
		text := colorizeOTAStatus("error")
		if s.Error != "" {
			text += ": " + s.Error
		}
		if s.ErrorMessage != "" {
			text += " (" + s.ErrorMessage + ")"
		}
		return s.withOrigin(text)
	default:
		return s.withOrigin(s.Status)
	}
}

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch OTA update progress",
	Long: `Monitor OTA update status in real-time, printing changes as they occur.

Polls the OTA status every second and displays changes for both MDB and DBC.
Useful for watching download progress or installation status.

Press Ctrl+C to stop.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		components := []string{"mdb", "dbc"}
		prev := make(map[string]componentStatus)

		if JSONOutput == nil || !*JSONOutput {
			fmt.Println(format.Info("Watching OTA status (Ctrl+C to stop)"))
			fmt.Println()
		}

		readStatuses := func() map[string]componentStatus {
			result := make(map[string]componentStatus)
			otaData, err := RedisClient.HGetAll("ota")
			if err != nil {
				return result
			}
			for _, c := range components {
				s := readComponentStatus(otaData, c)
				if running, err := RedisClient.HGet(fmt.Sprintf("version:%s", c), "version_id"); err == nil {
					s.RunningVersion = running
				} else if previous, ok := prev[c]; ok {
					// Do not report a spurious version disappearance on a transient
					// read failure; the next successful poll will still detect a change.
					s.RunningVersion = previous.RunningVersion
				}
				result[c] = s
			}
			// Enrich MDB with vehicle state when pending-reboot
			if mdb, ok := result["mdb"]; ok && mdb.Status == "pending-reboot" {
				if vehicleData, err := RedisClient.HGetAll("vehicle"); err == nil {
					mdb.VehicleState = vehicleData["state"]
					mdb.VehicleStateTimestamp = vehicleData["state:timestamp"]
					result["mdb"] = mdb
				}
			}
			return result
		}

		// Print initial status
		initial := readStatuses()
		if len(initial) == 0 {
			fmt.Fprint(os.Stderr, format.Error("Failed to read OTA status\n"))
			return cli.ErrSilent
		}
		for _, c := range components {
			s := initial[c]
			prev[c] = s
			printWatchLine(c, s, nil)
		}

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-sigChan:
				fmt.Println()
				return nil
			case <-ticker.C:
				statuses := readStatuses()
				for _, c := range components {
					s := statuses[c]
					changed := !s.equals(prev[c])
					inPendingReboot := s.Status == "pending-reboot"
					if changed || inPendingReboot {
						completion := watchCompletionFor(prev[c], s)
						prev[c] = s
						printWatchLine(c, s, completion)
					}
				}
			}
		}
	},
}

// watchCompletion describes a pending-reboot that just resolved.
type watchCompletion struct {
	targetVersion  string
	runningVersion string
}

func (c watchCompletion) version() string {
	if c.targetVersion != "" {
		return c.targetVersion
	}
	return c.runningVersion
}

func (c watchCompletion) summary() string {
	switch {
	case c.runningVersion != "" && (c.targetVersion == "" || c.runningVersion == c.targetVersion):
		return "update complete — running " + c.runningVersion
	case c.targetVersion != "":
		return "update complete — target " + c.targetVersion
	default:
		return "update complete — rebooted"
	}
}

// watchCompletionFor reports a pending-reboot that just resolved. A board
// coming back idle after waiting to reboot is the success case that a bare
// "pending-reboot -> idle" change does not convey on its own.
func watchCompletionFor(prev, now componentStatus) *watchCompletion {
	if prev.Status != "pending-reboot" {
		return nil
	}
	if now.Status != "idle" && now.Status != "" {
		return nil
	}
	return &watchCompletion{
		targetVersion:  prev.UpdateVersion,
		runningVersion: now.RunningVersion,
	}
}

func printWatchLine(component string, s componentStatus, completion *watchCompletion) {
	if JSONOutput != nil && *JSONOutput {
		output := map[string]any{
			"timestamp": time.Now().Unix(),
			"component": component,
			"status":    s.Status,
		}
		if completion != nil {
			output["event"] = "update-complete"
			if v := completion.version(); v != "" {
				output["completed-version"] = v
			}
			// The running version comes from the version hash, which can lag the
			// reboot behind the status; do not report a stale one next to the
			// completion. A matching version is kept.
			if completion.runningVersion != completion.targetVersion {
				delete(output, "running-version")
			}
		}
		if s.StateOrigin != "" {
			output["state-origin"] = s.StateOrigin
		}
		if s.RunningVersion != "" {
			output["running-version"] = s.RunningVersion
		}
		if s.UpdateVersion != "" {
			output["update-version"] = s.UpdateVersion
		}
		if s.UpdateMethod != "" {
			output["update-method"] = s.UpdateMethod
		}
		if s.DownloadProgress != "" {
			output["download-progress"] = s.DownloadProgress
		}
		if s.DownloadBytes != "" {
			output["download-bytes"] = s.DownloadBytes
		}
		if s.DownloadTotal != "" {
			output["download-total"] = s.DownloadTotal
		}
		if s.InstallProgress != "" {
			output["install-progress"] = s.InstallProgress
		}
		if s.Error != "" {
			output["error"] = s.Error
		}
		if s.ErrorMessage != "" {
			output["error-message"] = s.ErrorMessage
		}
		if s.VehicleState != "" {
			output["vehicle-state"] = s.VehicleState
			output["vehicle-state-timestamp"] = s.VehicleStateTimestamp
		}
		jsonBytes, _ := json.Marshal(output)
		fmt.Println(string(jsonBytes))
	} else {
		timestamp := format.Dim(time.Now().Format("15:04:05"))
		if completion != nil {
			fmt.Printf("[%s] %s: %s\n",
				timestamp,
				colorizeComponent(component),
				format.Success("✓ "+completion.summary()))
			return
		}
		fmt.Printf("[%s] %s: %s\n",
			timestamp,
			colorizeComponent(component),
			s.summary())
	}
}

func init() {
	OTACmd.AddCommand(watchCmd)
}
