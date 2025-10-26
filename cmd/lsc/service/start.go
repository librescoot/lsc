package service

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
)

func init() {
	ServiceCmd.AddCommand(startCmd)
}

var startCmd = &cobra.Command{
	Use:   "start <service>",
	Short: "Start a systemd service",
	Long:  `Start a systemd service. Service name can be with or without .service suffix.`,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		for _, service := range args {
			serviceName := ensureServiceSuffix(service)

			err := exec.Command("systemctl", "start", serviceName).Run()
			if err != nil {
				fmt.Printf("Failed to start %s: %v\n", serviceName, err)
				continue
			}
			fmt.Printf("Started %s\n", serviceName)
		}
	},
}
