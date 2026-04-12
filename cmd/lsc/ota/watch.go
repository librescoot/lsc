package ota

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

type componentStatus struct {
	Status               string
	UpdateVersion        string
	UpdateMethod         string
	DownloadProgress     string
	DownloadBytes        string
	DownloadTotal        string
	InstallProgress      string
	Error                string
	ErrorMessage         string
	VehicleState         string
	VehicleStateTimestamp string
}

func readComponentStatus(otaData map[string]string, component string) componentStatus {
	get := func(field string) string {
		return otaData[fmt.Sprintf("%s:%s", field, component)]
	}
	return componentStatus{
		Status:           get("status"),
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

func (s componentStatus) summary() string {
	switch s.Status {
	case "idle", "":
		return format.Dim("idle")
	case "downloading":
		text := colorizeOTAStatus("downloading")
		if s.UpdateVersion != "" {
			text += " " + s.UpdateVersion
		}
		if s.DownloadProgress != "" {
			text += " " + formatProgress(s.DownloadProgress, s.DownloadBytes, s.DownloadTotal)
		}
		if s.UpdateMethod != "" {
			text += fmt.Sprintf(" [%s]", s.UpdateMethod)
		}
		return text
	case "preparing":
		text := colorizeOTAStatus("preparing")
		if s.UpdateVersion != "" {
			text += " " + s.UpdateVersion
		}
		if s.InstallProgress != "" {
			text += fmt.Sprintf(" %s%%", s.InstallProgress)
		}
		return text
	case "installing":
		text := colorizeOTAStatus("installing")
		if s.UpdateVersion != "" {
			text += " " + s.UpdateVersion
		}
		if s.InstallProgress != "" {
			text += fmt.Sprintf(" %s%%", s.InstallProgress)
		}
		return text
	case "pending-reboot":
		text := colorizeOTAStatus("pending-reboot")
		if s.UpdateVersion != "" {
			text += " " + s.UpdateVersion
		}
		if info := standbyTimerSummary(s.VehicleState, s.VehicleStateTimestamp); info != "" {
			text += fmt.Sprintf(" (%s)", info)
		}
		return text
	case "error":
		text := colorizeOTAStatus("error")
		if s.Error != "" {
			text += ": " + s.Error
		}
		if s.ErrorMessage != "" {
			text += " (" + s.ErrorMessage + ")"
		}
		return text
	default:
		return s.Status
	}
}

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch OTA update progress",
	Long: `Monitor OTA update status in real-time, printing changes as they occur.

Polls the OTA status every second and displays changes for both MDB and DBC.
Useful for watching download progress or installation status.

Press Ctrl+C to stop.`,
	Run: func(cmd *cobra.Command, args []string) {
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
			fmt.Fprintf(os.Stderr, format.Error("Failed to read OTA status\n"))
			return
		}
		for _, c := range components {
			s := initial[c]
			prev[c] = s
			printWatchLine(c, s)
		}

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-sigChan:
				fmt.Println()
				return
			case <-ticker.C:
				statuses := readStatuses()
				for _, c := range components {
					s := statuses[c]
					changed := !s.equals(prev[c])
					inPendingReboot := s.Status == "pending-reboot"
					if changed || inPendingReboot {
						prev[c] = s
						printWatchLine(c, s)
					}
				}
			}
		}
	},
}

func printWatchLine(component string, s componentStatus) {
	if JSONOutput != nil && *JSONOutput {
		output := map[string]any{
			"timestamp": time.Now().Unix(),
			"component": component,
			"status":    s.Status,
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
		timestamp := time.Now().Format("15:04:05")
		fmt.Printf("[%s] %s: %s\n",
			format.Dim(timestamp),
			format.Info(component),
			s.summary())
	}
}

func init() {
	OTACmd.AddCommand(watchCmd)
}
