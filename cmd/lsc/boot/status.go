package boot

import (
	"encoding/json"
	"fmt"
	"os"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current and next-boot rootfs slot",
	Run: func(cmd *cobra.Command, args []string) {
		st, err := readBootState()
		if err != nil {
			if JSONOutput != nil && *JSONOutput {
				b, _ := json.Marshal(map[string]any{"error": err.Error()})
				fmt.Println(string(b))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("boot status: %v\n"), err)
			}
			os.Exit(1)
		}

		pending := st.UpgradeAvailable == "1"

		if JSONOutput != nil && *JSONOutput {
			out := map[string]any{
				"slots": map[string]any{
					"A": map[string]any{"num": st.Layout.A.Num, "device": st.Layout.A.Device},
					"B": map[string]any{"num": st.Layout.B.Num, "device": st.Layout.B.Device},
				},
				"current": map[string]any{
					"num":   st.CurrentNum,
					"label": labelFor(st.Layout, st.CurrentNum),
				},
				"next": map[string]any{
					"num":   st.NextNum,
					"label": labelFor(st.Layout, st.NextNum),
				},
				"upgrade_available": st.UpgradeAvailable,
				"bootcount":         st.BootCount,
				"bootlimit":         st.BootLimit,
				"one_shot":          pending,
			}
			b, _ := json.MarshalIndent(out, "", "  ")
			fmt.Println(string(b))
			return
		}

		format.PrintSection("Boot slot status")
		fmt.Println()
		format.PrintKV("slot A", fmt.Sprintf("%s (part %d)", st.Layout.A.Device, st.Layout.A.Num))
		format.PrintKV("slot B", fmt.Sprintf("%s (part %d)", st.Layout.B.Device, st.Layout.B.Num))
		format.PrintKV("running", fmt.Sprintf("%s (part %d)", labelFor(st.Layout, st.CurrentNum), st.CurrentNum))

		nextStr := fmt.Sprintf("%s (part %d)", labelFor(st.Layout, st.NextNum), st.NextNum)
		if st.NextNum != st.CurrentNum {
			nextStr = format.Warning(nextStr) + " (switch)"
		}
		format.PrintKV("next boot", nextStr)

		if pending {
			format.PrintKV("mode", format.Warning("one-shot (auto-rollback on next reboot unless committed)"))
		} else {
			format.PrintKV("mode", "persistent")
		}
		format.PrintKV("upgrade_available", format.SafeValue(st.UpgradeAvailable, "?"))
		format.PrintKV("bootcount", format.SafeValue(st.BootCount, "?"))
		format.PrintKV("bootlimit", format.SafeValue(st.BootLimit, "?"))
		fmt.Println()
	},
}

func init() {
	BootCmd.AddCommand(statusCmd)
}
