package power

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"librescoot/lsc/internal/format"

	"github.com/spf13/cobra"
)

var hibernateForCmd = &cobra.Command{
	Use:   "hibernate-for <duration>",
	Short: "Hibernate the scooter for a specific duration",
	Long: `Request the power manager to hibernate (power off) the scooter and wake
it back up after the given duration. The duration accepts Go-style suffixes
(e.g. "30s", "10m", "8h"). The wake source is the nRF52 MCU, which keeps a
single-shot timer running while the iMX6 is powered down.

Example:
  lsc hibernate-for 8h    Hibernate until ~8 hours from now
  lsc power hibernate-for 30m  Same, via the power subcommand`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dur, err := time.ParseDuration(args[0])
		if err != nil {
			emitErr(args[0], fmt.Errorf("invalid duration: %w", err))
			return
		}
		seconds := int64(dur.Seconds())
		if seconds <= 0 {
			emitErr(args[0], fmt.Errorf("duration must be positive"))
			return
		}

		command := fmt.Sprintf("hibernate-for:%d", seconds)
		if err := RedisClient.LPush("scooter:power", command); err != nil {
			emitErr(command, fmt.Errorf("failed to send: %w", err))
			return
		}

		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command":  command,
				"duration": dur.String(),
				"seconds":  seconds,
				"status":   "success",
			})
			fmt.Println(string(output))
		} else {
			fmt.Printf("%s Hibernate-for sent: %s\n", format.Success("✓"), dur)
			fmt.Println(format.Warning("Warning: System will power off and wake up after the duration"))
		}
	},
}

var hibernateCancelCmd = &cobra.Command{
	Use:   "hibernate-cancel",
	Short: "Cancel a pending hibernate-for and disarm the wake timer",
	Long: `Return the power state to run and clear any wake-timer programmed on
the nRF52. Use this to abort a hibernate-for between sending the command and
the system actually powering off, or to invalidate a stale wake schedule.`,
	Run: func(cmd *cobra.Command, args []string) {
		const command = "hibernate-cancel"
		if err := RedisClient.LPush("scooter:power", command); err != nil {
			emitErr(command, fmt.Errorf("failed to send: %w", err))
			return
		}
		if JSONOutput != nil && *JSONOutput {
			output, _ := json.Marshal(map[string]interface{}{
				"command": command,
				"status":  "success",
			})
			fmt.Println(string(output))
		} else {
			fmt.Printf("%s Hibernate-cancel sent\n", format.Success("✓"))
		}
	},
}

func emitErr(command string, err error) {
	if JSONOutput != nil && *JSONOutput {
		output, _ := json.Marshal(map[string]interface{}{
			"command": command,
			"status":  "error",
			"error":   err.Error(),
		})
		fmt.Println(string(output))
		return
	}
	fmt.Fprintf(os.Stderr, format.Error("%v\n"), err)
}

func init() {
	PowerCmd.AddCommand(hibernateForCmd)
	PowerCmd.AddCommand(hibernateCancelCmd)
}
