package boot

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func installFakeFwSetenv(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fw_setenv")
	script := "#!/bin/sh\nset -eu\n" + body
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake fw_setenv: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestFwSetenvBatchUsesCompatibleScript(t *testing.T) {
	captureDir := t.TempDir()
	appliedDir := t.TempDir()
	capturedPath := filepath.Join(captureDir, "path")
	capturedMode := filepath.Join(captureDir, "mode")
	capturedScript := filepath.Join(captureDir, "script")
	t.Setenv("APPLIED_DIR", appliedDir)
	t.Setenv("CAPTURED_PATH", capturedPath)
	t.Setenv("CAPTURED_MODE", capturedMode)
	t.Setenv("CAPTURED_SCRIPT", capturedScript)

	// Model both on-vehicle constraints: `-s -` silently does nothing, and
	// lines without `=` are silently ignored even when given via a real path.
	installFakeFwSetenv(t, `
[ "$1" = "-s" ]
if [ "$2" = "-" ]; then
	exit 0
fi
[ -f "$2" ]
printf '%s' "$2" > "$CAPTURED_PATH"
stat -c '%a' "$2" > "$CAPTURED_MODE"
cp "$2" "$CAPTURED_SCRIPT"
while IFS= read -r line; do
	case "$line" in
		*=*)
			key=${line%%=*}
			value=${line#*=}
			case "$key" in
				bootcount|mender_boot_part|mender_boot_part_hex|upgrade_available)
					printf '%s' "$value" > "$APPLIED_DIR/$key"
					;;
			esac
		;;
	esac
done < "$2"
`)

	vars := map[string]string{
		"upgrade_available":    "1",
		"mender_boot_part":     "3",
		"mender_boot_part_hex": "3",
		"bootcount":            "0",
	}
	if err := fwSetenvBatch(vars); err != nil {
		t.Fatalf("fwSetenvBatch: %v", err)
	}

	pathBytes, err := os.ReadFile(capturedPath)
	if err != nil {
		t.Fatalf("fake fw_setenv did not receive a script path: %v", err)
	}
	tempPath := string(pathBytes)
	if tempPath == "-" {
		t.Fatal("fw_setenv received stdin dash instead of a script path")
	}
	if _, err := os.Stat(tempPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary script was not removed: %v", err)
	}

	mode, err := os.ReadFile(capturedMode)
	if err != nil {
		t.Fatalf("read captured mode: %v", err)
	}
	if got := strings.TrimSpace(string(mode)); got != "600" {
		t.Fatalf("temporary script mode = %q, want 600", got)
	}

	script, err := os.ReadFile(capturedScript)
	if err != nil {
		t.Fatalf("read captured script: %v", err)
	}
	wantScript := "bootcount=0\nmender_boot_part=3\nmender_boot_part_hex=3\nupgrade_available=1\n"
	if got := string(script); got != wantScript {
		t.Fatalf("script contents = %q, want %q", got, wantScript)
	}

	for key, want := range vars {
		value, err := os.ReadFile(filepath.Join(appliedDir, key))
		if err != nil {
			t.Errorf("%s was not applied: %v", key, err)
			continue
		}
		if got := string(value); got != want {
			t.Errorf("applied %s = %q, want %q", key, got, want)
		}
	}
}

func TestFwSetenvBatchRemovesScriptAfterCommandFailure(t *testing.T) {
	capturedPath := filepath.Join(t.TempDir(), "path")
	t.Setenv("CAPTURED_PATH", capturedPath)
	installFakeFwSetenv(t, `
printf '%s' "$2" > "$CAPTURED_PATH"
echo 'simulated write failure' >&2
exit 42
`)

	err := fwSetenvBatch(map[string]string{"bootcount": "0"})
	if err == nil {
		t.Fatal("fwSetenvBatch succeeded, want command error")
	}
	if !strings.Contains(err.Error(), "simulated write failure") {
		t.Fatalf("error does not include command output: %v", err)
	}

	pathBytes, readErr := os.ReadFile(capturedPath)
	if readErr != nil {
		t.Fatalf("read captured path: %v", readErr)
	}
	if _, statErr := os.Stat(string(pathBytes)); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("temporary script was not removed after failure: %v", statErr)
	}
}

func TestFwSetenvBatchPropagatesRemoveError(t *testing.T) {
	capturedPath := filepath.Join(t.TempDir(), "path")
	t.Setenv("CAPTURED_PATH", capturedPath)
	installFakeFwSetenv(t, `printf '%s' "$2" > "$CAPTURED_PATH"`)

	removeErr := errors.New("simulated remove failure")
	originalRemove := removeFwSetenvScript
	removeFwSetenvScript = func(string) error { return removeErr }
	t.Cleanup(func() { removeFwSetenvScript = originalRemove })

	err := fwSetenvBatch(map[string]string{"bootcount": "0"})
	if !errors.Is(err, removeErr) {
		t.Fatalf("fwSetenvBatch error = %v, want remove error", err)
	}

	// The injected failure deliberately leaves the file behind; clean it up
	// with the real remover so the test itself does not leak a stale file.
	pathBytes, readErr := os.ReadFile(capturedPath)
	if readErr != nil {
		t.Fatalf("read captured path: %v", readErr)
	}
	if removeErr := originalRemove(string(pathBytes)); removeErr != nil {
		t.Fatalf("test cleanup: %v", removeErr)
	}
}
