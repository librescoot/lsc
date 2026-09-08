package lsc

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
	"unicode/utf8"

	completionutil "librescoot/lsc/internal/completion"
	redisclient "librescoot/lsc/internal/redis"

	"github.com/BurntSushi/toml"
	"github.com/librescoot/event-service/api"
	"github.com/librescoot/eventbus"
	"github.com/redis/go-redis/v9"
	"github.com/spf13/cobra"
)

type extensionAPI struct {
	list   func(context.Context, api.ListRequest) (api.ListResponse, error)
	show   func(context.Context, api.ShowRequest) (api.ShowResponse, error)
	add    func(context.Context, api.AddRequest) (api.MutationResponse, error)
	set    func(context.Context, api.SetEnabledRequest) (api.MutationResponse, error)
	test   func(context.Context, api.TestRequest) (api.TestResponse, error)
	status func(context.Context, api.Empty) (api.StatusResponse, error)
}

func extensionRemote[Req, Resp any](method string) func(context.Context, Req) (Resp, error) {
	return func(ctx context.Context, req Req) (Resp, error) {
		var zero Resp
		if redisClient == nil {
			return zero, fmt.Errorf("event-service unavailable: Redis is not connected")
		}
		resp, err := api.Call[Req, Resp](ctx, redisClient.GetClient(), method, req)
		if err != nil {
			return zero, fmt.Errorf("event-service %s: %w", method, err)
		}
		return resp, nil
	}
}

func init() {
	rootCmd.AddCommand(newExtCommand(extensionAPI{
		list:   extensionRemote[api.ListRequest, api.ListResponse](api.MethodList),
		show:   extensionRemote[api.ShowRequest, api.ShowResponse](api.MethodShow),
		add:    extensionRemote[api.AddRequest, api.MutationResponse](api.MethodAdd),
		set:    extensionRemote[api.SetEnabledRequest, api.MutationResponse](api.MethodSetEnabled),
		test:   extensionRemote[api.TestRequest, api.TestResponse](api.MethodTest),
		status: extensionRemote[api.Empty, api.StatusResponse](api.MethodStatus),
	}))
}

func extJSON(cmd *cobra.Command, value any) error {
	return json.NewEncoder(cmd.OutOrStdout()).Encode(value)
}
func extSummary(w io.Writer, r api.RuleSummary) error {
	_, err := fmt.Fprintf(w, "%s  enabled=%t loaded=%t last-fire=%d errors=%d active-runs=%d source=%s\n", r.Name, r.Enabled, r.Loaded, r.LastFire, r.Errors, r.ActiveRuns, r.Source)
	if err != nil {
		return err
	}
	for _, d := range r.Diagnostics {
		if _, err = fmt.Fprintf(w, "  diagnostic: %s\n", d); err != nil {
			return err
		}
	}
	return nil
}
func extMutation(cmd *cobra.Command, r api.MutationResponse) error {
	if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "Warning: current running rules are unchanged until an explicit event-service restart. No restart was performed."); err != nil {
		return err
	}
	if JSONOutput {
		return extJSON(cmd, r)
	}
	_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\nPending restart: %t\nRevision: %s\n", r.Name, r.Message, r.PendingRestart, r.Revision)
	return err
}
func extList(ctx context.Context, backend extensionAPI) (api.ListResponse, error) {
	var all api.ListResponse
	for offset := 0; ; {
		page, err := backend.list(ctx, api.ListRequest{Offset: offset, Limit: 100})
		if err != nil {
			return all, err
		}
		if offset == 0 {
			all = page
			all.Rules = append([]api.RuleSummary{}, page.Rules...)
		} else {
			if page.Revision != all.Revision || page.AppliedRevision != all.AppliedRevision || page.Total != all.Total || page.PendingRestart != all.PendingRestart {
				return all, fmt.Errorf("extension configuration changed during pagination; run list again")
			}
			all.Rules = append(all.Rules, page.Rules...)
			for _, d := range page.Diagnostics {
				found := false
				for _, old := range all.Diagnostics {
					if old == d {
						found = true
						break
					}
				}
				if !found {
					all.Diagnostics = append(all.Diagnostics, d)
				}
			}
		}
		if page.Total < 0 || len(page.Rules) > 100 || len(all.Rules) > all.Total {
			return all, fmt.Errorf("invalid extension list pagination")
		}
		offset += len(page.Rules)
		if offset == all.Total {
			return all, nil
		}
		if len(page.Rules) == 0 {
			return all, fmt.Errorf("incomplete extension list page")
		}
	}
}
func completeExtNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	candidates := completionutil.WithRedis(cmd, func(ctx context.Context, client *redisclient.Client) ([]string, error) {
		response, err := api.Call[api.ListRequest, api.ListResponse](ctx, client.GetClient(), api.MethodList, api.ListRequest{Limit: 100})
		if err != nil {
			return nil, err
		}
		values := make([]string, 0, len(response.Rules))
		for _, rule := range response.Rules {
			if !strings.HasPrefix(rule.Name, toComplete) {
				continue
			}
			state := "disabled"
			if rule.Enabled {
				state = "enabled"
			}
			values = append(values, rule.Name+"\t"+state)
		}
		sort.Strings(values)
		return values, nil
	})
	return candidates, cobra.ShellCompDirectiveNoFileComp
}

func extNameArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.ExactArgs(1)(cmd, args); err != nil {
		return err
	}
	name := args[0]
	if !utf8.ValidString(name) || len(name) < 1 || len(name) > 128 || strings.TrimSpace(name) != name || name == "." || name == ".." || strings.ContainsAny(name, "/\\") || strings.ContainsFunc(name, unicode.IsControl) {
		return fmt.Errorf("name must be 1..128 bytes, trimmed, without controls or path separators, and not a dot path")
	}
	return nil
}
func newExtCommand(backend extensionAPI) *cobra.Command {
	ext := &cobra.Command{Use: "ext", Short: "Manage event-service extensions (changes require restart)", GroupID: "main"}
	ext.AddCommand(&cobra.Command{Use: "list", Short: "List desired and loaded rules", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		r, err := extList(cmd.Context(), backend)
		if err != nil {
			return err
		}
		if JSONOutput {
			return extJSON(cmd, r)
		}
		if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Rules: %d  Revision: %s  Applied: %s\nPending restart: %t\n", r.Total, r.Revision, r.AppliedRevision, r.PendingRestart); err != nil {
			return err
		}
		for _, rule := range r.Rules {
			if err = extSummary(cmd.OutOrStdout(), rule); err != nil {
				return err
			}
		}
		for _, d := range r.Diagnostics {
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Diagnostic: %s\n", d); err != nil {
				return err
			}
		}
		return nil
	}})
	ext.AddCommand(&cobra.Command{Use: "show <name>", Short: "Show rule summary and TOML definition", Args: extNameArgs, ValidArgsFunction: completeExtNames, RunE: func(cmd *cobra.Command, args []string) error {
		r, err := backend.show(cmd.Context(), api.ShowRequest{Name: args[0]})
		if err != nil {
			return err
		}
		if JSONOutput {
			return extJSON(cmd, r)
		}
		if err = extSummary(cmd.OutOrStdout(), r.Rule); err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Revision: %s\nPending restart: %t\n\n%s\n", r.Revision, r.PendingRestart, r.Definition)
		return err
	}})
	for _, enabled := range []bool{true, false} {
		name := "disable"
		if enabled {
			name = "enable"
		}
		ext.AddCommand(&cobra.Command{Use: name + " <name>", Short: name + " a rule after the next explicit restart", Args: extNameArgs, ValidArgsFunction: completeExtNames, RunE: func(cmd *cobra.Command, args []string) error {
			shown, err := backend.show(cmd.Context(), api.ShowRequest{Name: args[0]})
			if err != nil {
				return err
			}
			r, err := backend.set(cmd.Context(), api.SetEnabledRequest{Name: args[0], Enabled: enabled, ExpectedRevision: shown.Revision})
			if err != nil {
				return err
			}
			return extMutation(cmd, r)
		}})
	}
	ext.AddCommand(newExtAdd(backend))
	var eventJSON string
	test := &cobra.Command{Use: "test <name> --event JSON", Short: "Dry-run conditions and action previews; never execute or publish", ValidArgsFunction: completeExtNames, Args: func(cmd *cobra.Command, args []string) error {
		if err := extNameArgs(cmd, args); err != nil {
			return err
		}
		_, err := extParseEvent(eventJSON)
		return err
	}, RunE: func(cmd *cobra.Command, args []string) error {
		event, _ := extParseEvent(eventJSON)
		r, err := backend.test(cmd.Context(), api.TestRequest{Name: args[0], Event: event})
		if err != nil {
			return err
		}
		if _, err = fmt.Fprintln(cmd.ErrOrStderr(), "Dry run only: delayed checks use the current snapshot, not future state. No events published or actions executed."); err != nil {
			return err
		}
		if JSONOutput {
			return extJSON(cmd, r)
		}
		if _, err = fmt.Fprintf(cmd.OutOrStdout(), "%s: enabled=%t matched=%t\nCooldown: %s Debounce: %s Repeat: %d every %s\n%s\n", r.Name, r.Enabled, r.Matched, r.Cooldown, r.Debounce, r.RepeatCount, r.RepeatEvery, r.Note); err != nil {
			return err
		}
		for _, step := range r.Steps {
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "Step %d: %s condition=%t after=%s durable=%t %s error=%s\n", step.Index, step.Kind, step.Condition, step.After, step.Durable, step.Description, step.Error); err != nil {
				return err
			}
		}
		if r.Error != "" {
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Error:", r.Error)
		}
		return err
	}}
	test.Flags().StringVar(&eventJSON, "event", "", "Event envelope as JSON (requires topic)")
	ext.AddCommand(test)
	ext.AddCommand(&cobra.Command{Use: "status", Short: "Query live service workers, queue and counters", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		r, err := backend.status(cmd.Context(), api.Empty{})
		if err != nil {
			return err
		}
		if JSONOutput {
			return extJSON(cmd, r)
		}
		if _, err = fmt.Fprintf(cmd.OutOrStdout(), "event-service alive: %s (API %d)\nWorkers: %d busy / %d total\nQueue: %d / %d\nRevision: %s Applied: %s\nPending restart: %t\n", r.Version, r.APIVersion, r.BusyWorkers, r.Workers, r.QueueDepth, r.QueueCapacity, r.Revision, r.AppliedRevision, r.PendingRestart); err != nil {
			return err
		}
		keys := make([]string, 0, len(r.Counters))
		for k := range r.Counters {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if _, err = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", k, r.Counters[k]); err != nil {
				return err
			}
		}
		return nil
	}})
	ext.AddCommand(&cobra.Command{Use: "tail [topic-glob]", Short: "Watch ev: events (exact topic, * or prefix.*)", Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.MaximumNArgs(1)(cmd, args); err != nil {
			return err
		}
		if len(args) > 0 {
			return extTopicPattern(args[0])
		}
		return nil
	}, RunE: func(cmd *cobra.Command, args []string) error {
		pattern := "*"
		if len(args) > 0 {
			pattern = args[0]
		}
		if redisClient == nil {
			return fmt.Errorf("redis is not connected")
		}
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		sub := redisClient.GetClient().PSubscribe(ctx, "ev:"+pattern)
		return extTail(ctx, cmd, sub)
	}})
	return ext
}

