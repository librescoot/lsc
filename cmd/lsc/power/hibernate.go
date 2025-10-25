package power

import (
	"fmt"
	"os"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var (
	hibernateManual bool
	hibernateTimer  bool
)

var hibernateCmd = &cobra.Command{
	Use:   "hibernate",
	Short: "Set power state to hibernate",
	Long:  `Request the power manager to transition to hibernate (power off) state.`,
	Run: func(cmd *cobra.Command, args []string) {
		command := "hibernate"

		if hibernateManual {
			command = "hibernate-manual"
		} else if hibernateTimer {
			command = "hibernate-timer"
		}

		if err := RedisClient.LPush("scooter:power", command); err != nil {
			fmt.Fprintf(os.Stderr, format.Error("Failed to send hibernate command: %v\n"), err)
			return
		}

		fmt.Printf("%s Power state set to: %s\n", format.Success("✓"), command)
		fmt.Println(format.Warning("Warning: System will power off"))
	},
}

func init() {
	hibernateCmd.Flags().BoolVar(&hibernateManual, "manual", false, "Use hibernate-manual mode")
	hibernateCmd.Flags().BoolVar(&hibernateTimer, "timer", false, "Use hibernate-timer mode")

	PowerCmd.AddCommand(hibernateCmd)
}
