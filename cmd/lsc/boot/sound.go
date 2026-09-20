package boot

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var soundCmd = &cobra.Command{
	Use:   "sound [on|off|toggle]",
	Short: "Show or set the DBC boot animation sound",
	Long: `Show or set whether the DBC's boot animation plays its startup sound.

This is the DBC's U-Boot variable boot_sound. With it off the animation still
plays, silently. Like the theme, the request is recorded in Redis and applied
by the DBC's dbc-dispatcher, so it works whether or not the DBC is running and
takes effect on the next boot.`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgs:         []string{"on", "off", "toggle"},
	ValidArgsFunction: completeBootSoundArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			showBootSound()
			return
		}
		setBootSound(args[0])
	},
}

func init() {
	BootCmd.AddCommand(soundCmd)
}

// parseSoundValue interprets a stored value. Absent means on, which is what a
// device that has never had one selected should do. err is only for a value
// that is neither on nor off.
func parseSoundValue(raw string) (bool, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return true, fmt.Errorf("boot_sound holds an unusable value %q", raw)
	}
}

// resolveBootSound maps a user argument onto the next stored value.
func resolveBootSound(arg string, current bool) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "on", "true", "yes", "1":
		return true, nil
	case "off", "false", "no", "0":
		return false, nil
	case "toggle":
		return !current, nil
	default:
		return current, fmt.Errorf("expected on, off or toggle, got %q", arg)
	}
}

// soundFromState is the DBC's last reported sound setting. ok is false when it
// has never reported, which is indistinguishable from an asleep DBC.
func soundFromState(state map[string]string) (enabled bool, ok bool) {
	raw := stateValue(state, soundField)
	if raw == "" {
		return true, false
	}
	enabled, err := parseSoundValue(raw)
	if err != nil {
		return true, false
	}
	return enabled, true
}

func showBootSound() {
	state, err := readDbcBootState()
	if err != nil {
		failBoot("sound", err)
	}

	enabled, reported := soundFromState(state)
	note := stateValue(state, noteField)

	if JSONOutput != nil && *JSONOutput {
		out := map[string]any{"reported": reported}
		if reported {
			out["enabled"] = enabled
			out["note"] = note
			out["updated"] = state["updated"]
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
		return
	}

	format.PrintSection("Boot sound (DBC)")
	fmt.Println()
	if !reported {
		format.PrintKV("state", "unknown — the DBC has not reported")
		fmt.Println()
		fmt.Println("   The DBC publishes its state when it is running. A request set now")
		fmt.Println("   is applied the next time it starts.")
		return
	}
	stateText := "on"
	if !enabled {
		stateText = "off"
	}
	format.PrintKV("state", stateText)
	if note != "" && note != "ok" {
		format.PrintKV("last note", noteExplanation(note))
	}
	if age := describeAge(state); age != "" {
		format.PrintKV("reported", age)
	}
	fmt.Println()
	fmt.Println("   Set with `lsc boot sound on|off`. Applies on the next boot.")
}

func setBootSound(arg string) {
	state, err := readDbcBootState()
	if err != nil {
		failBoot("sound", err)
	}
	current, _ := soundFromState(state)

	enabled, err := resolveBootSound(arg, current)
	if err != nil {
		failBoot("sound", err)
	}
	value := "0"
	if enabled {
		value = "1"
	}

	sentAt := time.Now().Unix()
	if err := requestBootChange(soundField, value); err != nil {
		failBoot("sound", err)
	}

	result := waitForApply(soundField, value, sentAt, bootApplyWait)

	if !result.Reported {
		reportQueued("boot sound", soundField, value)
		return
	}
	if !result.Applied {
		failBoot("sound", fmt.Errorf("the DBC refused %q: %s", value, noteExplanation(result.Note)))
	}

	if JSONOutput != nil && *JSONOutput {
		b, _ := json.Marshal(map[string]any{
			"action":   "sound",
			soundField: value,
			"enabled":  enabled,
			"applied":  true,
			"reboot":   "required",
		})
		fmt.Println(string(b))
		return
	}
	stateText := "off"
	if enabled {
		stateText = "on"
	}
	fmt.Printf("%s boot sound %s.\n", format.Success("OK:"), stateText)
	fmt.Println("   Reboot to apply.")
}

func completeBootSoundArgs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var matches []string
	for _, option := range []string{"on", "off", "toggle"} {
		if strings.HasPrefix(option, toComplete) {
			matches = append(matches, option)
		}
	}
	return matches, cobra.ShellCompDirectiveNoFileComp
}
