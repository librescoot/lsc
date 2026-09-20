package boot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"librescoot/lsc/internal/format"
	"librescoot/lsc/internal/redis"
)

// The DBC's boot theme and startup sound live in its U-Boot environment, and
// only a running DBC can write that: U-Boot reads it before Linux exists, and
// the MDB has no path to the DBC's eMMC. So lsc does not reach across to the
// DBC at all. It records the desired value in Redis and dbc-dispatcher on the
// DBC reconciles it — on every start, and immediately on every live request.
//
// That makes a request survive the DBC being powered off, which matters
// because the scooter spends most of its life asleep. The trade is one boot of
// lag when the DBC was off: it applies while running, and U-Boot has already
// read the environment for that boot.
const (
	bootDesiredKey = "boot:desired"
	bootStateKey   = "boot:dbc"
	bootChannel    = "boot:command"

	themeField = "theme"
	soundField = "sound"
	noteField  = "note"

	// How long to wait for the DBC to answer before assuming it is asleep.
	// A live DBC answers in milliseconds.
	bootApplyWait = 2 * time.Second
)

// Set by the root command.
var redisClient *redis.Client

func SetRedisClient(client *redis.Client) { redisClient = client }

// bootState is what the DBC last published about its own environment. Empty
// when the DBC has never reported, which is what an asleep DBC looks like.
func readDbcBootState() (map[string]string, error) {
	if redisClient == nil {
		return nil, errors.New("no Redis connection")
	}
	return redisClient.HGetAll(bootStateKey)
}

// requestBootChange records the desired value, then nudges the DBC. The nudge
// is only the fast path: if the DBC is off, the desired value alone is what
// makes the request survive until it next starts.
func requestBootChange(field, value string) error {
	if redisClient == nil {
		return errors.New("no Redis connection")
	}
	if err := redisClient.HSet(bootDesiredKey, field, value); err != nil {
		return err
	}
	// Message shape dbc-dispatcher expects: "boot-theme <name>", "boot-sound off".
	if err := redisClient.Publish(context.Background(), bootChannel,
		fmt.Sprintf("boot-%s %s", field, value)); err != nil {
		fmt.Fprintf(os.Stderr, format.Warning("boot: could not nudge the DBC: %v\n"), err)
	}
	return nil
}

type applyResult struct {
	// Reported is false when the DBC never answered, which is not a failure:
	// it means the DBC is off and the request is queued.
	Reported bool
	Applied  bool
	Note     string
}

// waitForApply polls the published state for the DBC's answer. A note is only
// treated as this request's verdict when the state was written after the
// request went out, otherwise a refusal left over from an earlier attempt
// would be reported as the answer to this one.
func waitForApply(field, value string, sentAt int64, timeout time.Duration) applyResult {
	deadline := time.Now().Add(timeout)
	var result applyResult
	for {
		state, err := readDbcBootState()
		if err == nil {
			if updated, err := strconv.ParseInt(state["updated"], 10, 64); err == nil && updated >= sentAt {
				note := state[noteField]
				if state[field] == value && (note == "" || note == "ok") {
					return applyResult{Reported: true, Applied: true, Note: "ok"}
				}
				if state[field] != value && note != "" && note != "ok" {
					return applyResult{Reported: true, Note: note}
				}
				result = applyResult{Reported: true, Note: note}
			}
		}
		if time.Now().After(deadline) {
			return result
		}
		time.Sleep(150 * time.Millisecond)
	}
}

// noteExplanation renders the DBC's refusal codes.
func noteExplanation(note string) string {
	switch note {
	case "unknown-theme":
		return "that theme is not installed on the DBC"
	case "bad-sound":
		return "the DBC rejected the sound value"
	case "env-error":
		return "the DBC could not write its own U-Boot environment"
	case "":
		return "the DBC has not answered"
	default:
		return note
	}
}

// describeAge renders how stale the published state is.
func describeAge(state map[string]string) string {
	updated, err := strconv.ParseInt(state["updated"], 10, 64)
	if err != nil || updated == 0 {
		return ""
	}
	age := time.Since(time.Unix(updated, 0))
	switch {
	case age < time.Minute:
		return "just now"
	case age < 2*time.Hour:
		return fmt.Sprintf("%d min ago", int(age.Minutes()))
	case age < 48*time.Hour:
		return fmt.Sprintf("%d h ago", int(age.Hours()))
	default:
		return fmt.Sprintf("%d d ago", int(age.Hours()/24))
	}
}

func failBoot(command string, err error) {
	if JSONOutput != nil && *JSONOutput {
		b, _ := json.Marshal(map[string]any{"action": command, "error": err.Error()})
		fmt.Println(string(b))
	}
	fmt.Fprintf(os.Stderr, format.Error("boot %s: %v\n"), command, err)
	os.Exit(1)
}

// reportQueued is the outcome when the DBC is off: the request is kept, and
// saying so plainly is the whole point of not pretending to have applied it.
func reportQueued(command, field, value string) {
	if JSONOutput != nil && *JSONOutput {
		b, _ := json.Marshal(map[string]any{
			"action": command,
			field:    value,
			"queued": true,
			"reboot": "after the DBC next runs",
		})
		fmt.Println(string(b))
		return
	}
	fmt.Printf("%s %s %s queued — the DBC is not running.\n", format.Success("OK:"), command, value)
	fmt.Println("   It applies when the DBC next starts, so it shows on the boot after that.")
}

// stateValue reads one field, treating whitespace-only as absent.
func stateValue(state map[string]string, field string) string {
	return strings.TrimSpace(state[field])
}
