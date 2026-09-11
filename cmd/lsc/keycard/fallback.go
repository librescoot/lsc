package keycard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// This file holds the direct file-editing paths used when keycard-service is
// not running. They exist so that lsc still works on a vehicle where the
// service is stopped or absent; with the service up, every mutation goes
// through its Redis command interface instead (see service.go).

func authorizedFilePath() string {
	p, _ := getKeycardPaths()
	return p
}

func masterFilePath() string {
	_, p := getKeycardPaths()
	return p
}

var errUIDAlreadyRegistered = errors.New("UID is already registered")

// addUIDToFile appends uid unless either role already lists it.
func addUIDToFile(path, uid string) error {
	authorizedPath, masterPath := getKeycardPaths()
	otherPath := authorizedPath
	if filepath.Base(path) == filepath.Base(authorizedPath) {
		otherPath = filepath.Join(filepath.Dir(path), filepath.Base(masterPath))
	} else if filepath.Base(path) == filepath.Base(masterPath) {
		otherPath = filepath.Join(filepath.Dir(path), filepath.Base(authorizedPath))
	}
	return addUIDToRoleFile(path, otherPath, uid)
}

func addUIDsToFile(path string, uids []string) (int, error) {
	added := 0
	for _, uid := range uids {
		if err := addUIDToFile(path, uid); err != nil {
			if errors.Is(err, errUIDAlreadyRegistered) {
				continue
			}
			return added, err
		}
		added++
	}
	return added, nil
}

func addUIDToRoleFile(path, otherPath, uid string) error {
	return withKeycardFileLock(path, func() error {
		uids, err := readKeycardFile(path)
		if err != nil {
			return err
		}
		otherUIDs, err := readKeycardFile(otherPath)
		if err != nil {
			return err
		}
		for _, existing := range append(uids, otherUIDs...) {
			if existing == uid {
				return errUIDAlreadyRegistered
			}
		}
		return writeKeycardFile(path, removeDuplicates(append(uids, uid)))
	})
}

func withKeycardFileLock(path string, fn func() error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(filepath.Dir(path), ".lsc.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX); err != nil {
		return err
	}
	defer unix.Flock(int(lock.Fd()), unix.LOCK_UN)
	return fn()
}

// removeUIDFromFile drops uid from the file.
//
// lastCredential keeps the service's anti-lockout invariant alive while the
// service is down: with nothing else enforcing it, removing the only card
// that can unlock would strand the vehicle on the next start.
func removeUIDFromFile(path, uid string, lastCredential bool) error {
	return withKeycardFileLock(path, func() error {
		uids, err := readKeycardFile(path)
		if err != nil {
			return err
		}

		remaining := make([]string, 0, len(uids))
		found := false
		for _, existing := range uids {
			if existing == uid {
				found = true
				continue
			}
			remaining = append(remaining, existing)
		}
		if !found {
			return fmt.Errorf("UID is not registered")
		}
		if lastCredential && len(remaining) == 0 {
			return fmt.Errorf("refused: this is the last card that can unlock the vehicle. " +
				"Add the replacement card first, then remove this one")
		}

		return writeKeycardFile(path, remaining)
	})
}
