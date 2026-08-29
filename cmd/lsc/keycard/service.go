package keycard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"librescoot/lsc/internal/redis"
)

const (
	keycardUnit        = "librescoot-keycard"
	keycardCommandList = "scooter:keycard"
	keycardHashKey     = "keycard"
	keycardResultField = "command-result"
	keycardTimeout     = 5 * time.Second
)

// serviceRunning reports whether keycard-service is up. When it is, every
// mutation goes through its Redis command interface: the service keeps both
// UID lists in memory and rewrites them on its next mutation, so editing the
// files underneath it loses the edit, and only the service publishes the
// keycard:events the installer and the BLE bridge listen for.
func serviceRunning() bool {
	out, _ := exec.Command("systemctl", "is-active", keycardUnit).Output()
	return strings.TrimSpace(string(out)) == "active"
}

// fallbackNotice warns on stderr that we are editing the UID files directly.
var fallbackWarned bool

func fallbackNotice() {
	if fallbackWarned {
		return
	}
	fallbackWarned = true
	fmt.Fprintf(os.Stderr, "Warning: %s is not running, editing the UID files directly. "+
		"No keycard:events will be published for this change.\n", keycardUnit)
}

// sendKeycardCommand pushes a command onto scooter:keycard and waits for the
// service to answer in the keycard hash.
//
// The result field is deleted before the push so that a reply identical to
// the previous one is still recognised as a reply: the field carries no
// request id, so "it changed" is the only signal available. Commands that
// answer with several writes (list, master:list) cannot be read back this
// way at all, which is why the read paths go to the UID files instead.
func sendKeycardCommand(command string) (string, error) {
	if RedisClient == nil {
		return "", fmt.Errorf("no Redis connection")
	}

	ctx, cancel := context.WithTimeout(context.Background(), keycardTimeout)
	defer cancel()

	if err := RedisClient.HDelWithContext(ctx, keycardHashKey, keycardResultField); err != nil {
		return "", fmt.Errorf("failed to clear previous result: %w", err)
	}

	// Subscribe before pushing so a fast reply cannot be missed.
	pubsub := RedisClient.Subscribe(ctx, keycardHashKey)
	defer pubsub.Close()
	ch := pubsub.Channel()

	if err := RedisClient.LPushWithContext(ctx, keycardCommandList, command); err != nil {
		return "", fmt.Errorf("failed to send command: %w", err)
	}

	// The pub/sub notification carries the field name only, and can be lost
	// if the service coalesces writes, so poll as a backstop.
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		value, err := RedisClient.HGetWithContext(ctx, keycardHashKey, keycardResultField)
		if err == nil && value != "" {
			return value, nil
		}
		if err != nil && !redis.IsNil(err) {
			return "", fmt.Errorf("failed to read command result: %w", err)
		}

		select {
		case <-ctx.Done():
			return "", fmt.Errorf("timed out after %s waiting for %s to answer %q",
				keycardTimeout, keycardUnit, command)
		case <-ticker.C:
		case msg, ok := <-ch:
			if !ok || msg == nil {
				// Subscription gone; the ticker still drives the polling.
				ch = nil
			}
		}
	}
}

// keycardResultError turns a service result code into a human message, or nil
// for "ok". The codes are the service's stable wire vocabulary.
func keycardResultError(result string) error {
	switch {
	case result == "ok":
		return nil
	case result == "error:bad-uid":
		return fmt.Errorf("invalid UID: must be 1-10 bytes of hex")
	case result == "error:already-authorized":
		return fmt.Errorf("card is already authorized")
	case result == "error:already-registered":
		return fmt.Errorf("UID is already registered, as a card or as a master")
	case result == "error:not-found":
		return fmt.Errorf("UID is not registered")
	case result == "error:last-credential":
		return fmt.Errorf("refused: this is the last card that can unlock the vehicle. " +
			"Add the replacement card first, then remove this one")
	case result == "error:save-failed":
		return fmt.Errorf("%s could not write the change to disk", keycardUnit)
	case result == "error:unknown-command":
		return fmt.Errorf("%s does not understand this command; it may be too old", keycardUnit)
	case strings.HasPrefix(result, "error:wrong-mode:"):
		mode := strings.TrimPrefix(result, "error:wrong-mode:")
		return fmt.Errorf("%s is in %s mode and cannot do this right now", keycardUnit, mode)
	case strings.HasPrefix(result, "error:"):
		return fmt.Errorf("%s returned %s", keycardUnit, result)
	default:
		return fmt.Errorf("unexpected reply from %s: %q", keycardUnit, result)
	}
}

// runKeycardCommand sends a command and maps the reply to an error.
func runKeycardCommand(command string) error {
	result, err := sendKeycardCommand(command)
	if err != nil {
		return err
	}
	return keycardResultError(result)
}

// isResult reports whether a command failed with exactly the given code,
// which callers use to treat "already there" or "not there" as a skip rather
// than a failure when acting on several UIDs at once.
func isResult(command, code string) (bool, error) {
	result, err := sendKeycardCommand(command)
	if err != nil {
		return false, err
	}
	if result == code {
		return true, nil
	}
	return false, keycardResultError(result)
}
