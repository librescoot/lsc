package keycard

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestAddUIDToRoleFileRejectsEitherRoleDuplicate(t *testing.T) {
	dir := t.TempDir()
	authorized := filepath.Join(dir, "authorized_uids.txt")
	masters := filepath.Join(dir, "master_uids.txt")
	const uid = "11223344"

	if err := os.WriteFile(masters, []byte(uid+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := addUIDToRoleFile(authorized, masters, uid); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("adding master as authorized error = %v, want already registered", err)
	}

	if err := os.WriteFile(masters, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(authorized, []byte(uid+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := addUIDToRoleFile(masters, authorized, uid); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("adding authorized as master error = %v, want already registered", err)
	}
}

func TestAddUIDToRoleFileSerializesCrossRoleWriters(t *testing.T) {
	for i := 0; i < 25; i++ {
		dir := t.TempDir()
		authorized := filepath.Join(dir, "authorized_uids.txt")
		masters := filepath.Join(dir, "master_uids.txt")
		const uid = "11223344"
		start := make(chan struct{})
		errs := make(chan error, 2)
		var ready sync.WaitGroup
		ready.Add(2)
		for _, paths := range [][2]string{{authorized, masters}, {masters, authorized}} {
			go func(path, other string) {
				ready.Done()
				<-start
				errs <- addUIDToRoleFile(path, other, uid)
			}(paths[0], paths[1])
		}
		ready.Wait()
		close(start)
		first, second := <-errs, <-errs
		if (first == nil) == (second == nil) {
			t.Fatalf("iteration %d errors = %v, %v; want exactly one success", i, first, second)
		}
		failed := first
		if failed == nil {
			failed = second
		}
		if !errors.Is(failed, errUIDAlreadyRegistered) {
			t.Fatalf("iteration %d failure = %v, want duplicate sentinel", i, failed)
		}
	}
}

func TestAddUIDsToFilePropagatesCounterpartReadFailure(t *testing.T) {
	dir := t.TempDir()
	authorized := filepath.Join(dir, "authorized_uids.txt")
	masters := filepath.Join(dir, "master_uids.txt")
	if err := os.Mkdir(authorized, 0o755); err != nil {
		t.Fatal(err)
	}
	added, err := addUIDsToFile(masters, []string{"11223344"})
	if err == nil {
		t.Fatal("addUIDsToFile unexpectedly ignored counterpart read failure")
	}
	if added != 0 {
		t.Fatalf("addUIDsToFile added = %d, want 0", added)
	}
}

func TestImportUIDsPropagatesCounterpartReadFailure(t *testing.T) {
	dir := t.TempDir()
	authorized := filepath.Join(dir, "authorized_uids.txt")
	masters := filepath.Join(dir, "master_uids.txt")
	if err := os.Mkdir(masters, 0o755); err != nil {
		t.Fatal(err)
	}
	imported, conflicts, err := importUIDs(false, []string{"11223344"}, "authorized:add:", authorized)
	if err == nil {
		t.Fatal("importUIDs unexpectedly ignored counterpart read failure")
	}
	if imported != 0 || conflicts != 0 {
		t.Fatalf("importUIDs counts = %d imported, %d conflicts; want zeroes", imported, conflicts)
	}
}

func TestAddUIDToRoleFileWritesNewUID(t *testing.T) {
	dir := t.TempDir()
	authorized := filepath.Join(dir, "authorized_uids.txt")
	masters := filepath.Join(dir, "master_uids.txt")

	if err := addUIDToRoleFile(authorized, masters, "11223344"); err != nil {
		t.Fatal(err)
	}
	uids, err := readKeycardFile(authorized)
	if err != nil {
		t.Fatal(err)
	}
	if len(uids) != 1 || uids[0] != "11223344" {
		t.Fatalf("authorized UIDs = %v, want [11223344]", uids)
	}
}
