package lsc

import (
	"librescoot/lsc/cmd/lsc/diag"
	"librescoot/lsc/cmd/lsc/power"

	"github.com/spf13/cobra"
)

// Shortcut commands for common operations
// These shortcuts simply delegate to the real vehicle commands

// lock shortcut - delegates to vehicle lock
var lockCmd = &cobra.Command{
	Use:     "lock",
	Short:   "Lock the scooter (shortcut for 'vehicle lock')",
	GroupID: "shortcuts",
	RunE:    vehicleLockCmd.RunE,
}

// unlock shortcut - delegates to vehicle unlock
var unlockCmd = &cobra.Command{
	Use:     "unlock",
	Short:   "Unlock the scooter (shortcut for 'vehicle unlock')",
	GroupID: "shortcuts",
	RunE:    vehicleUnlockCmd.RunE,
}

// open shortcut (seatbox) - delegates to vehicle open
var openCmd = &cobra.Command{
	Use:     "open",
	Short:   "Open the seatbox (shortcut for 'vehicle open')",
	GroupID: "shortcuts",
	RunE:    vehicleOpenCmd.RunE,
}

// dbc, engine, and blink shortcuts - will be created by createDiagShortcut below

// createShortcut creates a shortcut command that mirrors a subcommand of parent.
func createShortcut(parent *cobra.Command, name string, aliases []string) *cobra.Command {
	var realCmd *cobra.Command
	for _, c := range parent.Commands() {
		if c.Name() == name {
			realCmd = c
			break
		}
	}
	if realCmd == nil {
		return nil
	}

	shortcut := copyCommand(realCmd)
	shortcut.Aliases = aliases
	shortcut.GroupID = "shortcuts"
	return shortcut
}

// copyCommand creates a copy of a command (and its subcommands) so the
// shortcut tree never re-parents the original commands. cobra commands have a
// single parent pointer; adding the original subcommand objects to a shortcut
// would detach them from their real parent in help output and flag
// inheritance.
func copyCommand(realCmd *cobra.Command) *cobra.Command {
	copied := &cobra.Command{
		Use:                realCmd.Use,
		Short:              realCmd.Short,
		Long:               realCmd.Long,
		Args:               realCmd.Args,
		ValidArgs:          realCmd.ValidArgs,
		ValidArgsFunction:  realCmd.ValidArgsFunction,
		Run:                realCmd.Run,
		RunE:               realCmd.RunE,
		PreRun:             realCmd.PreRun,
		PreRunE:            realCmd.PreRunE,
		PostRun:            realCmd.PostRun,
		PostRunE:           realCmd.PostRunE,
		PersistentPreRun:   realCmd.PersistentPreRun,
		PersistentPreRunE:  realCmd.PersistentPreRunE,
		PersistentPostRun:  realCmd.PersistentPostRun,
		PersistentPostRunE: realCmd.PersistentPostRunE,
	}

	// Share flag values with the real command so package-level flag
	// variables stay wired up
	copied.Flags().AddFlagSet(realCmd.Flags())
	copied.PersistentFlags().AddFlagSet(realCmd.PersistentFlags())

	for _, subcmd := range realCmd.Commands() {
		copied.AddCommand(copyCommand(subcmd))
	}

	return copied
}

// get shortcut (get setting)
var getCmd = &cobra.Command{
	Use:     "get <key> [<key>...]",
	Short:   "Get one or more setting values (shortcut for 'settings get')",
	GroupID: "shortcuts",
	Args:    cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return settingsGetCmd.RunE(cmd, args)
	},
}

// set shortcut (set setting)
var setCmd = &cobra.Command{
	Use:     "set <key> <value> [<key> <value>...]",
	Short:   "Set one or more setting values (shortcut for 'settings set')",
	GroupID: "shortcuts",
	Args:    settingsSetArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return settingsSetCmd.RunE(cmd, args)
	},
}

