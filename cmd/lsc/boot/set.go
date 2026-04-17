package boot

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var setYes bool

var setCmd = &cobra.Command{
	Use:   "set <a|b|other|current|N>",
	Short: "Persistently set the Mender boot slot for all subsequent boots",
	Long: `Set mender_boot_part to the requested slot and clear upgrade_available
so the selection persists across reboots (no auto-rollback).

Argument may be:
  a, A                   — slot A
  b, B                   — slot B
  other, swap            — the currently inactive slot
  current                — the currently running slot (useful to clear a pending one-shot)
  <partition-number>     — raw partition number (e.g. 2, 3)`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		st, err := readBootState()
		if err != nil {
			fmt.Fprintf(os.Stderr, format.Error("boot set: %v\n"), err)
			os.Exit(1)
		}

		target, err := resolveTarget(st, args[0])
		if err != nil {
			fmt.Fprintf(os.Stderr, format.Error("boot set: %v\n"), err)
			os.Exit(1)
		}

		if !setYes {
			msg := fmt.Sprintf("Set next-boot slot to %s (part %d) persistently. Proceed?", target.Label, target.Num)
			if !confirmPrompt(msg) {
				fmt.Println("aborted")
				return
			}
		}

		err = fwSetenvBatch(map[string]string{
			"mender_boot_part":     fmt.Sprintf("%d", target.Num),
			"mender_boot_part_hex": fmt.Sprintf("%x", target.Num),
			"upgrade_available":    "0",
			"bootcount":            "0",
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, format.Error("fw_setenv failed: %v\n"), err)
			os.Exit(1)
		}

		if JSONOutput != nil && *JSONOutput {
			b, _ := json.Marshal(map[string]any{
				"action":     "set",
				"next_slot":  target.Label,
				"next_part":  target.Num,
				"persistent": true,
			})
			fmt.Println(string(b))
			return
		}

		fmt.Printf("%s next boot → slot %s (part %d), persistent.\n", format.Success("OK:"), target.Label, target.Num)
	},
}

func resolveTarget(st bootState, raw string) (slotInfo, error) {
	key := strings.ToLower(strings.TrimSpace(raw))
	switch key {
	case "a":
		return st.Layout.A, nil
	case "b":
		return st.Layout.B, nil
	case "other", "swap", "passive":
		return st.Layout.other(st.CurrentNum)
	case "current", "active":
		if s, ok := st.Layout.slotByNum(st.CurrentNum); ok {
			return s, nil
		}
		return slotInfo{}, fmt.Errorf("cannot resolve current slot")
	}
	if n, err := strconv.Atoi(key); err == nil {
		if s, ok := st.Layout.slotByNum(n); ok {
			return s, nil
		}
		return slotInfo{}, fmt.Errorf("partition %d is neither A (%d) nor B (%d)", n, st.Layout.A.Num, st.Layout.B.Num)
	}
	return slotInfo{}, fmt.Errorf("unrecognized target %q", raw)
}

func init() {
	setCmd.Flags().BoolVarP(&setYes, "yes", "y", false, "Skip confirmation prompt")
	BootCmd.AddCommand(setCmd)
}
