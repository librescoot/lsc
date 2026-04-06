package lsc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"librescoot/lsc/internal/confirm"
	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var noBlock bool

var vehicleCmd = &cobra.Command{
	Use:   "vehicle",
	Short: "Control vehicle state and hardware",
	Long: `Control vehicle lock/unlock state, hibernation, handlebar lock, and seatbox.`,
}

var vehicleLockCmd = &cobra.Command{
	Use:   "lock",
	Short: "Lock the scooter",
	Long:  `Lock the scooter and transition to stand-by state.`,
	Run: func(cmd *cobra.Command, args []string) {
		if !JSONOutput {
			fmt.Println("Locking scooter...")
		}

		if noBlock {
			// Send lock command without waiting
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

		// Wait for state to change to stand-by
		// Subscribe first, then send command to avoid missing the notification
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		err := confirm.WaitForFieldValueAfterCommand(ctx, redisClient, "vehicle", "state", "stand-by", 15*time.Second, func() error {
			return redisClient.LPush("scooter:state", "lock")
		})

		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "lock",
					"status":  "timeout",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprint(os.Stderr, format.Warning("Lock command sent but state confirmation timed out\n"))
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

var vehicleUnlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: "Unlock the scooter",
	Long:  `Unlock the scooter and transition to parked or ready-to-drive state.`,
	Run: func(cmd *cobra.Command, args []string) {
		if !JSONOutput {
			fmt.Println("Unlocking scooter...")
		}

		if noBlock {
			// Send unlock command without waiting
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

		// Wait for state to change (could be parked or ready-to-drive)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		state, err := confirm.WaitForFieldAnyValueAfterCommand(ctx, redisClient, "vehicle", "state", []string{"parked", "ready-to-drive"}, 10*time.Second, func() error {
			return redisClient.LPush("scooter:state", "unlock")
		})

		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "unlock",
					"status":  "timeout",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprint(os.Stderr, format.Warning("Unlock command sent but state confirmation timed out\n"))
			}
			return
		}

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
	},
}

var vehicleHibernateCmd = &cobra.Command{
	Use:   "hibernate",
	Short: "Lock and request hibernation",
	Long:  `Lock the scooter and request the system to enter hibernation mode.`,
	Run: func(cmd *cobra.Command, args []string) {
		if !JSONOutput {
			fmt.Println("Requesting hibernation...")
		}

		if noBlock {
			// Send hibernate command without waiting
			if err := redisClient.LPush("scooter:state", "lock-hibernate"); err != nil {
				if JSONOutput {
					output, _ := json.Marshal(map[string]interface{}{
						"command": "hibernate",
						"status":  "error",
						"error":   err.Error(),
					})
					fmt.Println(string(output))
				} else {
					fmt.Fprintf(os.Stderr, format.Error("Failed to send hibernate command: %v\n"), err)
				}
				return
			}

			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "hibernate",
					"status":  "sent",
				})
				fmt.Println(string(output))
			} else {
				fmt.Println(format.Success("Hibernate command sent"))
			}
			return
		}

		// Wait for state to change to stand-by
		// Subscribe first, then send command to avoid missing the notification
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		err := confirm.WaitForFieldValueAfterCommand(ctx, redisClient, "vehicle", "state", "stand-by", 15*time.Second, func() error {
			return redisClient.LPush("scooter:state", "lock-hibernate")
		})

		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "hibernate",
					"status":  "timeout",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprint(os.Stderr, format.Warning("Hibernate command sent but state confirmation timed out\n"))
			}
			return
		}

		if JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "hibernate",
				"status":  "success",
				"state":   "stand-by",
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success("Hibernation requested successfully"))
		}
	},
}

var vehicleForceLockCmd = &cobra.Command{
	Use:   "force-lock",
	Short: "Force lock without physical locking",
	Long:  `Force the scooter into stand-by state without waiting for physical locks to engage. Use with caution.`,
	Run: func(cmd *cobra.Command, args []string) {
		if !JSONOutput {
			fmt.Println("Force locking scooter...")
		}

		if noBlock {
			// Send force-lock command without waiting
			if err := redisClient.LPush("scooter:state", "force-lock"); err != nil {
				if JSONOutput {
					output, _ := json.Marshal(map[string]interface{}{
						"command": "force-lock",
						"status":  "error",
						"error":   err.Error(),
					})
					fmt.Println(string(output))
				} else {
					fmt.Fprintf(os.Stderr, format.Error("Failed to send force-lock command: %v\n"), err)
				}
				return
			}

			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "force-lock",
					"status":  "sent",
				})
				fmt.Println(string(output))
			} else {
				fmt.Println(format.Success("Force-lock command sent"))
			}
			return
		}

		// Wait for state to change to stand-by
		// Subscribe first, then send command to avoid missing the notification
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		err := confirm.WaitForFieldValueAfterCommand(ctx, redisClient, "vehicle", "state", "stand-by", 15*time.Second, func() error {
			return redisClient.LPush("scooter:state", "force-lock")
		})

		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "force-lock",
					"status":  "timeout",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprint(os.Stderr, format.Warning("Force-lock command sent but state confirmation timed out\n"))
			}
			return
		}

		if JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "force-lock",
				"status":  "success",
				"state":   "stand-by",
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success("Scooter force-locked successfully"))
		}
	},
}

