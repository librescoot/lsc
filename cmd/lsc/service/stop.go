package service

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
)

func init() {
	ServiceCmd.AddCommand(stopCmd)
}

var stopCmd = &cobra.Command{
	Use:   "stop <service>",
	Short: "Stop a systemd service",
	Long:  `Stop a systemd service. Service name can be with or without .service suffix.`,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		for _, service := range args {
			serviceName := ensureServiceSuffix(service)

			err := exec.Command("systemctl", "stop", serviceName).Run()
			if err != nil {
				fmt.Printf("Failed to stop %s: %v\n", serviceName, err)
				continue
			}
			fmt.Printf("Stopped %s\n", serviceName)
		}
	},
}
