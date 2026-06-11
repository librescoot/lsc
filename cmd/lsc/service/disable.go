package service

import (
	"fmt"
	"os/exec"

	"librescoot/lsc/internal/cli"

	"github.com/spf13/cobra"
)

func init() {
	ServiceCmd.AddCommand(disableCmd)
}

var disableCmd = &cobra.Command{
	Use:   "disable <service>",
	Short: "Disable a systemd service from starting on boot",
	Long:  `Disable a systemd service from starting automatically on boot. Service name can be with or without .service suffix.`,
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var failed bool
		for _, service := range args {
			serviceName := ensureServiceSuffix(service)

			err := exec.Command("systemctl", "disable", serviceName).Run()
			if err != nil {
				fmt.Printf("Failed to disable %s: %v\n", serviceName, err)
				failed = true
				continue
			}
			fmt.Printf("Disabled %s\n", serviceName)
		}
		if failed {
			return cli.ErrSilent
		}
		return nil
	},
}
