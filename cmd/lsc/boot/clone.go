package boot

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var (
	cloneArm bool
	cloneYes bool
)

var cloneCmd = &cobra.Command{
	Use:   "clone",
	Short: "Copy the running rootfs onto the other slot (live dd snapshot)",
	Long: `Snapshot the currently running rootfs block device onto the inactive slot.

Runs ` + "`sync`" + ` then ` + "`dd bs=4M conv=fsync status=progress`" + ` from the active
partition device to the passive one. Source is mounted read-write during the
copy, so the image captures a fuzzy, live state; ext4 journaling + fsck on
first mount clean that up, which is fine for a dev safety-net snapshot.

With --arm, also flips mender_boot_part to the freshly cloned slot so the
next reboot lands on the snapshot (equivalent to clone + ` + "`boot set other`" + `).`,
	Run: func(cmd *cobra.Command, args []string) {
		st, err := readBootState()
		if err != nil {
			fmt.Fprintf(os.Stderr, format.Error("boot clone: %v\n"), err)
			os.Exit(1)
		}

		src, ok := st.Layout.slotByNum(st.CurrentNum)
		if !ok {
			fmt.Fprintln(os.Stderr, format.Error("boot clone: cannot resolve current slot"))
			os.Exit(1)
		}
		dst, err := st.Layout.other(st.CurrentNum)
		if err != nil {
			fmt.Fprintf(os.Stderr, format.Error("boot clone: %v\n"), err)
			os.Exit(1)
		}

		srcSize, err := blockSize(src.Device)
		if err != nil {
			fmt.Fprintf(os.Stderr, format.Error("sizing %s: %v\n"), src.Device, err)
			os.Exit(1)
		}
		dstSize, err := blockSize(dst.Device)
		if err != nil {
			fmt.Fprintf(os.Stderr, format.Error("sizing %s: %v\n"), dst.Device, err)
			os.Exit(1)
		}
		if srcSize != dstSize {
			fmt.Fprintf(os.Stderr, format.Error("size mismatch: %s=%d bytes, %s=%d bytes\n"),
				src.Device, srcSize, dst.Device, dstSize)
			os.Exit(1)
		}

		if !cloneYes {
			msg := fmt.Sprintf("Clone %s (%s, %d bytes) → %s (%s) — DESTROYS existing contents on %s. Proceed?",
				src.Label, src.Device, srcSize, dst.Label, dst.Device, dst.Device)
			if !confirmPrompt(msg) {
				fmt.Println("aborted")
				return
			}
		}

		fmt.Printf("syncing filesystems...\n")
		if out, err := exec.Command("sync").CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, format.Error("sync failed: %v: %s\n"), err, strings.TrimSpace(string(out)))
			os.Exit(1)
		}

		fmt.Printf("cloning %s → %s (%d bytes)\n", src.Device, dst.Device, srcSize)
		dd := exec.Command("dd",
			"if="+src.Device,
			"of="+dst.Device,
			"bs=4M",
			"conv=fsync",
			"status=progress",
		)
		dd.Stdout = os.Stdout
		dd.Stderr = os.Stderr
		if err := dd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, format.Error("dd failed: %v\n"), err)
			os.Exit(1)
		}

		if cloneArm {
			if err := fwSetenvBatch(map[string]string{
				"mender_boot_part":     fmt.Sprintf("%d", dst.Num),
				"mender_boot_part_hex": fmt.Sprintf("%x", dst.Num),
				"upgrade_available":    "0",
				"bootcount":            "0",
			}); err != nil {
				fmt.Fprintf(os.Stderr, format.Error("clone ok but fw_setenv failed: %v\n"), err)
				os.Exit(1)
			}
		}

		if JSONOutput != nil && *JSONOutput {
			out := map[string]any{
				"action":     "clone",
				"from_slot":  src.Label,
				"from_part":  src.Num,
				"to_slot":    dst.Label,
				"to_part":    dst.Num,
				"bytes":      srcSize,
				"armed_next": cloneArm,
			}
			b, _ := json.Marshal(out)
			fmt.Println(string(b))
			return
		}

		fmt.Printf("%s cloned %s → %s\n", format.Success("OK:"), src.Device, dst.Device)
		if cloneArm {
			fmt.Printf("   next boot armed for slot %s (part %d).\n", dst.Label, dst.Num)
		}
	},
}

func blockSize(dev string) (int64, error) {
	out, err := exec.Command("blockdev", "--getsize64", dev).Output()
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing %q: %w", string(out), err)
	}
	return n, nil
}

func init() {
	cloneCmd.Flags().BoolVar(&cloneArm, "arm", false, "After cloning, set next boot to the cloned slot")
	cloneCmd.Flags().BoolVarP(&cloneYes, "yes", "y", false, "Skip confirmation prompt")
	BootCmd.AddCommand(cloneCmd)
}
