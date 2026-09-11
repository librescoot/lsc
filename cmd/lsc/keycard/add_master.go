package keycard

import (
	"fmt"

	"github.com/spf13/cobra"
)

var addMasterCmd = &cobra.Command{
	Use:   "add-master <uid> [uid...]",
	Short: "Add one or more master keycard UIDs",
	Long:  `Add one or more master keycard UIDs for learn mode. Multiple UIDs can be provided as separate arguments.`,
	Args:  cobra.MinimumNArgs(1),
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

		added := 0
		if !useService {
			var err error
			added, err = addUIDsToFile(masterFilePath(), uids)
			if err != nil {
				printError("Failed to add master", err)
				return err
			}
		} else {
			for _, uid := range uids {
				// already-registered covers a UID that is already a
				// master or already an authorized card; either way there is
				// nothing to add, so it counts as a skip and not a failure.
				skipped, err := isResult("master:add:"+uid, "already-registered")
				if err != nil {
					printError(fmt.Sprintf("Failed to add master %s", uid), err)
					return err
				}
				if !skipped {
					added++
				}
			}
		}

		if *JSONOutput {
			printJSONResponse("success", map[string]interface{}{"added": added}, nil)
		} else if added == 0 {
			printSuccess("All UIDs already exist")
		} else {
			printSuccess(fmt.Sprintf("Added %d master keycard UID(s)", added))
		}
		return nil
	},
}
