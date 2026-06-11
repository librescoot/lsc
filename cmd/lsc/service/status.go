package service

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"librescoot/lsc/internal/cli"

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
	RunE: func(cmd *cobra.Command, args []string) error {
		service := args[0]
		serviceName := ensureServiceSuffix(service)

		if *JSONOutput {
			status := getServiceStatus(serviceName)
			data, err := json.MarshalIndent(status, "", "  ")
			if err != nil {
				fmt.Printf("Error marshaling JSON: %v\n", err)
				return cli.ErrSilent
			}
			fmt.Println(string(data))
			return nil
		}
		// Use systemctl status for detailed output; propagate its exit status
		// (systemctl status exits non-zero for inactive/failed units)
		statusCmd := exec.Command("systemctl", "status", serviceName)
		statusCmd.Stdout = os.Stdout
		statusCmd.Stderr = os.Stderr
		if err := statusCmd.Run(); err != nil {
			return cli.ErrSilent
		}
		return nil
	},
}
