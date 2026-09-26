package doctor

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"librescoot/lsc/internal/cli"
	"librescoot/lsc/internal/format"
	"librescoot/lsc/internal/redis"

	"github.com/spf13/cobra"
)

// RedisClient and JSONOutput are the globals this package's commands read,
// assigned by the root command the way the other command packages do it.
var (
	RedisClient *redis.Client
	JSONOutput  *bool
)

// SetRedisClient sets the Redis client for the doctor command.
func SetRedisClient(client *redis.Client) {
	RedisClient = client
}

// SetJSONOutput sets the JSON output flag reference for the doctor command.
func SetJSONOutput(jsonOutput *bool) {
	JSONOutput = jsonOutput
}

// DoctorCmd is read-only: it reports what looks wrong and leaves the fixing to a
// human, because the checks span subsystems that a script should not repair by
// guess.
var DoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check the scooter for anything that looks wrong",
	Long: `Run read-only checks and report anything that looks wrong.

Exits non-zero if a check needs attention, so a script or a support runbook can
use it as a gate. Warnings (an update waiting for its reboot, low space in /data,
a link running on its backup transport) are printed without failing.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return run(cmd)
	},
}

func run(cmd *cobra.Command) error {
	e := env{
		state:       RedisClient,
		failedUnits: failedUnits,
		freeBytes:   freeBytes,
		link:        localLink,
		now:         time.Now,
	}

	results := make([]Result, 0, 6)
	for _, c := range allChecks() {
		result := c.run(e)
		if result.Name == "" {
			result.Name = c.name
		}
		results = append(results, result)
	}

	if JSONOutput != nil && *JSONOutput {
		var attention, warnings, unknown int
		text := make([]VerdictText, 0, len(results))
		for _, result := range results {
			switch result.Verdict {
			case VerdictAttention:
				attention++
			case VerdictWarning:
				warnings++
			case VerdictUnknown:
				unknown++
			}
			text = append(text, VerdictText{
				Name:    result.Name,
				Verdict: result.Verdict.String(),
				Detail:  result.Detail,
				Hint:    result.Hint,
			})
		}
		payload, err := json.MarshalIndent(map[string]any{
			"checks":    text,
			"attention": attention,
			"warnings":  warnings,
			"unknown":   unknown,
		}, "", "  ")
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), format.Error("Error marshaling JSON: %v\n"), err)
			return cli.ErrSilent
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(payload))
		if attention > 0 {
			return cli.ErrSilent
		}
		return nil
	}

	format.PrintSection("Doctor")
	fmt.Println()

	attention, warnings := 0, 0
	width := 0
	for _, result := range results {
		if len(result.Name) > width {
			width = len(result.Name)
		}
	}

	out := cmd.OutOrStdout()
	for _, result := range results {
		switch result.Verdict {
		case VerdictAttention:
			attention++
		case VerdictWarning:
			warnings++
		}
		fmt.Fprintf(out, "%-*s  %-18s %s\n", width, result.Name,
			verdictText(result.Verdict), result.Detail)
		if result.Hint != "" && result.Verdict >= VerdictWarning && result.Verdict != VerdictUnknown {
			fmt.Fprintf(out, "%-*s  %-18s %s\n", width, "", "", format.Dim(result.Hint))
		}
	}

	fmt.Println()
	summary := fmt.Sprintf("%d checks, %d need attention", len(results), attention)
	if warnings > 0 {
		summary += fmt.Sprintf(", %d warning(s)", warnings)
	}
	fmt.Fprintf(out, "%s\n", summary)

	if attention > 0 {
		return cli.ErrSilent
	}
	return nil
}

func verdictText(v Verdict) string {
	switch v {
	case VerdictOK:
		return format.Success("ok")
	case VerdictWarning:
		return format.Warning("warning")
	case VerdictAttention:
		return format.Error("attention")
	}
	return format.Dim("unknown")
}

// localLink reports how this board routes traffic to its peer, read from the
// routing table and named from the image variant.
func localLink() (Link, error) {
	variant, err := boardVariant()
	if err != nil {
		return Link{}, err
	}

	var link Link
	peerAddress := ""
	switch {
	case strings.Contains(variant, "mdb"):
		link.Peer, peerAddress = "DBC", "192.168.7.2"
	case strings.Contains(variant, "dbc"):
		link.Peer, peerAddress = "MDB", "192.168.7.1"
	default:
		return Link{}, nil
	}

	out, err := exec.Command("ip", "route", "show", peerAddress+"/32").Output()
	if err != nil {
		return Link{}, err
	}
	// Both routes can be present; USB carries the lower metric and is the one
	// the kernel picks.
	switch {
	case strings.Contains(string(out), "dev usb0"):
		link.Path = LinkUSB
	case strings.Contains(string(out), "dev ppp0"):
		link.Path = LinkBackup
	default:
		link.Path = LinkDown
	}
	return link, nil
}

// boardVariant is VARIANT_ID from os-release, which the images set to the
// machine name (unu-mdb, unu-dbc, librescoot-dbc-rpi4). An empty result means
// this is not a scooter board.
func boardVariant() (string, error) {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if value, ok := strings.CutPrefix(line, "VARIANT_ID="); ok {
			return strings.Trim(strings.TrimSpace(value), `"`), nil
		}
	}
	return "", nil
}

// failedUnits lists systemd units in the failed state. --no-legend also drops the
// footer, so every line is a unit.
func failedUnits() []string {
	out, err := exec.Command("systemctl", "list-units", "--state=failed",
		"--no-legend", "--plain", "--no-pager").Output()
	if err != nil && len(out) == 0 {
		return nil
	}
	var units []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		// A unit line starts with a unit name; anything else is a footer.
		if !strings.Contains(fields[0], ".") {
			continue
		}
		units = append(units, fields[0])
	}
	return units
}

// freeBytes reports the space left on the filesystem holding path.
func freeBytes(path string) (free, total uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), stat.Blocks * uint64(stat.Bsize), nil
}
