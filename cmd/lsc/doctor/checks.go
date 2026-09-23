package doctor

import (
	"fmt"
	"sort"
	"strings"

	"librescoot/lsc/internal/format"
)

// Verdict is how much attention a check wants.
type Verdict int

const (
	VerdictOK Verdict = iota
	// VerdictWarning is worth reading but is often a normal intermediate state,
	// such as an update waiting for its reboot.
	VerdictWarning
	// VerdictAttention is a state someone should fix.
	VerdictAttention
	// VerdictUnknown means the check could not be answered, which is not itself
	// a fault: an absent DBC or an older writer looks the same as a healthy one
	// to a reader that has nothing to compare.
	VerdictUnknown
)

func (v Verdict) String() string {
	switch v {
	case VerdictOK:
		return "ok"
	case VerdictWarning:
		return "warning"
	case VerdictAttention:
		return "attention"
	}
	return "unknown"
}

// Result is one check's outcome.
type Result struct {
	Name    string  `json:"name"`
	Verdict Verdict `json:"-"`
	Detail  string  `json:"detail,omitempty"`
	// Hint says what to do about it, in one line.
	Hint string `json:"hint,omitempty"`
}

type VerdictText struct {
	Name    string `json:"name"`
	Verdict string `json:"verdict"`
	Detail  string `json:"detail,omitempty"`
	Hint    string `json:"hint,omitempty"`
}

// stateReader is the part of the Redis client the checks use, so a test can drive
// them without a server.
type stateReader interface {
	HGetAll(key string) (map[string]string, error)
	SMembers(key string) ([]string, error)
}

// env is everything a check may touch.
type env struct {
	state stateReader
	// failedUnits lists systemd units in the failed state.
	failedUnits func() []string
	// freeBytes is the space left on a mounted filesystem.
	freeBytes func(path string) (free, total uint64, err error)
}

type check struct {
	name string
	run  func(env) Result
}

func allChecks() []check {
	return []check{
		{"kernel", checkKernel},
		{"services", checkServices},
		{"ota", checkOTA},
		{"faults", checkFaults},
		{"storage", checkStorage},
	}
}

// checkKernel compares what each board reports about its own kernel: the running
// release, the module tree that belongs to it, and the boot image the slot ships.
// version-service publishes those fields; a board that does not report them yet
// is unknown rather than broken.
func checkKernel(e env) Result {
	result := Result{Name: "kernel"}
	var problems []string
	unknown := 0
	boards := 0

	for _, board := range []string{"mdb", "dbc"} {
		version, err := e.state.HGetAll("version:" + board)
		if err != nil || version["id"] == "" {
			continue
		}
		boards++
		switch version["kernel_check"] {
		case "ok":
		case "skew":
			detail := version["kernel_check_detail"]
			if detail == "" {
				detail = "the running kernel, /lib/modules and /boot/zImage disagree"
			}
			problems = append(problems, fmt.Sprintf("%s: %s", board, detail))
		default:
			unknown++
		}
	}

	switch {
	case len(problems) > 0:
		result.Verdict = VerdictAttention
		result.Detail = strings.Join(problems, "; ")
		result.Hint = "Drivers that are not built in cannot load. Reinstall the image, or restore /boot."
	case boards == 0:
		result.Verdict = VerdictUnknown
		result.Detail = "no board reports its kernel"
	case unknown > 0:
		result.Verdict = VerdictUnknown
		result.Detail = fmt.Sprintf("%d of %d boards do not report their kernel", unknown, boards)
	default:
		result.Verdict = VerdictOK
		result.Detail = "kernel, modules and /boot/zImage agree"
	}
	return result
}

// checkServices reports systemd units in the failed state. Units that are enabled
// but idle are not a fault: the image ships oneshots, templates and
// connectivity-gated units that are meant to be inactive.
func checkServices(e env) Result {
	result := Result{Name: "services"}
	if e.failedUnits == nil {
		result.Verdict = VerdictUnknown
		result.Detail = "systemd not available"
		return result
	}
	failed := e.failedUnits()
	if len(failed) == 0 {
		result.Verdict = VerdictOK
		result.Detail = "no failed units"
		return result
	}
	sort.Strings(failed)
	result.Verdict = VerdictAttention
	result.Detail = fmt.Sprintf("%s failed", strings.Join(failed, ", "))
	result.Hint = "Use `lsc service logs <unit>` to see why it failed, or `lsc service restart <unit>` to restart it."
	return result
}

