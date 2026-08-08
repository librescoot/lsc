package lsc

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate completion script",
	Long: `To load completions:

Bash:

  $ source <(lsc completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ lsc completion bash > /etc/bash_completion.d/lsc
  # macOS:
  $ lsc completion bash > $(brew --prefix)/etc/bash_completion.d/lsc

Zsh:

  # If shell completion is not already enabled in your environment,
  # you will need to enable it.  You can execute the following once:

  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ lsc completion zsh > "${fpath[1]}/_lsc"

  # You will need to start a new shell for this setup to take effect.

Fish:

  $ lsc completion fish | source

  # To load completions for each session, execute once:
  $ lsc completion fish > ~/.config/fish/completions/lsc.fish

PowerShell:

  PS> lsc completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> lsc completion powershell > lsc.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			var buf bytes.Buffer
			if err := cmd.Root().GenBashCompletionV2(&buf, false); err != nil {
				return err
			}
			// Replace the fallback init_completion helper that depends on the
			// bash-completion package (_get_comp_words_by_ref) with an
			// equivalent implementation using bash built-ins.
			script := strings.Replace(buf.String(),
				"__lsc_init_completion()\n{\n    COMPREPLY=()\n    _get_comp_words_by_ref \"$@\" cur prev words cword\n}",
				"__lsc_init_completion()\n{\n    COMPREPLY=()\n    words=(\"${COMP_WORDS[@]}\")\n    cword=$COMP_CWORD\n    cur=\"${words[$cword]}\"\n    prev=\"${words[$((cword > 0 ? cword - 1 : 0))]}\"\n}",
				1)
			_, err := fmt.Fprint(os.Stdout, script)
			return err
		case "zsh":
			return cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			return cmd.Root().GenFishCompletion(os.Stdout, true)
		case "powershell":
			return cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
