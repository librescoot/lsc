package keycard

import "fmt"

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

// addUIDToFile appends uid unless the file already lists it.
func addUIDToFile(path, uid string) error {
	uids, err := readKeycardFile(path)
	if err != nil {
		return err
	}
	for _, existing := range uids {
		if existing == uid {
			return fmt.Errorf("UID is already registered")
		}
	}
	return writeKeycardFile(path, removeDuplicates(append(uids, uid)))
}

// removeUIDFromFile drops uid from the file.
//
// lastCredential keeps the service's anti-lockout invariant alive while the
// service is down: with nothing else enforcing it, removing the only card
// that can unlock would strand the vehicle on the next start.
func removeUIDFromFile(path, uid string, lastCredential bool) error {
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
}
