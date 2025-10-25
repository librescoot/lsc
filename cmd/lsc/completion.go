package lsc

import (
	"os"

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
	Args:                  cobra.ExactValidArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		switch args[0] {
		case "bash":
			cmd.Root().GenBashCompletion(os.Stdout)
		case "zsh":
			cmd.Root().GenZshCompletion(os.Stdout)
		case "fish":
			cmd.Root().GenFishCompletion(os.Stdout, true)
		case "powershell":
			cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
		}
	},
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