func extTopicPattern(s string) error {
	base := s
	if s == "*" {
		return nil
	}
	if strings.HasSuffix(s, ".*") {
		base = strings.TrimSuffix(s, ".*")
	}
	if base == "" || strings.ContainsAny(base, "*?[]\\:\x00") || strings.ContainsFunc(base, unicode.IsSpace) {
		return fmt.Errorf("invalid topic pattern %q: use an exact topic, * or prefix.*", s)
	}
	return nil
}
func extParseEvent(s string) (eventbus.Event, error) {
	var event eventbus.Event
	if err := json.Unmarshal([]byte(s), &event); err != nil {
		return event, fmt.Errorf("invalid event JSON: %w", err)
	}
	if event.Topic == "" {
		return event, fmt.Errorf("event topic is required")
	}
	return event, nil
}

type extSubscription interface {
	Receive(context.Context) (interface{}, error)
	ReceiveMessage(context.Context) (*redis.Message, error)
	Close() error
}

func extTail(ctx context.Context, cmd *cobra.Command, sub extSubscription) error {
	defer sub.Close()
	// Closing unblocks Receive even when the Redis client's socket read ignores context cancellation.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = sub.Close()
		case <-done:
		}
	}()
	confirmation, err := sub.Receive(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("subscribe to events: %w", err)
	}
	if subscribed, ok := confirmation.(*redis.Subscription); !ok || subscribed.Kind != "psubscribe" || subscribed.Count < 1 {
		return fmt.Errorf("event subscription was not confirmed")
	}
	for {
		msg, err := sub.ReceiveMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("event subscription closed: %w", err)
		}
		if msg == nil {
			return fmt.Errorf("event subscription closed")
		}
		if len(msg.Payload) > api.MaxRequestBytes {
			if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "Warning: oversized event on %q skipped (limit 64 KiB)\n", msg.Channel); err != nil {
				return err
			}
			continue
		}
		event, err := extParseEvent(msg.Payload)
		if err != nil {
			if _, err = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: malformed event on %s: %v\n", msg.Channel, err); err != nil {
				return err
			}
			continue
		}
		if JSONOutput {
			err = extJSON(cmd, event)
		} else {
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%d src=%s topic=%s from=%s to=%s\n", event.TS, event.Src, event.Topic, event.From, event.To)
		}
		if err != nil {
			return err
		}
	}
}

