package service

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

func init() {
	ServiceCmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List LibreScoot MDB services and their status",
	Long:  `List all LibreScoot systemd services running on the MDB with their current status.`,
	Run: func(cmd *cobra.Command, args []string) {
		// MDB services (lsc runs on Main Driver Board only)
		services := []string{
			"redis",
			"librescoot-vehicle",
			"librescoot-battery",
			"librescoot-ecu",
			"librescoot-modem",
			"librescoot-alarm",
			"librescoot-settings",
			"librescoot-keycard",
			"librescoot-boot-led",
			"librescoot-bluetooth",
			"librescoot-ums",
			"librescoot-onboot",
			"pm-service",
			"update-service",
			"version-service",
			"radio-gaga",
		}

		if *JSONOutput {
			outputJSON(services)
		} else {
			outputTable(services)
		}
	},
}

type serviceStatus struct {
	Name       string `json:"name"`
	Active     string `json:"active"`
	Enabled    string `json:"enabled"`
	Running    bool   `json:"running"`
	Status     string `json:"status"`
}

func getServiceStatus(service string) serviceStatus {
	status := serviceStatus{Name: service}

	// Get active state (running/failed/inactive)
	cmd := exec.Command("systemctl", "is-active", service)
	output, _ := cmd.Output()
	status.Active = strings.TrimSpace(string(output))
	status.Running = status.Active == "active"

	// Get enabled state
	cmd = exec.Command("systemctl", "is-enabled", service)
	output, _ = cmd.Output()
	status.Enabled = strings.TrimSpace(string(output))

	// Get one-line status
	cmd = exec.Command("systemctl", "show", service, "--property=StatusText", "--value")
	output, _ = cmd.Output()
	status.Status = strings.TrimSpace(string(output))

	return status
}

func outputJSON(services []string) {
	statuses := make([]serviceStatus, 0, len(services))
	for _, svc := range services {
		statuses = append(statuses, getServiceStatus(svc))
	}

	data, err := json.MarshalIndent(statuses, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling JSON: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

func outputTable(services []string) {
	fmt.Printf("%-30s %-10s %-10s\n", "SERVICE", "STATUS", "ENABLED")
	fmt.Println(strings.Repeat("─", 52))

	for _, svc := range services {
		status := getServiceStatus(svc)

		var statusStr string
		switch status.Active {
		case "active":
			statusStr = format.Success("active")
		case "failed":
			statusStr = format.Error("failed")
		case "inactive":
			statusStr = format.Warning("inactive")
		default:
			statusStr = status.Active
		}

		var enabledStr string
		switch status.Enabled {
		case "enabled":
			enabledStr = format.Success("enabled")
		case "disabled":
			enabledStr = format.Warning("disabled")
		default:
			enabledStr = status.Enabled
		}

		fmt.Printf("%-30s %-20s %-20s\n", status.Name, statusStr, enabledStr)
	}
}
