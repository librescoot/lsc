package lsc

import (
	"librescoot/lsc/cmd/lsc/diag"

	"github.com/spf13/cobra"
)

// Shortcut commands for common operations
// These shortcuts simply delegate to the real vehicle commands

// lock shortcut - delegates to vehicle lock
var lockCmd = &cobra.Command{
	Use:   "lock",
	Short: "Lock the scooter (shortcut for 'vehicle lock')",
	Run:   vehicleLockCmd.Run,
}

// unlock shortcut - delegates to vehicle unlock
var unlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: "Unlock the scooter (shortcut for 'vehicle unlock')",
	Run:   vehicleUnlockCmd.Run,
}

// open shortcut (seatbox) - delegates to vehicle open
var openCmd = &cobra.Command{
	Use:   "open",
	Short: "Open the seatbox (shortcut for 'vehicle open')",
	Run:   vehicleOpenCmd.Run,
}

// dbc, engine, and blink shortcuts - will be created by createDiagShortcut below

// createDiagShortcut creates a shortcut command that mirrors a diag subcommand
func createDiagShortcut(name string, aliases []string) *cobra.Command {
	// Find the real command
	var realCmd *cobra.Command
	for _, c := range diag.DiagCmd.Commands() {
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

	return shortcut
}

// get shortcut (get setting)
var getCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a setting value (shortcut for 'settings get')",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		settingsGetCmd.Run(cmd, args)
	},
}

// set shortcut (set setting)
var setCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a setting value (shortcut for 'settings set')",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		settingsSetCmd.Run(cmd, args)
	},
}

func init() {
	// Add --no-block flag to shortcuts that need it
	lockCmd.Flags().BoolVar(&noBlock, "no-block", false, "Don't wait for state change confirmation")
	unlockCmd.Flags().BoolVar(&noBlock, "no-block", false, "Don't wait for state change confirmation")
	openCmd.Flags().BoolVar(&noBlock, "no-block", false, "Don't wait for state change confirmation")

	// Add vehicle shortcut commands to root
	rootCmd.AddCommand(lockCmd)
	rootCmd.AddCommand(unlockCmd)
	rootCmd.AddCommand(openCmd)
	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(setCmd)

	// Create diagnostic shortcuts that mirror the full commands
	if batCmd := createDiagShortcut("battery", []string{"bat"}); batCmd != nil {
		rootCmd.AddCommand(batCmd)
	}
	if verCmd := createDiagShortcut("version", []string{"ver"}); verCmd != nil {
		rootCmd.AddCommand(verCmd)
	}
	if faultsCmd := createDiagShortcut("faults", nil); faultsCmd != nil {
		rootCmd.AddCommand(faultsCmd)
	}
	if eventsCmd := createDiagShortcut("events", nil); eventsCmd != nil {
		rootCmd.AddCommand(eventsCmd)
	}
	if dbcCmd := createDiagShortcut("dashboard", []string{"dbc", "dash"}); dbcCmd != nil {
		rootCmd.AddCommand(dbcCmd)
	}
	if engineCmd := createDiagShortcut("engine", nil); engineCmd != nil {
		rootCmd.AddCommand(engineCmd)
	}
	if blinkCmd := createDiagShortcut("blinkers", []string{"blink"}); blinkCmd != nil {
		rootCmd.AddCommand(blinkCmd)
	}
}
