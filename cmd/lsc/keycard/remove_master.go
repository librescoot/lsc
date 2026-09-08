package keycard

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"librescoot/lsc/internal/format"
)

var removeMasterCmd = &cobra.Command{
	Use:               "remove-master <uid> [uid...]",
	Short:             "Remove one or more master keycard UIDs",
	Long:              `Remove one or more master keycard UIDs. Multiple UIDs can be provided as separate arguments.`,
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: completeMasterUIDs,
	RunE: func(cmd *cobra.Command, args []string) error {
		uids := make([]string, 0, len(args))
		for _, arg := range args {
			if err := validateUIDFormat(arg); err != nil {
				printError(fmt.Sprintf("Invalid UID %s", arg), err)
				return err
			}
			uids = append(uids, normalizeUID(arg))
		}

		useService := serviceRunning()
		if !useService {
			fallbackNotice()
		}

		removed := 0
		for _, uid := range uids {
			if useService {
				notFound, err := isResult("master:remove:"+uid, "not-found")
				if err != nil {
					printError(fmt.Sprintf("Failed to remove master %s", uid), err)
					return err
				}
				if !notFound {
					removed++
				}
				continue
			}

			// Masters carry no anti-lockout rule: a vehicle with no master is
			// recoverable, unlike one with no card that can unlock.
			if err := removeUIDFromFile(masterFilePath(), uid, false); err != nil {
				continue
			}
			removed++
		}

		if removed == 0 {
			err := fmt.Errorf("no UIDs found to remove")
			printError("Failed to remove master keycards", err)
			return err
		}

		// The last master gone means the next start re-arms boot-time master
		// bootstrap, and the first card presented then claims the vehicle.
		mastersLeft := -1
		rearmed := false
		if remaining, err := readKeycardFile(masterFilePath()); err == nil {
			realMasters, mastersDisabled := splitMasters(remaining)
			mastersLeft = len(realMasters)
			// The sentinel keeps the bootstrap disarmed, so an empty list is
			// not on its own enough to re-arm it.
			rearmed = mastersLeft == 0 && !mastersDisabled
		}

		if *JSONOutput {
			printJSONResponse("success", map[string]interface{}{
				"removed":           removed,
				"masters_remaining": mastersLeft,
				"bootstrap_rearmed": rearmed,
			}, nil)
		} else {
			printSuccess(fmt.Sprintf("Removed %d master keycard UID(s)", removed))
			if rearmed {
				fmt.Fprintf(os.Stderr, "%s\n", format.Error(
					"Warning: no master cards remain. The next start re-arms master bootstrap, "+
						"and the first card presented becomes master. Add a master card, or run "+
						"'redis-cli lpush scooter:keycard set-master:NONE' to disable physical masters."))
			}
		}
		return nil
	},
}
