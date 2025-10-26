package service

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var (
	followLogs bool
	tailLines  int
)

func init() {
	logsCmd.Flags().BoolVarP(&followLogs, "follow", "f", false, "Follow log output (like tail -f)")
	logsCmd.Flags().IntVarP(&tailLines, "lines", "n", 50, "Number of recent log lines to show")
	ServiceCmd.AddCommand(logsCmd)
}

var logsCmd = &cobra.Command{
	Use:   "logs <service>",
	Short: "Show recent logs from a systemd service",
	Long:  `Show recent logs from a systemd service using journalctl. Service name can be with or without .service suffix.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		service := args[0]
		serviceName := ensureServiceSuffix(service)

		// Build journalctl command
		cmdArgs := []string{"-u", serviceName, "-n", fmt.Sprintf("%d", tailLines)}
		if followLogs {
			cmdArgs = append(cmdArgs, "-f")
		}

		journalCmd := exec.Command("journalctl", cmdArgs...)
		journalCmd.Stdout = os.Stdout
		journalCmd.Stderr = os.Stderr
		journalCmd.Stdin = os.Stdin

		err := journalCmd.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to retrieve logs for %s: %v\n", serviceName, err)
		}
	},
}
