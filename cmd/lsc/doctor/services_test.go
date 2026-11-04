package doctor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestInactiveServices(t *testing.T) {
	for _, tc := range []struct {
		name, state, kind, extra string
		stopped                  bool
	}{
		{"stopped", "inactive", "simple", "", true},
		{"running", "active", "simple", "", false},
		{"starting", "activating", "notify", "", false},
		{"oneshot", "inactive", "oneshot", "", false},
		{"socket", "inactive", "simple", "TriggeredBy=test.socket\n", false},
		{"timer", "inactive", "simple", "TriggeredBy=test.timer\n", false},
		{"condition", "inactive", "simple", "ConditionResult=no\n", false},
		{"assert", "inactive", "simple", "AssertResult=no\n", false},
		{"remain", "inactive", "simple", "RemainAfterExit=yes\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			got, err := readInactiveServices(func(name string, args ...string) ([]byte, error) {
				calls++
				if name != "systemctl" {
					t.Fatalf("unexpected command %s", name)
				}
				if calls == 1 {
					return []byte("test.service enabled enabled\ntemplate@.service enabled enabled\n"), nil
				}
				if strings.Contains(strings.Join(args, " "), "template@.") {
					t.Fatal("queried uninstantiated template")
				}
				return []byte(fmt.Sprintf("Id=test.service\nLoadState=loaded\nActiveState=%s\nType=%s\n%s", tc.state, tc.kind, tc.extra)), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if (len(got) == 1) != tc.stopped {
				t.Fatalf("inactive=%v", got)
			}
		})
	}
}

func TestServiceReadFailuresAreUnknown(t *testing.T) {
	for _, broken := range []string{"list", "show", "empty", "incomplete"} {
		t.Run(broken, func(t *testing.T) {
			calls := 0
			_, err := readInactiveServices(func(_ string, _ ...string) ([]byte, error) {
				calls++
				if broken == "list" || (calls == 2 && broken == "show") {
					return nil, errors.New("systemd down")
				}
				if broken == "empty" {
					return nil, nil
				}
				if calls == 1 {
					return []byte("test.service enabled\n"), nil
				}
				return []byte("Id=test.service\n"), nil
			})
			if err == nil {
				t.Fatal("expected unreadable service state")
			}
		})
	}
}

func TestDoctorWithoutRedisStillReturnsStructuredChecks(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	previous, previousJSON := RedisClient, JSONOutput
	defer func() { RedisClient, JSONOutput = previous, previousJSON; DoctorCmd.SetOut(nil) }()
	RedisClient = nil
	jsonOutput := true
	JSONOutput = &jsonOutput
	var output bytes.Buffer
	DoctorCmd.SetOut(&output)
	_ = run(DoctorCmd)
	var report struct {
		Schema int           `json:"schema_version"`
		Checks []VerdictText `json:"checks"`
	}
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Schema != 1 || len(report.Checks) != 6 {
		t.Fatalf("report=%+v", report)
	}
	for _, check := range report.Checks {
		if check.Name == "faults" && check.Verdict != "unknown" {
			t.Fatal(check)
		}
	}
}
