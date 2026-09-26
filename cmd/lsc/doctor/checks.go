package doctor

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

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

// LinkPath is the transport this board's routing table sends peer traffic over.
type LinkPath int

const (
	// LinkUnknown means the command is not running on a board, so the
	// scooter's own inter-board link cannot be judged from here.
	LinkUnknown LinkPath = iota
	LinkUSB
	LinkBackup
	LinkDown
)

// Link is the route the local board has to its peer.
type Link struct {
	// Peer names the other board: "DBC" when running on the MDB and the other
	// way round.
	Peer string
	Path LinkPath
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
	// link reports how this board's routing table reaches its peer.
	link func() (Link, error)
	// now is the clock, so a test can age the last-lock record.
	now func() time.Time
}

type check struct {
	name string
	run  func(env) Result
}

func allChecks() []check {
	return []check{
		{"link", checkLink},
		{"kernel", checkKernel},
		{"services", checkServices},
		{"ota", checkOTA},
		{"faults", checkFaults},
		{"storage", checkStorage},
	}
}

// checkLink reports whether this board has a route to its peer, and which of
// the two physically independent links carries it.
//
// The DBC reaches the MDB's Redis over the USB link (192.168.9.0/24), with a
// PPP-over-UART backup (192.168.8.0/24) that survives the USB link being lost.
// Both sides keep their stable service address (192.168.7.1/.2): the link
// monitor installs a metric-50 route to the peer over USB while a probe answers,
// and the PPP hooks install a metric-200 route over the backup. Reading those
// /32 routes is therefore both the liveness and the transport answer, whereas
// the connected /24 route stays in the table with the peer switched off.
//
// The live route is only meaningful while the DBC is on its own USB. To run lsc
// a person plugs a laptop into the MDB's USB port, which takes the DBC's place
// there, so the live answer during a support session says nothing about normal
// operation. The verdict therefore comes from the transport vehicle-service
// recorded when the DBC was last powered off, and the live route is context.
func checkLink(e env) Result {
	result := Result{Name: "link"}
	if e.link == nil {
		result.Verdict = VerdictUnknown
		result.Detail = "routing table not available"
		return result
	}
	link, err := e.link()
	if err != nil {
		result.Verdict = VerdictUnknown
		result.Detail = "routing table not readable"
		return result
	}

	if last, ok := readLastLink(e); ok {
		return lastLockResult(result, last, link, e.now)
	}

	switch link.Path {
	case LinkUSB:
		result.Verdict = VerdictOK
		result.Detail = link.Peer + " over the USB link"
	case LinkBackup:
		result.Verdict = VerdictWarning
		result.Detail = link.Peer + " over the PPP backup link"
		result.Hint = "The USB link is down. Check the USB cable and the DBC's power."
	case LinkDown:
		result.Verdict = VerdictAttention
		result.Detail = "no route to the " + link.Peer
		result.Hint = "Check that the DBC is powered and its USB cable is seated; the backup link needs ppp-link."
	default:
		result.Verdict = VerdictUnknown
		result.Detail = "not running on a board"
	}
	return result
}

// lastLink is the link state vehicle-service recorded in the system hash when the
// DBC was last powered off.
type lastLink struct {
	transport string // "usb0", "ppp0" or "none"
	usb       string // "up", "down" or "" when not recorded
	ppp       string // "up", "down" or "" when not recorded
	at        int64  // unix seconds, 0 when unknown
}

func readLastLink(e env) (lastLink, bool) {
	if e.state == nil {
		return lastLink{}, false
	}
	system, err := e.state.HGetAll("system")
	if err != nil || system["dbc-link"] == "" {
		return lastLink{}, false
	}
	last := lastLink{
		transport: system["dbc-link"],
		usb:       system["dbc-usb"],
		ppp:       system["dbc-ppp"],
	}
	if at, err := strconv.ParseInt(system["dbc-link-at"], 10, 64); err == nil {
		last.at = at
	}
	return last, true
}

// lastLockResult judges the scooter on the session that just ended, with the
// live route as context. A support session where the laptop holds the USB port,
// or a switched-off DBC, must not read as a fault.
func lastLockResult(result Result, last lastLink, link Link, now func() time.Time) Result {
	var detail string
	switch last.transport {
	case "usb0":
		result.Verdict = VerdictOK
		detail = "DBC over the USB link at last lock"
	case "ppp0":
		result.Verdict = VerdictWarning
		detail = "DBC over the PPP backup link at last lock"
		result.Hint = "The DBC ran on the UART backup, so the USB link was down. Check the USB cable and connector."
	case "none":
		result.Verdict = VerdictAttention
		detail = "no DBC link at last lock"
		result.Hint = "The DBC was unreachable when it was last powered off. Check its power and the USB cable."
	default:
		result.Verdict = VerdictUnknown
		detail = "DBC link unclear at last lock (" + last.transport + ")"
	}

	// The active link working says nothing about the fallback. Surface a PPP
	// route that never came up, so a reader does not assume it is there.
	if last.transport == "usb0" && last.ppp == "down" {
		result.Verdict = VerdictWarning
		result.Hint = "The USB link carried the traffic, but the PPP fallback did not come up: if USB fails, the DBC loses Redis."
	}

	if health := linkHealthText(last); health != "" {
		detail += " (" + health + ")"
	}
	if age := ageText(last.at, now); age != "" {
		detail += ", " + age
	}
	detail += liveContext(last, link)
	result.Detail = detail
	return result
}

// linkHealthText names whether each link had a route at the last lock, so a
// reader can judge whether the fallback was usable at all. An unrecorded link
// is left out rather than guessed at.
func linkHealthText(last lastLink) string {
	var parts []string
	if last.usb != "" {
		parts = append(parts, "USB "+last.usb)
	}
	if last.ppp != "" {
		parts = append(parts, "PPP "+last.ppp)
	}
	return strings.Join(parts, ", ")
}

// liveContext names the current transport when it differs from the recorded
// one, so a reader can tell "the scooter runs on USB" from "it was on USB and is
// on the backup right now".
func liveContext(last lastLink, link Link) string {
	switch link.Path {
	case LinkUSB:
		if last.transport != "usb0" {
			return "; USB now"
		}
	case LinkBackup:
		if last.transport != "ppp0" {
			return "; PPP now"
		}
	case LinkDown:
		return "; no route now"
	}
	return ""
}

// ageText is a coarse age for a unix timestamp, so a stale record is not read as
// a fresh one. An unknown timestamp yields nothing.
func ageText(unix int64, now func() time.Time) string {
	if unix <= 0 || now == nil {
		return ""
	}
	age := now().Sub(time.Unix(unix, 0))
	switch {
	case age < time.Minute:
		return "just now"
	case age < time.Hour:
		return fmt.Sprintf("%dm ago", int(age.Minutes()))
	case age < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(age.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(age.Hours()/24))
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
