package service

import (
	"fmt"
	"os/exec"

	"librescoot/lsc/internal/cli"

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
	RunE: func(cmd *cobra.Command, args []string) error {
		var failed bool
		for _, service := range args {
			serviceName := ensureServiceSuffix(service)

			err := exec.Command("systemctl", "start", serviceName).Run()
			if err != nil {
				fmt.Printf("Failed to start %s: %v\n", serviceName, err)
				failed = true
				continue
			}
			fmt.Printf("Started %s\n", serviceName)
		}
		if failed {
			return cli.ErrSilent
		}
		return nil
	},
}