var vehicleOpenCmd = &cobra.Command{
	Use:     "open",
	Aliases: []string{"open-seatbox"},
	Short:   "Open the seatbox",
	Long:    `Send command to open the seatbox lock.`,
	Run: func(cmd *cobra.Command, args []string) {
		if !JSONOutput {
			fmt.Println("Opening seatbox...")
		}

		if noBlock {
			// Send open command without waiting
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

		// Wait briefly for lock state to change to open
		// Subscribe first, then send command to avoid missing the notification
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := confirm.WaitForFieldValueAfterCommand(ctx, redisClient, "vehicle", "seatbox:lock", "open", 5*time.Second, func() error {
			return redisClient.LPush("scooter:seatbox", "open")
		})

		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "open",
					"status":  "timeout",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprint(os.Stderr, format.Warning("Seatbox command sent but lock confirmation timed out\n"))
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

var vehicleHandlebarLockCmd = &cobra.Command{
	Use:   "handlebar-lock",
	Short: "Lock the handlebar",
	Long:  `Engage the handlebar lock mechanism. Normally handled automatically when locking the vehicle.`,
	Run: func(cmd *cobra.Command, args []string) {
		if !JSONOutput {
			fmt.Println("Locking handlebar...")
		}

		if noBlock {
			if err := redisClient.LPush("scooter:hardware", "handlebar:lock"); err != nil {
				if JSONOutput {
					output, _ := json.Marshal(map[string]interface{}{
						"command": "handlebar-lock",
						"status":  "error",
						"error":   err.Error(),
					})
					fmt.Println(string(output))
				} else {
					fmt.Fprintf(os.Stderr, format.Error("Failed to send handlebar lock command: %v\n"), err)
				}
				return
			}

			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "handlebar-lock",
					"status":  "sent",
				})
				fmt.Println(string(output))
			} else {
				fmt.Println(format.Success("Handlebar lock command sent"))
			}
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := confirm.WaitForFieldValueAfterCommand(ctx, redisClient, "vehicle", "handlebar:lock-sensor", "locked", 5*time.Second, func() error {
			return redisClient.LPush("scooter:hardware", "handlebar:lock")
		})

		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "handlebar-lock",
					"status":  "timeout",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprint(os.Stderr, format.Warning("Handlebar lock command sent but sensor confirmation timed out\n"))
			}
			return
		}

		if JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "handlebar-lock",
				"status":  "success",
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success("Handlebar locked successfully"))
		}
	},
}

var vehicleHandlebarUnlockCmd = &cobra.Command{
	Use:   "handlebar-unlock",
	Short: "Unlock the handlebar",
	Long:  `Disengage the handlebar lock mechanism. Normally handled automatically when unlocking the vehicle.`,
	Run: func(cmd *cobra.Command, args []string) {
		if !JSONOutput {
			fmt.Println("Unlocking handlebar...")
		}

		if noBlock {
			if err := redisClient.LPush("scooter:hardware", "handlebar:unlock"); err != nil {
				if JSONOutput {
					output, _ := json.Marshal(map[string]interface{}{
						"command": "handlebar-unlock",
						"status":  "error",
						"error":   err.Error(),
					})
					fmt.Println(string(output))
				} else {
					fmt.Fprintf(os.Stderr, format.Error("Failed to send handlebar unlock command: %v\n"), err)
				}
				return
			}

			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "handlebar-unlock",
					"status":  "sent",
				})
				fmt.Println(string(output))
			} else {
				fmt.Println(format.Success("Handlebar unlock command sent"))
			}
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := confirm.WaitForFieldValueAfterCommand(ctx, redisClient, "vehicle", "handlebar:lock-sensor", "unlocked", 5*time.Second, func() error {
			return redisClient.LPush("scooter:hardware", "handlebar:unlock")
		})

		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "handlebar-unlock",
					"status":  "timeout",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprint(os.Stderr, format.Warning("Handlebar unlock command sent but sensor confirmation timed out\n"))
			}
			return
		}

		if JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": "handlebar-unlock",
				"status":  "success",
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success("Handlebar unlocked successfully"))
		}
	},
}

func init() {
	// Add --no-block flag to all vehicle commands
	vehicleCmd.PersistentFlags().BoolVar(&noBlock, "no-block", false, "Don't wait for state change confirmation")

	// Add subcommands
	vehicleCmd.AddCommand(vehicleLockCmd)
	vehicleCmd.AddCommand(vehicleUnlockCmd)
	vehicleCmd.AddCommand(vehicleForceLockCmd)
	vehicleCmd.AddCommand(vehicleHibernateCmd)
	vehicleCmd.AddCommand(vehicleOpenCmd)
	vehicleCmd.AddCommand(vehicleHandlebarLockCmd)
	vehicleCmd.AddCommand(vehicleHandlebarUnlockCmd)

	rootCmd.AddCommand(vehicleCmd)
	vehicleCmd.GroupID = "main"
}
