package boot

import (
	"encoding/json"
	"fmt"
	"os"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var armorYes bool

var armorCmd = &cobra.Command{
	Use:   "armor",
	Short: "Arm auto-rollback for the current slot (tentative boot with fallback)",
	Long: `Mark the next boot of the CURRENT slot as tentative, with fallback to the other slot.

Sets mender_boot_part=<current>, upgrade_available=1, bootcount=0.

Useful before changing something risky (kernel, bootloader, init scripts).
If the change works, run ` + "`lsc boot commit`" + ` to make the current slot permanent.
If the change breaks booting badly enough to reboot-loop, U-Boot will
eventually hit bootlimit and roll back to the other slot automatically.

Note: rollback only triggers on counted reboots. A kernel that hangs
without a watchdog reboot won't trigger fallback.`,
	Run: func(cmd *cobra.Command, args []string) {
		st, err := readBootState()
		if err != nil {
			fmt.Fprintf(os.Stderr, format.Error("boot armor: %v\n"), err)
			os.Exit(1)
		}

		other, err := st.Layout.other(st.CurrentNum)
		if err != nil {
			fmt.Fprintf(os.Stderr, format.Error("boot armor: %v\n"), err)
			os.Exit(1)
		}

		if !armorYes {
			msg := fmt.Sprintf("Arm rollback: next boot stays on %s (part %d); failures fall back to %s (part %d). Proceed?",
				labelFor(st.Layout, st.CurrentNum), st.CurrentNum, other.Label, other.Num)
			if !confirmPrompt(msg) {
				fmt.Println("aborted")
				return
			}
		}

		if err := fwSetenvBatch(map[string]string{
			"mender_boot_part":     fmt.Sprintf("%d", st.CurrentNum),
			"mender_boot_part_hex": fmt.Sprintf("%x", st.CurrentNum),
			"upgrade_available":    "1",
			"bootcount":            "0",
		}); err != nil {
			fmt.Fprintf(os.Stderr, format.Error("fw_setenv failed: %v\n"), err)
			os.Exit(1)
		}

		if JSONOutput != nil && *JSONOutput {
			b, _ := json.Marshal(map[string]any{
				"action":        "armor",
				"slot":          labelFor(st.Layout, st.CurrentNum),
				"part":          st.CurrentNum,
				"fallback_slot": other.Label,
				"fallback_part": other.Num,
				"one_shot":      true,
			})
			fmt.Println(string(b))
			return
		}

		fmt.Printf("%s armed: next boot tentative on %s (part %d); fallback to %s (part %d).\n",
			format.Success("OK:"), labelFor(st.Layout, st.CurrentNum), st.CurrentNum, other.Label, other.Num)
		fmt.Println("   Run `lsc boot commit` after a successful boot to keep the current slot.")
	},
}

func init() {
	armorCmd.Flags().BoolVarP(&armorYes, "yes", "y", false, "Skip confirmation prompt")
	BootCmd.AddCommand(armorCmd)
}
