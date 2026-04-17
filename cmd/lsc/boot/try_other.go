package boot

import (
	"encoding/json"
	"fmt"
	"os"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var tryOtherYes bool

var tryOtherCmd = &cobra.Command{
	Use:   "try-other",
	Short: "One-shot boot into the other slot; next reboot auto-rolls back",
	Long: `Schedule the next boot to use the inactive Mender rootfs slot, without committing.

This sets upgrade_available=1 and resets bootcount. After reboot you'll be
running on the other slot; if you reboot again without committing (by running
e.g. ` + "`mender-update commit`" + `), U-Boot will auto-rollback to the original slot
once bootcount exceeds bootlimit.`,
	Run: func(cmd *cobra.Command, args []string) {
		st, err := readBootState()
		if err != nil {
			fmt.Fprintf(os.Stderr, format.Error("boot try-other: %v\n"), err)
			os.Exit(1)
		}

		target, err := st.Layout.other(st.CurrentNum)
		if err != nil {
			fmt.Fprintf(os.Stderr, format.Error("boot try-other: %v\n"), err)
			os.Exit(1)
		}

		if !tryOtherYes {
			msg := fmt.Sprintf("Schedule one-shot boot from slot %s (part %d) → slot %s (part %d). Proceed?",
				labelFor(st.Layout, st.CurrentNum), st.CurrentNum, target.Label, target.Num)
			if !confirmPrompt(msg) {
				fmt.Println("aborted")
				return
			}
		}

		err = fwSetenvBatch(map[string]string{
			"mender_boot_part":     fmt.Sprintf("%d", target.Num),
			"mender_boot_part_hex": fmt.Sprintf("%x", target.Num),
			"upgrade_available":    "1",
			"bootcount":            "0",
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, format.Error("fw_setenv failed: %v\n"), err)
			os.Exit(1)
		}

		if JSONOutput != nil && *JSONOutput {
			b, _ := json.Marshal(map[string]any{
				"action":      "try-other",
				"next_slot":   target.Label,
				"next_part":   target.Num,
				"one_shot":    true,
				"rollback_on": "next reboot without commit",
			})
			fmt.Println(string(b))
			return
		}

		fmt.Printf("%s next boot → slot %s (part %d), one-shot.\n", format.Success("OK:"), target.Label, target.Num)
		fmt.Println("   Reboot now to test. A subsequent reboot without commit auto-rolls back.")
	},
}

func init() {
	tryOtherCmd.Flags().BoolVarP(&tryOtherYes, "yes", "y", false, "Skip confirmation prompt")
	BootCmd.AddCommand(tryOtherCmd)
}
