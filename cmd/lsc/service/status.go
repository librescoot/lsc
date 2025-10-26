package service

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func init() {
	ServiceCmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status <service>",
	Short: "Show detailed status of a systemd service",
	Long:  `Show detailed status of a systemd service including active state, enabled state, and recent logs. Service name can be with or without .service suffix.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		service := args[0]
		serviceName := ensureServiceSuffix(service)

		if *JSONOutput {
			status := getServiceStatus(serviceName)
			data, err := json.MarshalIndent(status, "", "  ")
			if err != nil {
				fmt.Printf("Error marshaling JSON: %v\n", err)
				return
			}
			fmt.Println(string(data))
		} else {
			// Use systemctl status for detailed output
			cmd := exec.Command("systemctl", "status", serviceName)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Run()
		}
	},
}
