package lsc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"librescoot/lsc/internal/cli"
	"librescoot/lsc/internal/confirm"
	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var noBlock bool

// stateCommand describes a command queue push with optional confirmation via
// the vehicle hash pub/sub channel.
type stateCommand struct {
	name        string        // JSON "command" field
	progressMsg string        // printed before sending (text mode)
	queue       string        // Redis list to LPUSH
	payload     string        // command payload
	sentMsg     string        // printed after a --no-block send (text mode)
	field       string        // vehicle hash field to confirm
	expect      []string      // acceptable confirmation values
	timeout     time.Duration // confirmation timeout
	timeoutMsg  string        // printed when confirmation times out (text mode)
	successMsg  func(state string) string
	jsonState   bool // include confirmed state in JSON success output
}

func (sc stateCommand) run(cmd *cobra.Command, args []string) error {
	if !JSONOutput {
		fmt.Println(sc.progressMsg)
	}

	if noBlock {
		if err := redisClient.LPush(sc.queue, sc.payload); err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]any{
					"command": sc.name,
					"status":  "error",
					"error":   err.Error(),
				})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Failed to send %s command: %v\n"), sc.name, err)
			}
			return cli.ErrSilent
		}

		if JSONOutput {
			output, _ := json.Marshal(map[string]any{
				"command": sc.name,
				"status":  "sent",
			})
			fmt.Println(string(output))
		} else {
			fmt.Println(format.Success(sc.sentMsg))
		}
		return nil
	}

	// Subscribe first, then send command to avoid missing the notification
	ctx, cancel := context.WithTimeout(context.Background(), sc.timeout)
	defer cancel()

	state, err := confirm.WaitForFieldAnyValueAfterCommand(ctx, redisClient, "vehicle", sc.field, sc.expect, sc.timeout, func() error {
		return redisClient.LPush(sc.queue, sc.payload)
	})

	if err != nil {
		if JSONOutput {
			output, _ := json.Marshal(map[string]any{
				"command": sc.name,
				"status":  "timeout",
				"error":   err.Error(),
			})
			fmt.Println(string(output))
		} else {
			fmt.Fprint(os.Stderr, format.Warning(sc.timeoutMsg+"\n"))
		}
		return cli.ErrSilent
	}

	if JSONOutput {
		result := map[string]any{
			"command": sc.name,
			"status":  "success",
		}
		if sc.jsonState {
			result["state"] = state
		}
		output, _ := json.Marshal(result)
		fmt.Println(string(output))
	} else {
		fmt.Println(format.Success(sc.successMsg(state)))
	}
	return nil
}

var vehicleCmd = &cobra.Command{
	Use:   "vehicle",
	Short: "Control vehicle state and hardware",
	Long:  `Control vehicle lock/unlock state, hibernation, handlebar lock, and seatbox.`,
}

var vehicleLockCmd = &cobra.Command{
	Use:   "lock",
	Short: "Lock the scooter",
	Long:  `Lock the scooter and transition to stand-by state.`,
	RunE: stateCommand{
		name:        "lock",
		progressMsg: "Locking scooter...",
		sentMsg:     "Lock command sent",
		queue:       "scooter:state",
		payload:     "lock",
		field:       "state",
		expect:      []string{"stand-by"},
		timeout:     15 * time.Second,
		timeoutMsg:  "Lock command sent but state confirmation timed out",
		successMsg:  func(string) string { return "Scooter locked successfully" },
		jsonState:   true,
	}.run,
}

var vehicleUnlockCmd = &cobra.Command{
	Use:   "unlock",
	Short: "Unlock the scooter",
	Long:  `Unlock the scooter and transition to parked or ready-to-drive state.`,
	RunE: stateCommand{
		name:        "unlock",
		progressMsg: "Unlocking scooter...",
		sentMsg:     "Unlock command sent",
		queue:       "scooter:state",
		payload:     "unlock",
		field:       "state",
		expect:      []string{"parked", "ready-to-drive"},
		timeout:     10 * time.Second,
		timeoutMsg:  "Unlock command sent but state confirmation timed out",
		successMsg: func(state string) string {
			return fmt.Sprintf("Scooter unlocked successfully (state: %s)", state)
		},
		jsonState: true,
	}.run,
}

