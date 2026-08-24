package lsc

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"librescoot/lsc/internal/cli"
	"librescoot/lsc/internal/format"
	"librescoot/lsc/internal/mapstate"

	"github.com/spf13/cobra"
)

var mapsCmd = &cobra.Command{
	Use:     "maps",
	Aliases: []string{"tiles"},
	Short:   "Show installed map and routing tiles",
	Long: `Display which offline map and routing tiles are installed on the dashboard.

The 'maps' hash lives on the MDB but describes files under the DBC's /data, so
this works with the dashboard powered off. The installer writes it while
provisioning, ums-service after a USB map update, and the dashboard rewrites it
whenever it runs.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fields, err := redisClient.HGetAll(mapstate.Hash)
		if err != nil {
			if JSONOutput {
				output, _ := json.Marshal(map[string]any{"error": err.Error()})
				fmt.Println(string(output))
			} else {
				fmt.Fprintf(os.Stderr, format.Error("Error fetching map state: %v\n"), err)
			}
			return cli.ErrSilent
		}

		state := mapstate.Parse(fields)

		if JSONOutput {
			jsonBytes, _ := json.MarshalIndent(state.JSON(), "", "  ")
			fmt.Println(string(jsonBytes))
			return nil
		}

		printMapState(state, time.Now())
		return nil
	},
}

func printMapState(state mapstate.State, now time.Time) {
	format.PrintSection("Installed Maps")

	if !state.Recorded {
		fmt.Println(format.Dim("No map state recorded."))
		fmt.Println(format.Dim("The hash does not survive an MDB reboot and comes back when the dashboard"))
		fmt.Println(format.Dim("next boots, so this says nothing about what is installed on the DBC."))
		fmt.Println()
		return
	}

	if label := state.RegionLabel(); label != "" {
		format.PrintKV("Region", label)
	} else {
		format.PrintKV("Region", format.Dim("unknown"))
	}

	if state.AnyInstalled() {
		printMapArtifact("Map tiles", state.Map, now)
		printMapArtifact("Routing tiles", state.Routing, now)
	} else {
		format.PrintKV("Tiles", format.Dim("none installed"))
	}

	fmt.Println()
	if state.LastUpdateCheck != "" {
		format.PrintKV("Update check", mapstate.Timestamp(state.LastUpdateCheck, now))
	}
	switch {
	case !state.UpdateAvailableKnown:
		format.PrintKV("Update available", format.Dim("unknown"))
	case state.UpdateAvailable:
		format.PrintKV("Update available", format.Warning("yes"))
	default:
		format.PrintKV("Update available", "no")
	}
	if state.UpdatedAt != "" {
		format.PrintKV("Recorded", mapstate.Timestamp(state.UpdatedAt, now))
	}

	fmt.Println()
}

// printMapArtifact prints the detail block for an installed artifact. An
// artifact that is not on disk gets a single line instead of a block of empty
// fields.
func printMapArtifact(title string, a mapstate.Artifact, now time.Time) {
	if !a.Installed {
		fmt.Println()
		format.PrintKV(title, format.Dim("not installed"))
		return
	}

	format.PrintSubsection(title + ":")
	if a.SizeKnown {
		format.PrintKV("  Size", format.Bytes(a.Size))
	}
	if a.MTime != "" {
		format.PrintKV("  Written", mapstate.Timestamp(a.MTime, now))
	}
	if a.SHA256 != "" {
		format.PrintKV("  SHA-256", a.SHA256)
	}
	if a.PublishedAt != "" {
		format.PrintKV("  Published", mapstate.Timestamp(a.PublishedAt, now))
	}
	if !a.HasProvenance() {
		format.PrintKV("  Provenance", format.Dim("unknown, no release recorded"))
	}
}

func init() {
	rootCmd.AddCommand(mapsCmd)
	mapsCmd.GroupID = "main"
}
