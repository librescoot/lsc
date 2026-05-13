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
	Run:     vehicleLockCmd.Run,
}

// unlock shortcut - delegates to vehicle unlock
var unlockCmd = &cobra.Command{
	Use:     "unlock",
	Short:   "Unlock the scooter (shortcut for 'vehicle unlock')",
	GroupID: "shortcuts",
	Run:     vehicleUnlockCmd.Run,
}

// open shortcut (seatbox) - delegates to vehicle open
var openCmd = &cobra.Command{
	Use:     "open",
	Short:   "Open the seatbox (shortcut for 'vehicle open')",
	GroupID: "shortcuts",
	Run:     vehicleOpenCmd.Run,
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

	// Create shortcut with same properties
	shortcut := &cobra.Command{
		Use:                realCmd.Use,
		Aliases:            aliases,
		Short:              realCmd.Short,
		Long:               realCmd.Long,
		GroupID:            "shortcuts",
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

	// Copy flags
	shortcut.Flags().AddFlagSet(realCmd.Flags())
	shortcut.PersistentFlags().AddFlagSet(realCmd.PersistentFlags())

	// Copy subcommands
	for _, subcmd := range realCmd.Commands() {
		shortcut.AddCommand(subcmd)
	}

	return shortcut
}

// get shortcut (get setting)
var getCmd = &cobra.Command{
	Use:     "get <key>",
	Short:   "Get a setting value (shortcut for 'settings get')",
	GroupID: "shortcuts",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		settingsGetCmd.Run(cmd, args)
	},
}

// set shortcut (set setting)
var setCmd = &cobra.Command{
	Use:     "set <key> <value>",
	Short:   "Set a setting value (shortcut for 'settings set')",
	GroupID: "shortcuts",
	Args:    cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		settingsSetCmd.Run(cmd, args)
	},
}

// del shortcut (delete setting)
var delCmd = &cobra.Command{
	Use:     "del <key>",
	Short:   "Delete a setting key (shortcut for 'settings del')",
	GroupID: "shortcuts",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		settingsDelCmd.Run(cmd, args)
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
