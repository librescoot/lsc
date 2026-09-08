package lsc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/librescoot/event-service/api"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"
)

func runExt(t *testing.T, backend extensionAPI, asJSON bool, args ...string) (string, string, error) {
	t.Helper()
	old := JSONOutput
	JSONOutput = asJSON
	defer func() { JSONOutput = old }()
	cmd := newExtCommand(backend)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	var out, stderr bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), stderr.String(), err
}
func TestExtValidationBeforeRPC(t *testing.T) {
	cases := [][]string{
		{"add"}, {"add", "x", "--do", "redis", "--list", "x", "--push", "y"},
		{"add", "x", "--on", "input.*", "--do", "redis", "--list", "x"},
		{"add", "x", "--on", "input.*", "--do", "redis", "--list", "x", "--push", "y", "--command", "true"},
		{"add", "x", "--on", "bad*", "--do", "redis", "--list", "x", "--push", "y"},
		{"add", "x", "--on", "*", "--do", "exec", "--command", "true", "--timeout", "0s"},
		{"add", "x", "--on", "*", "--do", "can", "--iface", "can0", "--id", "20000000"},
		{"add", "x", "--on", "*", "--do", "can", "--iface", "can0", "--id", "123", "--dlc", "2"},
		{"add", "x", "--on", "*", "--do", "can", "--iface", "can0", "--id", "123", "--rtr", "--data", "01"},
		{"enable"}, {"disable", "x", "y"}, {"show", ""}, {"show", "../rule"}, {"enable", "bad\nname"}, {"show", strings.Repeat("x", 129)}, {"test", "x", "--event", "null"},
		{"test", "x", "--event", `{"topic":"x"} {}`}, {"status", "extra"}, {"list", "extra"},
		{"tail", "ev:*"}, {"tail", "x?"}, {"tail", "[ab]"}, {"tail", "x*y"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, _, err := runExt(t, extensionAPI{}, false, args...)
			if err == nil {
				t.Fatal("expected validation error before nil API boundary")
			}
		})
	}
}
func TestExtAddEscapingAndRevision(t *testing.T) {
	var called int
	name := "quote\" café"
	push := "hello\"\\\n\t\x01é"
	backend := extensionAPI{
		list: func(_ context.Context, r api.ListRequest) (api.ListResponse, error) {
			if r.Limit != 100 {
				t.Fatalf("limit=%d", r.Limit)
			}
			return api.ListResponse{Revision: "rev1"}, nil
		},
		add: func(_ context.Context, r api.AddRequest) (api.MutationResponse, error) {
			called++
			if r.ExpectedRevision != "rev1" {
				t.Fatal(r)
			}
			var parsed struct {
				Rule []struct {
					Name string
					On   []string
					Step []struct{ Do, List, Push string }
				}
			}
			md, err := toml.Decode(r.Definition, &parsed)
			if err != nil || len(md.Undecoded()) != 0 {
				t.Fatalf("TOML: %s %v", r.Definition, err)
			}
			if len(parsed.Rule) != 1 || parsed.Rule[0].Name != name || len(parsed.Rule[0].On) != 3 || parsed.Rule[0].Step[0].Push != push {
				t.Fatalf("bad roundtrip: %#v", parsed)
			}
			return api.MutationResponse{Name: name, PendingRestart: true, Revision: "rev2"}, nil
		},
	}
	out, stderr, err := runExt(t, backend, false, "add", name, "--on", "input.a,input.b", "--on", "vehicle.*", "--do", "redis", "--list", "test:list", "--push", push)
	if err != nil || called != 1 || !strings.Contains(out, "Pending restart: true") || !strings.Contains(stderr, "current running rules are unchanged") {
		t.Fatalf("%s %s %v called=%d", out, stderr, err, called)
	}
}
func TestExtEnableDisableJSONAndNoRetry(t *testing.T) {
	for _, name := range []string{"enable", "disable"} {
		t.Run(name, func(t *testing.T) {
			calls := 0
			backend := extensionAPI{show: func(context.Context, api.ShowRequest) (api.ShowResponse, error) {
				return api.ShowResponse{Revision: "r"}, nil
			}, set: func(_ context.Context, r api.SetEnabledRequest) (api.MutationResponse, error) {
				calls++
				if r.Enabled != (name == "enable") || r.ExpectedRevision != "r" || r.Name != "demo" {
					t.Fatal(r)
				}
				return api.MutationResponse{Name: r.Name, PendingRestart: true}, nil
			}}
			out, stderr, err := runExt(t, backend, true, name, "demo")
			var result api.MutationResponse
			if err != nil || json.Unmarshal([]byte(out), &result) != nil || !result.PendingRestart || !strings.Contains(stderr, "No restart was performed") || calls != 1 {
				t.Fatalf("%s %s %v", out, stderr, err)
			}
			backend.set = func(context.Context, api.SetEnabledRequest) (api.MutationResponse, error) {
				calls++
				return api.MutationResponse{}, errors.New("revision conflict")
			}
			_, _, err = runExt(t, backend, false, name, "demo")
			if err == nil || calls != 2 {
				t.Fatalf("mutation retried or error lost: %v calls=%d", err, calls)
			}
		})
	}
}
func TestExtListPagination(t *testing.T) {
	calls := 0
	backend := extensionAPI{list: func(_ context.Context, r api.ListRequest) (api.ListResponse, error) {
		calls++
		if r.Limit != 100 {
			t.Fatal(r)
		}
		count := 100
		if r.Offset == 100 {
			count = 1
		} else if r.Offset != 0 {
			t.Fatal(r)
		}
		return api.ListResponse{Rules: make([]api.RuleSummary, count), Total: 101, Revision: "r", AppliedRevision: "a", Diagnostics: []string{"broken rule"}}, nil
	}}
	out, _, err := runExt(t, backend, true, "list")
	var listed api.ListResponse
	if err != nil || json.Unmarshal([]byte(out), &listed) != nil || len(listed.Rules) != 101 || len(listed.Diagnostics) != 1 || calls != 2 {
		t.Fatalf("list: %v %s", err, out)
	}
	for _, mode := range []string{"revision", "empty", "total"} {
		t.Run(mode, func(t *testing.T) {
			backend.list = func(_ context.Context, r api.ListRequest) (api.ListResponse, error) {
				p := api.ListResponse{Rules: make([]api.RuleSummary, 1), Total: 2, Revision: "r"}
				if r.Offset > 0 {
					switch mode {
					case "revision":
						p.Revision = "changed"
					case "empty":
						p.Rules = nil
					case "total":
						p.Total = 3
					}
				}
				return p, nil
			}
			_, _, err := runExt(t, backend, false, "list")
			if err == nil {
				t.Fatal("inconsistent pagination accepted")
			}
		})
	}
}
func TestExtHumanAndJSONReadCommands(t *testing.T) {
	backend := extensionAPI{
		list: func(context.Context, api.ListRequest) (api.ListResponse, error) {
			return api.ListResponse{Total: 1, Rules: []api.RuleSummary{{Name: "demo", Loaded: true, Enabled: true, LastFire: 123, Errors: 4, Diagnostics: []string{"rule diagnostic"}}}, Diagnostics: []string{"file diagnostic"}}, nil
		},
		show: func(context.Context, api.ShowRequest) (api.ShowResponse, error) {
			return api.ShowResponse{Rule: api.RuleSummary{Name: "demo", Loaded: true}, Definition: "[[rule]]\nname = \"demo\""}, nil
		},
		status: func(context.Context, api.Empty) (api.StatusResponse, error) {
			return api.StatusResponse{Workers: 4, BusyWorkers: 2, QueueCapacity: 64, Counters: map[string]string{"can.tx": "3", "can.errors": "1"}}, nil
		},
		test: func(_ context.Context, r api.TestRequest) (api.TestResponse, error) {
			if r.Event.Topic != "input.test" {
				t.Fatal(r)
			}
			return api.TestResponse{Name: "demo", Matched: true, Steps: []api.StepPreview{{Kind: "can", After: "2s", Description: "would send CAN"}}}, nil
		},
	}
	cases := []struct {
		args     []string
		contains string
	}{{[]string{"list"}, "last-fire=123 errors=4"}, {[]string{"show", "demo"}, "[[rule]]"}, {[]string{"status"}, "can.tx: 3"}, {[]string{"test", "demo", "--event", `{"topic":"input.test"}`}, "would send CAN"}}
	for _, c := range cases {
		for _, asJSON := range []bool{false, true} {
			out, stderr, err := runExt(t, backend, asJSON, c.args...)
			if err != nil {
				t.Fatal(err)
			}
			if asJSON {
				if !json.Valid([]byte(out)) {
					t.Fatal(out)
				}
			} else if !strings.Contains(out, c.contains) {
				t.Fatal(out)
			}
			if c.args[0] == "test" && !strings.Contains(stderr, "current snapshot") {
				t.Fatal(stderr)
			}
		}
	}
}
func TestExtUnavailable(t *testing.T) {
	unavailable := errors.New("event-service unavailable")
	b := extensionAPI{list: func(context.Context, api.ListRequest) (api.ListResponse, error) {
		return api.ListResponse{}, unavailable
	}, status: func(context.Context, api.Empty) (api.StatusResponse, error) { return api.StatusResponse{}, unavailable }}
	for _, args := range [][]string{{"list"}, {"status"}, {"add", "x", "--on", "*", "--do", "redis", "--list", "x", "--push", "y"}} {
		_, _, err := runExt(t, b, false, args...)
		if !errors.Is(err, unavailable) {
			t.Fatal(err)
		}
	}
	old := redisClient
	redisClient = nil
	defer func() { redisClient = old }()
	_, err := extensionRemote[api.Empty, api.StatusResponse](api.MethodStatus)(context.Background(), api.Empty{})
	if err == nil {
		t.Fatal("nil Redis accepted")
	}
}

