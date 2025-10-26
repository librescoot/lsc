package service

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
)

func init() {
	ServiceCmd.AddCommand(restartCmd)
}

var restartCmd = &cobra.Command{
	Use:   "restart <service>",
	Short: "Restart a systemd service",
	Long:  `Restart a systemd service. Service name can be with or without .service suffix.`,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		for _, service := range args {
			serviceName := ensureServiceSuffix(service)

			err := exec.Command("systemctl", "restart", serviceName).Run()
			if err != nil {
				fmt.Printf("Failed to restart %s: %v\n", serviceName, err)
				continue
			}
			fmt.Printf("Restarted %s\n", serviceName)
		}
	},
}
