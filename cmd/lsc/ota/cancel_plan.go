package ota

import (
	"fmt"

	"github.com/spf13/cobra"
)

var cancelPlanCmd = &cobra.Command{
	Use:   "cancel-plan <mdb|dbc>",
	Short: "Request cancellation of a stopped v2 transport plan",
	Long:  "Request cancellation of a stopped OTA transport plan. This does not roll back Mender and is refused while an update or pending commit is active.",
	Args:  cobra.ExactArgs(1), ValidArgs: []string{"mdb", "dbc"},
	RunE: func(cmd *cobra.Command, args []string) error {
		component := args[0]
		if component != "mdb" && component != "dbc" {
			return fmt.Errorf("invalid component %q", component)
		}
		if err := RedisClient.LPush("scooter:update:"+component, "cancel-v2-plan"); err != nil {
			return err
		}
		fmt.Printf("Plan cancellation requested for %s; inspect OTA status for the result\n", component)
		return nil
	},
}

func init() { OTACmd.AddCommand(cancelPlanCmd) }
