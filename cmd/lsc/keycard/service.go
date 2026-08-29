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
	keycardErrorField  = "command-error"
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

// keycardAnswer is one command's reply. Result is the prose the service has
// always written; Code is its machine-readable form, empty on success and also
// empty against a keycard-service too old to write it.
type keycardAnswer struct {
	Result string
	Code   string
}

// failed reports whether the answer is an error, by either vocabulary, so an
// older service that only writes prose is still read correctly.
func (a keycardAnswer) failed() bool {
	return a.Code != "" || strings.HasPrefix(a.Result, "error:")
}

// legacyCodes recovers a code from the wording a keycard-service too old to
// write command-error would have used. Only the outcomes lsc acts on are
// listed; anything else falls through to the prose.
var legacyCodes = map[string]string{
	"error:empty uid":                          "empty-uid",
	"error:already authorized":                 "already-authorized",
	"error:not found":                          "not-found",
	"error:cannot remove last authorized card": "last-credential",
	"error:unknown command":                    "unknown-command",
}

// code is the answer's machine-readable form, inferred from the prose when the
// service did not supply one.
func (a keycardAnswer) code() string {
	if a.Code != "" {
		return a.Code
	}
	return legacyCodes[a.Result]
}

// sendKeycardCommand pushes a command onto scooter:keycard and waits for the
// service to answer in the keycard hash.
//
// Both answer fields are deleted before the push so that a reply identical to
// the previous one is still recognised as a reply: they carry no request id,
// so "it changed" is the only signal available. Deleting command-error also
// keeps a code from an older command being read as this one's, which is what
// an older service leaving the field alone would otherwise look like.
//
// Commands that answer with several writes (list, master:list) cannot be read
// back this way at all, which is why the read paths go to the UID files.
func sendKeycardCommand(command string) (keycardAnswer, error) {
	if RedisClient == nil {
		return keycardAnswer{}, fmt.Errorf("no Redis connection")
	}

	ctx, cancel := context.WithTimeout(context.Background(), keycardTimeout)
	defer cancel()

	if err := RedisClient.HDelWithContext(ctx, keycardHashKey,
		keycardResultField, keycardErrorField); err != nil {
		return keycardAnswer{}, fmt.Errorf("failed to clear previous result: %w", err)
	}

	// Subscribe before pushing so a fast reply cannot be missed.
	pubsub := RedisClient.Subscribe(ctx, keycardHashKey)
	defer pubsub.Close()
	ch := pubsub.Channel()

	if err := RedisClient.LPushWithContext(ctx, keycardCommandList, command); err != nil {
		return keycardAnswer{}, fmt.Errorf("failed to send command: %w", err)
	}

	// The pub/sub notification carries the field name only, and can be lost
	// if the service coalesces writes, so poll as a backstop.
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		value, err := RedisClient.HGetWithContext(ctx, keycardHashKey, keycardResultField)
		if err == nil && value != "" {
			// Written in the same operation as the result, so it is already
			// there. Absent means a keycard-service too old to write it.
			code, codeErr := RedisClient.HGetWithContext(ctx, keycardHashKey, keycardErrorField)
			if codeErr != nil {
				code = ""
			}
			return keycardAnswer{Result: value, Code: code}, nil
		}
		if err != nil && !redis.IsNil(err) {
			return keycardAnswer{}, fmt.Errorf("failed to read command result: %w", err)
		}

		select {
		case <-ctx.Done():
			return keycardAnswer{}, fmt.Errorf("timed out after %s waiting for %s to answer %q",
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

// answerError turns an answer into a human message, or nil for success.
//
// The code is preferred where the service supplies one. Where it does not, the
// prose is passed through as it stands: an older service's wording is already
// a readable sentence, and inventing a mapping from it would only guess.
func answerError(a keycardAnswer) error {
	if !a.failed() {
		return nil
	}

	switch code := a.code(); {
	case code == "empty-uid":
		return fmt.Errorf("no UID given")
	case code == "bad-uid":
		return fmt.Errorf("invalid UID: must be 1-10 bytes of hex")
	case code == "already-authorized":
		return fmt.Errorf("card is already authorized")
	case code == "already-registered":
		return fmt.Errorf("UID is already registered, as a card or as a master")
	case code == "not-found":
		return fmt.Errorf("UID is not registered")
	case code == "last-credential":
		return fmt.Errorf("refused: this is the last card that can unlock the vehicle. " +
			"Add the replacement card first, then remove this one")
	case code == "save-failed":
		return fmt.Errorf("%s could not write the change to disk", keycardUnit)
	case code == "unknown-command":
		return fmt.Errorf("%s does not understand this command; it may be too old", keycardUnit)
	case strings.HasPrefix(code, "wrong-mode:"):
		mode := strings.TrimPrefix(code, "wrong-mode:")
		return fmt.Errorf("%s is in %s mode and cannot do this right now", keycardUnit, mode)
	}

	return fmt.Errorf("%s returned %s", keycardUnit, a.Result)
}

// runKeycardCommand sends a command and maps the reply to an error.
func runKeycardCommand(command string) error {
	answer, err := sendKeycardCommand(command)
	if err != nil {
		return err
	}
	return answerError(answer)
}

// isResult reports whether a command failed with one of the given codes,
// which callers use to treat "already there" or "not there" as a skip rather
// than a failure when acting on several UIDs at once.
//
// Several codes, because one outcome can have more than one name: add: on a
// UID that is already registered answers already-authorized for a card and
// already-registered for a master, and an import skips both alike.
func isResult(command string, codes ...string) (bool, error) {
	answer, err := sendKeycardCommand(command)
	if err != nil {
		return false, err
	}
	actual := answer.code()
	for _, code := range codes {
		if actual == code {
			return true, nil
		}
	}
	return false, answerError(answer)
}
