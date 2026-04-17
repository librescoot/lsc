package boot

import (
	"encoding/json"
	"fmt"
	"os"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var commitYes bool

var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Clear a pending one-shot boot (upgrade_available=0, bootcount=0)",
	Long: `Commit the currently running slot as the permanent choice.

Use after a successful ` + "`try-other`" + ` or ` + "`armor`" + ` when you want to stay where you are.
This sets upgrade_available=0 and resets bootcount, so U-Boot stops
counting boots and won't roll back on the next reboot.`,
	Run: func(cmd *cobra.Command, args []string) {
		st, err := readBootState()
		if err != nil {
			fmt.Fprintf(os.Stderr, format.Error("boot commit: %v\n"), err)
			os.Exit(1)
		}

		if st.UpgradeAvailable != "1" {
			fmt.Println("nothing pending: upgrade_available is already 0")
			return
		}

		if !commitYes {
			msg := fmt.Sprintf("Commit slot %s (part %d) as permanent. Proceed?",
				labelFor(st.Layout, st.CurrentNum), st.CurrentNum)
			if !confirmPrompt(msg) {
				fmt.Println("aborted")
				return
			}
		}

		if err := fwSetenvBatch(map[string]string{
			"upgrade_available": "0",
			"bootcount":         "0",
		}); err != nil {
			fmt.Fprintf(os.Stderr, format.Error("fw_setenv failed: %v\n"), err)
			os.Exit(1)
		}

		if JSONOutput != nil && *JSONOutput {
			b, _ := json.Marshal(map[string]any{
				"action":     "commit",
				"slot":       labelFor(st.Layout, st.CurrentNum),
				"part":       st.CurrentNum,
				"persistent": true,
			})
			fmt.Println(string(b))
			return
		}

		fmt.Printf("%s committed slot %s (part %d); no rollback on next reboot.\n",
			format.Success("OK:"), labelFor(st.Layout, st.CurrentNum), st.CurrentNum)
	},
}

func init() {
	commitCmd.Flags().BoolVarP(&commitYes, "yes", "y", false, "Skip confirmation prompt")
	BootCmd.AddCommand(commitCmd)
}