type fakeExtSubscription struct {
	messages   []*redis.Message
	confirmed  bool
	closed     chan struct{}
	once       sync.Once
	receiveErr error
	block      bool
}

func (s *fakeExtSubscription) Receive(context.Context) (interface{}, error) {
	s.confirmed = true
	return &redis.Subscription{Kind: "psubscribe", Channel: "ev:*", Count: 1}, s.receiveErr
}
func (s *fakeExtSubscription) ReceiveMessage(context.Context) (*redis.Message, error) {
	if !s.confirmed {
		panic("receive before confirmation")
	}
	if len(s.messages) > 0 {
		m := s.messages[0]
		s.messages = s.messages[1:]
		return m, nil
	}
	if s.block {
		<-s.closed
	}
	return nil, io.EOF
}
func (s *fakeExtSubscription) Close() error { s.once.Do(func() { close(s.closed) }); return nil }

type failingExtWriter struct{}

func (failingExtWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestExtTail(t *testing.T) {
	for _, asJSON := range []bool{false, true} {
		old := JSONOutput
		JSONOutput = asJSON
		sub := &fakeExtSubscription{closed: make(chan struct{}), messages: []*redis.Message{{Channel: "ev:x", Payload: "oops"}, {Channel: "ev:x", Payload: `{"topic":"x","src":"` + strings.Repeat("x", api.MaxRequestBytes) + `"}`}, {Channel: "ev:x", Payload: `{"topic":"x","ts":12,"src":"test","from":"a","to":"b"}`}}}
		cmd := &cobra.Command{}
		var out, stderr bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&stderr)
		err := extTail(context.Background(), cmd, sub)
		JSONOutput = old
		if !errors.Is(err, io.EOF) || !strings.Contains(stderr.String(), "malformed event") || !strings.Contains(stderr.String(), "oversized event") {
			t.Fatalf("%v %s", err, stderr.String())
		}
		if asJSON {
			if !json.Valid(bytes.TrimSpace(out.Bytes())) || strings.Count(out.String(), "\n") != 1 {
				t.Fatal(out.String())
			}
		} else if !strings.Contains(out.String(), "12 src=test topic=x from=a to=b") {
			t.Fatal(out.String())
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sub := &fakeExtSubscription{closed: make(chan struct{}), block: true}
	if err := extTail(ctx, &cobra.Command{}, sub); err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{}
	cmd.SetOut(failingExtWriter{})
	sub = &fakeExtSubscription{closed: make(chan struct{}), messages: []*redis.Message{{Payload: `{"topic":"x"}`}}}
	if err := extTail(context.Background(), cmd, sub); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	cmd.SetErr(failingExtWriter{})
	sub = &fakeExtSubscription{closed: make(chan struct{}), messages: []*redis.Message{{Payload: `bad`}}}
	if err := extTail(context.Background(), cmd, sub); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	sub = &fakeExtSubscription{closed: make(chan struct{}), receiveErr: io.EOF}
	if err := extTail(context.Background(), cmd, sub); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
}
func TestExtExecAndCANDefinition(t *testing.T) {
	for _, args := range [][]string{{"--do", "exec", "--command", "/usr/bin/true", "--timeout", "1s"}, {"--do", "can", "--iface", "can0", "--id", "0x123", "--data", "01 02"}, {"--do", "can", "--iface", "can0", "--id", "1abc", "--rtr", "--dlc", "8"}} {
		called := false
		b := extensionAPI{list: func(context.Context, api.ListRequest) (api.ListResponse, error) {
			return api.ListResponse{Revision: "r"}, nil
		}, add: func(_ context.Context, r api.AddRequest) (api.MutationResponse, error) {
			called = true
			var p map[string]any
			if _, err := toml.Decode(r.Definition, &p); err != nil {
				t.Fatal(err)
			}
			return api.MutationResponse{PendingRestart: true}, nil
		}}
		_, _, err := runExt(t, b, false, append([]string{"add", "demo", "--on", "*"}, args...)...)
		if err != nil || !called {
			t.Fatalf("%v called=%t", err, called)
		}
	}
}