var vehicleHibernateCmd = &cobra.Command{
	Use:   "hibernate",
	Short: "Lock and request hibernation",
	Long:  `Lock the scooter and request the system to enter hibernation mode.`,
	RunE: stateCommand{
		name:        "hibernate",
		progressMsg: "Requesting hibernation...",
		sentMsg:     "Hibernate command sent",
		queue:       "scooter:state",
		payload:     "lock-hibernate",
		field:       "state",
		expect:      []string{"stand-by"},
		timeout:     15 * time.Second,
		timeoutMsg:  "Hibernate command sent but state confirmation timed out",
		successMsg:  func(string) string { return "Hibernation requested successfully" },
		jsonState:   true,
	}.run,
}

var vehicleForceLockCmd = &cobra.Command{
	Use:   "force-lock",
	Short: "Force lock without physical locking",
	Long:  `Force the scooter into stand-by state without waiting for physical locks to engage. Use with caution.`,
	RunE: stateCommand{
		name:        "force-lock",
		progressMsg: "Force locking scooter...",
		sentMsg:     "Force-lock command sent",
		queue:       "scooter:state",
		payload:     "force-lock",
		field:       "state",
		expect:      []string{"stand-by"},
		timeout:     15 * time.Second,
		timeoutMsg:  "Force-lock command sent but state confirmation timed out",
		successMsg:  func(string) string { return "Scooter force-locked successfully" },
		jsonState:   true,
	}.run,
}

var vehicleOpenCmd = &cobra.Command{
	Use:     "open",
	Aliases: []string{"open-seatbox"},
	Short:   "Open the seatbox",
	Long:    `Send command to open the seatbox lock.`,
	RunE: stateCommand{
		name:        "open",
		progressMsg: "Opening seatbox...",
		sentMsg:     "Seatbox open command sent",
		queue:       "scooter:seatbox",
		payload:     "open",
		field:       "seatbox:lock",
		expect:      []string{"open"},
		timeout:     5 * time.Second,
		timeoutMsg:  "Seatbox command sent but lock confirmation timed out",
		successMsg:  func(string) string { return "Seatbox opened successfully" },
	}.run,
}

var vehicleHandlebarLockCmd = &cobra.Command{
	Use:   "handlebar-lock",
	Short: "Lock the handlebar",
	Long:  `Engage the handlebar lock mechanism. Normally handled automatically when locking the vehicle.`,
	RunE: stateCommand{
		name:        "handlebar-lock",
		progressMsg: "Locking handlebar...",
		sentMsg:     "Handlebar lock command sent",
		queue:       "scooter:hardware",
		payload:     "handlebar:lock",
		field:       "handlebar:lock-sensor",
		expect:      []string{"locked"},
		timeout:     5 * time.Second,
		timeoutMsg:  "Handlebar lock command sent but sensor confirmation timed out",
		successMsg:  func(string) string { return "Handlebar locked successfully" },
	}.run,
}

var vehicleHandlebarUnlockCmd = &cobra.Command{
	Use:   "handlebar-unlock",
	Short: "Unlock the handlebar",
	Long:  `Disengage the handlebar lock mechanism. Normally handled automatically when unlocking the vehicle.`,
	RunE: stateCommand{
		name:        "handlebar-unlock",
		progressMsg: "Unlocking handlebar...",
		sentMsg:     "Handlebar unlock command sent",
		queue:       "scooter:hardware",
		payload:     "handlebar:unlock",
		field:       "handlebar:lock-sensor",
		expect:      []string{"unlocked"},
		timeout:     5 * time.Second,
		timeoutMsg:  "Handlebar unlock command sent but sensor confirmation timed out",
		successMsg:  func(string) string { return "Handlebar unlocked successfully" },
	}.run,
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
