package service

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
)

func init() {
	ServiceCmd.AddCommand(enableCmd)
}

var enableCmd = &cobra.Command{
	Use:   "enable <service>",
	Short: "Enable a systemd service to start on boot",
	Long:  `Enable a systemd service to start automatically on boot. Service name can be with or without .service suffix.`,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		for _, service := range args {
			serviceName := ensureServiceSuffix(service)

			err := exec.Command("systemctl", "enable", serviceName).Run()
			if err != nil {
				fmt.Printf("Failed to enable %s: %v\n", serviceName, err)
				continue
			}
			fmt.Printf("Enabled %s\n", serviceName)
		}
	},
}
