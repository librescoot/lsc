package service

import (
	"fmt"
	"os/exec"

	"librescoot/lsc/internal/cli"

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
	RunE: func(cmd *cobra.Command, args []string) error {
		var failed bool
		for _, service := range args {
			serviceName := ensureServiceSuffix(service)

			err := exec.Command("systemctl", "stop", serviceName).Run()
			if err != nil {
				fmt.Printf("Failed to stop %s: %v\n", serviceName, err)
				failed = true
				continue
			}
			fmt.Printf("Stopped %s\n", serviceName)
		}
		if failed {
			return cli.ErrSilent
		}
		return nil
	},
}
