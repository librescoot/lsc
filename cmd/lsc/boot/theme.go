package boot

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

// Preferred display order for completion and for the "known themes" hint.
var canonicalBootThemes = []string{"librescoot", "windowsxp", "librescoot-xp", "coopertino"}

// The value ends up in the DBC's U-Boot environment, so keep it to a shape a
// theme name can take and nothing else. The DBC checks the name again against
// the animations it actually has.
var themeNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

var themeCmd = &cobra.Command{Use: "theme [name]",
	Short: "Show or select the DBC boot logo and animation theme",
	Long: `Show the DBC's boot theme, or request a different one.

The theme is the U-Boot variable boot_animation on the DBC; U-Boot expands it
into boot.animation=<name> (the userspace animation) and logo.name=<name> (the
kernel splash), so one name drives the whole boot sequence.

lsc records the request in Redis and the DBC's dbc-dispatcher applies it, so
this works whether or not the DBC is running: a request made while it is
asleep is applied when it next starts. Either way the theme shows on the next
boot, because U-Boot has already read its environment by the time the DBC can
write it.

A missing or unknown name falls back to the built-in logo and the librescoot
animation, so a bad value cannot leave the scooter unable to boot.`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: completeBootThemes,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 0 {
			showBootThemes()
			return
		}
		setBootTheme(args[0])
	},
}

func init() {
	BootCmd.AddCommand(themeCmd)
}

func validateBootTheme(name string) error {
	if !themeNameRe.MatchString(name) {
		return fmt.Errorf("invalid theme name %q", name)
	}
	return nil
}

// themeFromState is the DBC's last reported theme. ok is false when it has
// never reported, which is indistinguishable from an asleep DBC.
func themeFromState(state map[string]string) (theme string, ok bool) {
	theme = stateValue(state, themeField)
	if theme == "" {
		return "", false
	}
	return theme, true
}

func showBootThemes() {
	state, err := readDbcBootState()
	if err != nil {
		failBoot("theme", err)
	}

	theme, reported := themeFromState(state)
	note := stateValue(state, noteField)

	if JSONOutput != nil && *JSONOutput {
		out := map[string]any{
			"reported":  reported,
			"available": canonicalBootThemes,
		}
		if reported {
			out["theme"] = theme
			out["note"] = note
			out["updated"] = state["updated"]
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
		return
	}

	format.PrintSection("Boot theme (DBC)")
	fmt.Println()
	if !reported {
		format.PrintKV("current", "unknown — the DBC has not reported")
		fmt.Println()
		fmt.Println("   The DBC publishes its theme when it is running. A request set now")
		fmt.Println("   is applied the next time it starts.")
		return
	}
	format.PrintKV("current", theme)
	if note != "" && note != "ok" {
		format.PrintKV("last note", noteExplanation(note))
	}
	if age := describeAge(state); age != "" {
		format.PrintKV("reported", age)
	}
	fmt.Println()
	fmt.Println("   Select with `lsc boot theme <name>`. Applies on the next boot.")
}

func setBootTheme(name string) {
	name = strings.TrimSpace(name)
	if err := validateBootTheme(name); err != nil {
		failBoot("theme", err)
	}

	sentAt := time.Now().Unix()
	if err := requestBootChange(themeField, name); err != nil {
		failBoot("theme", err)
	}

	result := waitForApply(themeField, name, sentAt, bootApplyWait)

	if !result.Reported {
		reportQueued("boot theme", themeField, name)
		return
	}
	if !result.Applied {
		failBoot("theme", fmt.Errorf("the DBC refused %q: %s", name, noteExplanation(result.Note)))
	}

	if JSONOutput != nil && *JSONOutput {
		b, _ := json.Marshal(map[string]any{
			"action":   "theme",
			themeField: name,
			"applied":  true,
			"reboot":   "required",
		})
		fmt.Println(string(b))
		return
	}
	fmt.Printf("%s boot theme → %s.\n", format.Success("OK:"), name)
	fmt.Println("   Reboot to see it.")
}

// Completion only offers the shipped names: querying the DBC would either be
// stale or block the shell.
func completeBootThemes(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var matches []string
	for _, theme := range canonicalBootThemes {
		if strings.HasPrefix(theme, toComplete) {
			matches = append(matches, theme)
		}
	}
	return matches, cobra.ShellCompDirectiveNoFileComp
}
