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
	Long: `Control vehicle lock/unlock state, hibernation, and seatbox.

Note: Handlebar lock/unlock is automatic and controlled by the vehicle state machine.
It unlocks when the vehicle is unlocked and locks when locked.`,
}

var vehicleLockCmd = &cobra.Command{
	Use:   "lock",
	Short: "Lock the scooter",
	Long:  `Lock the scooter and transition to stand-by state.`,
	Run: func(cmd *cobra.Command, args []string) {
		if !JSONOutput {
			fmt.Println("Locking scooter...")
		}

		// Send lock command
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

		// Wait for state to change to stand-by
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

var vehicleUnlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: "Unlock the scooter",
	Long:  `Unlock the scooter and transition to parked or ready-to-drive state.`,
	Run: func(cmd *cobra.Command, args []string) {
		if !JSONOutput {
			fmt.Println("Unlocking scooter...")
		}

		// Send unlock command
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

		// Wait for state to change (could be parked or ready-to-drive)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Subscribe and wait for state change
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
					// Check current state
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

var vehicleHibernateCmd = &cobra.Command{
	Use:   "hibernate",
	Short: "Lock and request hibernation",
	Long:  `Lock the scooter and request the system to enter hibernation mode.`,
	Run: func(cmd *cobra.Command, args []string) {
		if !JSONOutput {
			fmt.Println("Requesting hibernation...")
		}

		// Send lock-hibernate command
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

		if noBlock {
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
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := confirm.WaitForFieldValue(ctx, redisClient, "vehicle", "state", "stand-by", 10*time.Second); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "hibernate",
					"status":  "timeout",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Warning("Hibernate command sent but state confirmation timed out\n"))
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

var vehicleOpenCmd = &cobra.Command{
	Use:     "open",
	Aliases: []string{"open-seatbox"},
	Short:   "Open the seatbox",
	Long:    `Send command to open the seatbox lock.`,
	Run: func(cmd *cobra.Command, args []string) {
		if !JSONOutput {
			fmt.Println("Opening seatbox...")
		}

		// Send open command
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

		// Wait briefly for lock state to change to open
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := confirm.WaitForFieldValue(ctx, redisClient, "vehicle", "seatbox:lock", "open", 5*time.Second); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]interface{}{
					"command": "open",
					"status":  "timeout",
					"error":   err.Error(),
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

func init() {
	// Add --no-block flag to all vehicle commands
	vehicleCmd.PersistentFlags().BoolVar(&noBlock, "no-block", false, "Don't wait for state change confirmation")

	// Add subcommands
	vehicleCmd.AddCommand(vehicleLockCmd)
	vehicleCmd.AddCommand(vehicleUnlockCmd)
	vehicleCmd.AddCommand(vehicleHibernateCmd)
	vehicleCmd.AddCommand(vehicleOpenCmd)

	rootCmd.AddCommand(vehicleCmd)
}