// del shortcut (delete setting)
var delCmd = &cobra.Command{
	Use:     "del <key>",
	Short:   "Delete a setting key (shortcut for 'settings del')",
	GroupID: "shortcuts",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return settingsDelCmd.RunE(cmd, args)
	},
}

// hibernate-for / hibernate-cancel shortcuts delegate to the power subcommand
// implementations so users can run `lsc hibernate-for 8h` without typing
// `lsc power hibernate-for 8h`.
var hibernateForShortcut = findPowerSubcommand("hibernate-for")
var hibernateCancelShortcut = findPowerSubcommand("hibernate-cancel")

func findPowerSubcommand(name string) *cobra.Command {
	for _, c := range power.PowerCmd.Commands() {
		if c.Name() == name {
			shortcut := &cobra.Command{
				Use:     c.Use,
				Short:   c.Short + " (shortcut for 'power " + c.Name() + "')",
				Long:    c.Long,
				GroupID: "shortcuts",
				Args:    c.Args,
				Run:     c.Run,
				RunE:    c.RunE,
			}
			shortcut.Flags().AddFlagSet(c.Flags())
			return shortcut
		}
	}
	return nil
}

func init() {
	// Add --no-block flag to shortcuts that need it
	lockCmd.Flags().BoolVar(&noBlock, "no-block", false, "Don't wait for state change confirmation")
	unlockCmd.Flags().BoolVar(&noBlock, "no-block", false, "Don't wait for state change confirmation")
	openCmd.Flags().BoolVar(&noBlock, "no-block", false, "Don't wait for state change confirmation")

	// Add --force flag to set shortcut
	setCmd.Flags().BoolVar(&forceSet, "force", false, "Skip validation and force set the value")

	// Add vehicle shortcut commands to root
	rootCmd.AddCommand(lockCmd)
	rootCmd.AddCommand(unlockCmd)
	rootCmd.AddCommand(openCmd)
	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(setCmd)
	rootCmd.AddCommand(delCmd)
	if hibernateForShortcut != nil {
		rootCmd.AddCommand(hibernateForShortcut)
	}
	if hibernateCancelShortcut != nil {
		rootCmd.AddCommand(hibernateCancelShortcut)
	}

	// Create diagnostic shortcuts that mirror the full commands
	if batCmd := createShortcut(diag.DiagCmd, "battery", []string{"bat"}); batCmd != nil {
		rootCmd.AddCommand(batCmd)
	}
	if verCmd := createShortcut(diag.DiagCmd, "version", []string{"ver"}); verCmd != nil {
		rootCmd.AddCommand(verCmd)
	}
	if faultsCmd := createShortcut(diag.DiagCmd, "faults", nil); faultsCmd != nil {
		rootCmd.AddCommand(faultsCmd)
	}
	if eventsCmd := createShortcut(diag.DiagCmd, "events", nil); eventsCmd != nil {
		rootCmd.AddCommand(eventsCmd)
	}
	if dbcCmd := createShortcut(diag.DiagCmd, "dashboard", []string{"dbc", "dash"}); dbcCmd != nil {
		rootCmd.AddCommand(dbcCmd)
	}
	if engineCmd := createShortcut(diag.DiagCmd, "engine", nil); engineCmd != nil {
		rootCmd.AddCommand(engineCmd)
	}
	if blinkCmd := createShortcut(diag.DiagCmd, "blinkers", []string{"blink"}); blinkCmd != nil {
		rootCmd.AddCommand(blinkCmd)
	}
	if blCmd := createShortcut(diag.DiagCmd, "backlight", nil); blCmd != nil {
		rootCmd.AddCommand(blCmd)
	}

	if hibCmd := createShortcut(power.PowerCmd, "hibernate", nil); hibCmd != nil {
		hibCmd.Short = "Set power state to hibernate (shortcut for 'power hibernate')"
		rootCmd.AddCommand(hibCmd)
	}
}