type extAddOptions struct {
	on                                                  []string
	kind, list, push, command, timeout, iface, id, data string
	rtr                                                 bool
	dlc                                                 int
}

func newExtAdd(backend extensionAPI) *cobra.Command {
	var o extAddOptions
	var definition string
	cmd := &cobra.Command{Use: "add <name> --on topic --do redis --list key --push value", Short: "Add a single-step rule for the next explicit restart", Args: func(cmd *cobra.Command, args []string) error {
		if err := extNameArgs(cmd, args); err != nil {
			return err
		}
		var err error
		definition, err = extDefinition(cmd, args[0], o)
		return err
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		listed, err := backend.list(cmd.Context(), api.ListRequest{Limit: 100})
		if err != nil {
			return err
		}
		r, err := backend.add(cmd.Context(), api.AddRequest{Definition: definition, ExpectedRevision: listed.Revision})
		if err != nil {
			return err
		}
		return extMutation(cmd, r)
	}}
	f := cmd.Flags()
	f.StringSliceVar(&o.on, "on", nil, "Trigger topics (repeat or comma separate)")
	f.StringVar(&o.kind, "do", "", "Action: redis, exec or can")
	f.StringVar(&o.list, "list", "", "Redis list")
	f.StringVar(&o.push, "push", "", "Value to push")
	f.StringVar(&o.command, "command", "", "Executable path/name (not shell text)")
	f.StringVar(&o.timeout, "timeout", "", "Exec timeout (positive duration)")
	f.StringVar(&o.iface, "iface", "", "CAN interface")
	f.StringVar(&o.id, "id", "", "CAN identifier (hex)")
	f.StringVar(&o.data, "data", "", "CAN hex bytes")
	f.BoolVar(&o.rtr, "rtr", false, "CAN remote request frame")
	f.IntVar(&o.dlc, "dlc", 0, "RTR requested length (0..8)")
	return cmd
}
func extDefinition(cmd *cobra.Command, name string, o extAddOptions) (string, error) {
	if len(o.on) == 0 {
		return "", fmt.Errorf("--on is required")
	}
	for _, topic := range o.on {
		if err := extTopicPattern(topic); err != nil {
			return "", err
		}
	}
	step := map[string]any{"do": o.kind}
	allowed := map[string]bool{}
	switch o.kind {
	case "redis":
		allowed["list"] = true
		allowed["push"] = true
		if o.list == "" || o.push == "" {
			return "", fmt.Errorf("redis requires nonempty --list and --push")
		}
		step["list"] = o.list
		step["push"] = o.push
	case "exec":
		allowed["command"] = true
		allowed["timeout"] = true
		if strings.TrimSpace(o.command) == "" {
			return "", fmt.Errorf("exec requires --command")
		}
		step["command"] = o.command
		if cmd.Flags().Changed("timeout") {
			d, err := time.ParseDuration(o.timeout)
			if err != nil || d <= 0 {
				return "", fmt.Errorf("--timeout must be a positive duration")
			}
			step["timeout"] = o.timeout
		}
	case "can":
		for _, k := range []string{"iface", "id", "data", "rtr", "dlc"} {
			allowed[k] = true
		}
		if len(o.iface) == 0 || len(o.iface) > 15 || strings.ContainsAny(o.iface, "/\x00") || strings.ContainsFunc(o.iface, unicode.IsSpace) {
			return "", fmt.Errorf("invalid --iface")
		}
		id := strings.TrimPrefix(strings.TrimPrefix(o.id, "0x"), "0X")
		if id == "" || strings.ContainsAny(id, "+-") {
			return "", fmt.Errorf("invalid CAN --id")
		}
		if _, err := strconv.ParseUint(id, 16, 29); err != nil {
			return "", fmt.Errorf("invalid CAN --id: %w", err)
		}
		data := strings.TrimSpace(o.data)
		if strings.ContainsFunc(data, unicode.IsSpace) {
			fields := strings.Fields(data)
			for _, field := range fields {
				if len(field) != 2 {
					return "", fmt.Errorf("CAN bytes must have two hex digits")
				}
			}
			data = strings.Join(fields, "")
		}
		decoded, err := hex.DecodeString(data)
		if err != nil || len(decoded) > 8 {
			return "", fmt.Errorf("CAN --data must contain at most 8 hex bytes")
		}
		if o.rtr {
			if data != "" || o.dlc < 0 || o.dlc > 8 {
				return "", fmt.Errorf("RTR requires no data and DLC 0..8")
			}
		} else if cmd.Flags().Changed("dlc") {
			return "", fmt.Errorf("--dlc requires --rtr")
		}
		step["iface"] = o.iface
		step["id"] = o.id
		step["data"] = o.data
		step["rtr"] = o.rtr
		if cmd.Flags().Changed("dlc") {
			step["dlc"] = o.dlc
		}
	default:
		return "", fmt.Errorf("--do must be redis, exec or can")
	}
	for _, flag := range []string{"list", "push", "command", "timeout", "iface", "id", "data", "rtr", "dlc"} {
		if cmd.Flags().Changed(flag) && !allowed[flag] {
			return "", fmt.Errorf("--%s is not valid for --do %s", flag, o.kind)
		}
	}
	var b bytes.Buffer
	err := toml.NewEncoder(&b).Encode(map[string]any{"rule": []map[string]any{{"name": name, "on": o.on, "step": []map[string]any{step}}}})
	if err != nil {
		return "", err
	}
	if b.Len() > api.MaxRequestBytes/2 {
		return "", fmt.Errorf("rule definition is too large")
	}
	return b.String(), nil
}
