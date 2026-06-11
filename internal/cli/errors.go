package cli

import "errors"

// ErrSilent signals a command failure whose message has already been printed
// (text or JSON). Execute() exits non-zero without printing it again.
var ErrSilent = errors.New("command failed")
