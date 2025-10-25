package lsc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"librescoot/lsc/cmd/lsc/diag"
	"librescoot/lsc/internal/confirm"
	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

// Shortcut commands for common operations

// lock shortcut
var lockCmd = &cobra.Command{
	Use:   "lock",
	Short: "Lock the scooter (shortcut for 'vehicle lock')",
	Run: func(cmd *cobra.Command, args []string) {
		if !JSONOutput {
			fmt.Println("Locking scooter...")
		}

		if err := redisClient.LPush("scooter:state", "lock"); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "lock",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to send lock command: %v\n"), err)
			}
			return
		}

		if noBlock {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "lock",
					"status":  "sent",
				})
				fmt.Println(string(output))
			} else {
				fmt.Println(format.Success("Lock command sent"))
			}
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := confirm.WaitForFieldValue(ctx, redisClient, "vehicle", "state", "stand-by", 10*time.Second); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "lock",
					"status":  "timeout",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Warning("Lock command sent but state confirmation timed out\n"))
			}
			return
		}

		if JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "lock",
				"status":  "success",
				"state":   "stand-by",
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success("Scooter locked successfully"))
		}
	},
}

// unlock shortcut
var unlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: "Unlock the scooter (shortcut for 'vehicle unlock')",
	Run: func(cmd *cobra.Command, args []string) {
		if !JSONOutput {
			fmt.Println("Unlocking scooter...")
		}

		if err := redisClient.LPush("scooter:state", "unlock"); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "unlock",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to send unlock command: %v\n"), err)
			}
			return
		}

		if noBlock {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "unlock",
					"status":  "sent",
				})
				fmt.Println(string(output))
			} else {
				fmt.Println(format.Success("Unlock command sent"))
			}
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		pubsub := redisClient.Subscribe(ctx, "vehicle")
		defer pubsub.Close()

		ch := pubsub.Channel()
		timeout := time.After(10 * time.Second)

		for {
			select {
			case <-timeout:
				if JSONOutput {
					output, _ := json.Marshal(map[string]interface{}{
						"command": "unlock",
						"status":  "timeout",
					})
					fmt.Println(string(output))
				} else {
					fmt.Fprintf(os.Stderr, format.Warning("Unlock command sent but state confirmation timed out\n"))
				}
				return
			case msg := <-ch:
				if msg.Payload == "state" {
					state, err := redisClient.HGet("vehicle", "state")
					if err == nil && (state == "parked" || state == "ready-to-drive") {
						if JSONOutput {
							output, _ := json.Marshal(map[string]interface{}{
								"command": "unlock",
								"status":  "success",
								"state":   state,
							})
							fmt.Println(string(output))
						} else {
							fmt.Println(format.Success(fmt.Sprintf("Scooter unlocked successfully (state: %s)", state)))
						}
						return
					}
				}
			}
		}
	},
}

// open shortcut (seatbox)
var openCmd = &cobra.Command{
	Use:   "open",
	Short: "Open the seatbox (shortcut for 'vehicle open')",
	Run: func(cmd *cobra.Command, args []string) {
		if !JSONOutput {
			fmt.Println("Opening seatbox...")
		}

		if err := redisClient.LPush("scooter:seatbox", "open"); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "open",
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to send seatbox open command: %v\n"), err)
			}
			return
		}

		if noBlock {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "open",
					"status":  "sent",
				})
				fmt.Println(string(output))
			} else {
				fmt.Println(format.Success("Seatbox open command sent"))
			}
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := confirm.WaitForFieldValue(ctx, redisClient, "vehicle", "seatbox:lock", "open", 5*time.Second); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "open",
					"status":  "timeout",
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Warning("Seatbox command sent but lock confirmation timed out\n"))
			}
			return
		}

		if JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "open",
				"status":  "success",
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success("Seatbox opened successfully"))
		}
	},
}

// dbc shortcut (dashboard control)
var dbcCmd = &cobra.Command{
	Use:       "dbc [on|off]",
	Aliases:   []string{"dash"},
	Short:     "Control dashboard power (shortcut for hardware command)",
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"on", "off"},
	Run: func(cmd *cobra.Command, args []string) {
		action := args[0]

		if action != "on" && action != "off" {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "dbc",
					"status":  "error",
					"error":   fmt.Sprintf("invalid action: %s", action),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Invalid action '%s'. Must be 'on' or 'off'\n"), action)
			}
			return
		}

		command := fmt.Sprintf("dashboard:%s", action)
		if err := redisClient.LPush("scooter:hardware", command); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "dbc",
					"action":  action,
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to send dashboard command: %v\n"), err)
			}
			return
		}

		if JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "dbc",
				"action":  action,
				"status":  "success",
			})
			fmt.Println(string(output))
		} else {
			fmt.Printf("%s Dashboard power: %s\n", format.Success("✓"), action)
		}
	},
}

// engine shortcut (engine power control)
var engineCmd = &cobra.Command{
	Use:       "engine [on|off]",
	Short:     "Control engine power (shortcut for hardware command)",
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"on", "off"},
	Run: func(cmd *cobra.Command, args []string) {
		action := args[0]

		if action != "on" && action != "off" {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "engine",
					"status":  "error",
					"error":   fmt.Sprintf("invalid action: %s", action),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Invalid action '%s'. Must be 'on' or 'off'\n"), action)
			}
			return
		}

		command := fmt.Sprintf("engine:%s", action)
		if err := redisClient.LPush("scooter:hardware", command); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "engine",
					"action":  action,
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to send engine command: %v\n"), err)
			}
			return
		}

		if JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "engine",
				"action":  action,
				"status":  "success",
			})
			fmt.Println(string(output))
		} else {
			fmt.Printf("%s Engine power: %s\n", format.Success("✓"), action)
		}
	},
}

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

	// Add vehicle/hardware shortcut commands to root
	rootCmd.AddCommand(lockCmd)
	rootCmd.AddCommand(unlockCmd)
	rootCmd.AddCommand(openCmd)
	rootCmd.AddCommand(dbcCmd)
	rootCmd.AddCommand(engineCmd)
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
}