// checkOTA reports an update that failed, or one that is installed and waiting
// for the reboot that activates it.
func checkOTA(e env) Result {
	result := Result{Name: "ota"}
	ota, err := e.state.HGetAll("ota")
	if err != nil || len(ota) == 0 {
		result.Verdict = VerdictUnknown
		result.Detail = "no update state"
		return result
	}

	var failures, waiting, installing []string
	for _, component := range []string{"mdb", "dbc"} {
		if message := ota["error-message:"+component]; message != "" {
			failures = append(failures, fmt.Sprintf("%s: %s", component, message))
			continue
		}
		if ota["error:"+component] != "" {
			failures = append(failures, fmt.Sprintf("%s: %s", component, ota["error:"+component]))
			continue
		}
		status := ota["status:"+component]
		switch {
		case strings.Contains(status, "fail"), strings.Contains(status, "error"):
			failures = append(failures, fmt.Sprintf("%s: %s", component, status))
		case status == "pending-reboot":
			waiting = append(waiting, component)
		case status == "installing":
			installing = append(installing, component)
		}
	}

	switch {
	case len(failures) > 0:
		result.Verdict = VerdictAttention
		result.Detail = strings.Join(failures, "; ")
		result.Hint = "Use `lsc ota status` to see the failed install."
	case len(installing) > 0:
		result.Verdict = VerdictWarning
		result.Detail = fmt.Sprintf("%s installing", strings.Join(installing, ", "))
		result.Hint = "Leave the scooter powered until it finishes."
	case len(waiting) > 0:
		result.Verdict = VerdictWarning
		result.Detail = fmt.Sprintf("%s installed, waiting for the reboot that activates it", strings.Join(waiting, ", "))
	default:
		result.Verdict = VerdictOK
		result.Detail = "no update in flight"
	}
	return result
}

// checkFaults reports the active fault sets the vehicle, ECU and batteries keep.
func checkFaults(e env) Result {
	result := Result{Name: "faults"}
	sources := []struct {
		name   string
		key    string
		series format.FaultSeries
	}{
		{"vehicle", "vehicle:fault", format.SeriesVehicle},
		{"ecu", "engine-ecu:fault", format.SeriesMotor},
		{"battery 0", "battery:0:fault", format.SeriesBattery},
		{"battery 1", "battery:1:fault", format.SeriesBattery},
	}

	var reported []string
	total := 0
	readable := false
	for _, source := range sources {
		faults, err := e.state.SMembers(source.key)
		if err != nil {
			continue
		}
		readable = true
		if len(faults) == 0 {
			continue
		}
		sort.Strings(faults)
		labelled := make([]string, 0, len(faults))
		for _, fault := range faults {
			labelled = append(labelled, format.FaultLabel(source.series, fault))
		}
		total += len(faults)
		reported = append(reported, fmt.Sprintf("%s: %s", source.name, strings.Join(labelled, ", ")))
	}

	switch {
	case !readable:
		result.Verdict = VerdictUnknown
		result.Detail = "no fault state"
	case total == 0:
		result.Verdict = VerdictOK
		result.Detail = "none active"
	default:
		result.Verdict = VerdictAttention
		result.Detail = strings.Join(reported, "; ")
		result.Hint = "Use `lsc diag faults` to see detail, or `lsc logs` to save logs."
	}
	return result
}

// Storage thresholds. The rootfs is a fixed-size A/B slot that the image fills, so
// only the writable /data partition is judged; elsewhere a near-full filesystem is
// what the image looks like.
const (
	storageAttentionBytes = 100 << 20
	storageWarningBytes   = 500 << 20
)

func checkStorage(e env) Result {
	result := Result{Name: "storage"}
	if e.freeBytes == nil {
		result.Verdict = VerdictUnknown
		result.Detail = "free space not available"
		return result
	}
	free, total, err := e.freeBytes("/data")
	if err != nil {
		result.Verdict = VerdictUnknown
		result.Detail = "/data not readable"
		return result
	}
	detail := fmt.Sprintf("/data %s free of %s", humanBytes(free), humanBytes(total))
	switch {
	case free < storageAttentionBytes:
		result.Verdict = VerdictAttention
		result.Detail = detail
		result.Hint = "Map and routing tiles live in /data. Remove the ones you no longer need."
	case free < storageWarningBytes:
		result.Verdict = VerdictWarning
		result.Detail = detail
		result.Hint = "Map and routing tiles live in /data."
	default:
		result.Verdict = VerdictOK
		result.Detail = detail
	}
	return result
}

func humanBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	value := float64(bytes)
	for _, suffix := range []string{"KiB", "MiB", "GiB", "TiB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, suffix)
		}
	}
	return fmt.Sprintf("%.1f PiB", value/unit)
}
