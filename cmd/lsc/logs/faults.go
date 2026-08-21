package logs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"librescoot/lsc/internal/redis"
)

const (
	// faultStreamKey is a Redis stream, not a hash. It is written with XADD by
	// the fault reporters in redis-ipc and by battery-service, so it has to be
	// read with a stream command; HGETALL against it returns nothing useful.
	faultStreamKey = "events:faults"

	// faultEventsFilename lives next to the hash snapshots in the bundle's
	// redis directory. The .log suffix marks it as line-oriented text rather
	// than one of the JSON hash dumps.
	faultEventsFilename = "events-faults.log"

	// faultEventLimit matches the writers' MAXLEN ~ 1000, so a full read of a
	// capped stream fits in one request.
	faultEventLimit = 1000

	faultKindRaise   = "RAISE"
	faultKindClear   = "CLEAR"
	faultKindUnknown = "?"

	faultEventsHeader = "# " + faultStreamKey + " (Redis stream, oldest first)\n" +
		"# stream-id  time-utc  event  group  code  description\n"
	faultEventsEmptyNote = "# no fault events recorded\n"
)

// faultEvent is one decoded entry of the events:faults stream.
type faultEvent struct {
	ID          string
	Time        time.Time
	Kind        string
	Group       string
	Code        string
	Description string
}

// captureFaultEvents reads the fault stream and writes it to
// {redisDir}/events-faults.log, returning the number of entries captured.
//
// The file is always written, even with no entries, so that a bundle from a
// scooter that raised no faults is distinguishable from a bundle where the
// capture failed.
func captureFaultEvents(redisDir string) (int, error) {
	msgs, err := RedisClient.XRevRangeN(context.Background(), faultStreamKey, "+", "-", faultEventLimit)
	if err != nil && !redis.IsNil(err) {
		return 0, err
	}

	return writeFaultEvents(redisDir, parseFaultEvents(msgs))
}

func writeFaultEvents(redisDir string, events []faultEvent) (int, error) {
	path := filepath.Join(redisDir, faultEventsFilename)
	if err := os.WriteFile(path, renderFaultEvents(events), 0644); err != nil {
		return 0, err
	}
	return len(events), nil
}

// parseFaultEvents decodes XREVRANGE output (newest first) into oldest-first
// fault events. Entry IDs carry a millisecond timestamp in their first
// component, which is the only time source the stream has.
func parseFaultEvents(msgs []redis.XMessage) []faultEvent {
	events := make([]faultEvent, 0, len(msgs))
	for i := len(msgs) - 1; i >= 0; i-- {
		msg := msgs[i]
		kind, code := classifyFaultCode(faultField(msg.Values, "code"))
		events = append(events, faultEvent{
			ID:          msg.ID,
			Time:        faultEventTime(msg.ID),
			Kind:        kind,
			Group:       faultField(msg.Values, "group"),
			Code:        code,
			Description: faultField(msg.Values, "description"),
		})
	}
	return events
}

func renderFaultEvents(events []faultEvent) []byte {
	var b strings.Builder
	b.WriteString(faultEventsHeader)
	if len(events) == 0 {
		b.WriteString(faultEventsEmptyNote)
		return []byte(b.String())
	}

	for _, ev := range events {
		ts := "-"
		if !ev.Time.IsZero() {
			ts = ev.Time.UTC().Format("2006-01-02T15:04:05.000Z07:00")
		}
		fmt.Fprintf(&b, "%s  %s  %s  %s  %s",
			orDash(ev.ID), ts, ev.Kind, orDash(ev.Group), orDash(ev.Code))
		if ev.Description != "" {
			b.WriteString("  ")
			b.WriteString(ev.Description)
		}
		b.WriteString("\n")
	}
	return []byte(b.String())
}

// classifyFaultCode splits a stream code field into an event kind and the bare
// code. A clear is written as the raised code with a leading minus, so the
// sign is checked on the raw string: parsing first would fold "-0" into "0"
// and turn a clear of code 0 into a raise.
func classifyFaultCode(raw string) (string, string) {
	code := strings.TrimSpace(raw)
	if code == "" {
		return faultKindUnknown, ""
	}
	if strings.HasPrefix(code, "-") {
		return faultKindClear, code[1:]
	}
	return faultKindRaise, code
}

func faultEventTime(id string) time.Time {
	ms, err := strconv.ParseInt(strings.SplitN(id, "-", 2)[0], 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

// faultField reads one field of a stream entry, flattening whitespace so a
// value containing a newline cannot break the line-per-event layout.
func faultField(values map[string]interface{}, key string) string {
	val, ok := values[key]
	if !ok {
		return ""
	}
	s := fmt.Sprintf("%v", val)
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(s)
	return strings.TrimSpace(s)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
