package boot

import (
	"github.com/spf13/cobra"
)

var JSONOutput *bool

// BootCmd is the parent for developer-only boot-partition commands.
// Hidden from `lsc --help`; still reachable as `lsc boot ...`.
var BootCmd = &cobra.Command{
	Use:    "boot",
	Short:  "Developer: manipulate A/B rootfs selection (hidden)",
	Long:   `Developer-only commands to inspect and change the Mender A/B rootfs partition via U-Boot env.`,
	Hidden: true,
}

func SetJSONOutput(j *bool) { JSONOutput = j }
