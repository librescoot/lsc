package boot

import (
	"github.com/spf13/cobra"
)

var JSONOutput *bool

// AnnotationLocalOnly marks the commands that act on the U-Boot environment of
// the board lsc itself is running on. Those must work without Redis. The theme
// and sound commands are deliberately not marked: they act on the DBC, so they
// need Redis to reach the dispatcher that owns its environment.
const AnnotationLocalOnly = "librescoot.local-only"

// BootCmd is the parent for developer-only boot-partition commands.
// Hidden from `lsc --help`; still reachable as `lsc boot ...`.
var BootCmd = &cobra.Command{
	Use:   "boot",
	Short: "Developer: U-Boot env — A/B rootfs, boot theme, sound (hidden)",
	Long: `Developer-only commands to inspect and change the U-Boot environment.

Mender A/B rootfs selection and the boot theme/sound are separate concerns:
the slot commands edit this board's own environment, while the theme and sound
commands record a request in Redis that the DBC's dbc-dispatcher applies to
its own environment, whenever it is next running.`,
	Hidden: true,
}

func SetJSONOutput(j *bool) { JSONOutput = j }

func markLocalOnly(cmd *cobra.Command) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[AnnotationLocalOnly] = "true"
}
