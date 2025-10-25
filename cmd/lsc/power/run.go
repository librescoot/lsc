package power

import (
	"fmt"
	"os"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Set power state to run",
	Long:  `Request the power manager to transition to run (normal operation) state.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := RedisClient.LPush("scooter:power", "run"); err != nil {
			fmt.Fprintf(os.Stderr, format.Error("Failed to send run command: %v\n"), err)
			return
		}

		fmt.Println(format.Success("Power state set to: run"))
	},
}

func init() {
	PowerCmd.AddCommand(runCmd)
}
