package lsd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"time"

	rdb "github.com/redis/go-redis/v9"
)

const pairingChannel = "cloud:pairing"

var pairingRequestID = regexp.MustCompile(`^(?:[0-9a-f]{32}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

type pairingRPCRequest struct {
	ID       string          `json:"id"`
	Method   string          `json:"method"`
	Reply    string          `json:"reply_channel"`
	Deadline int64           `json:"deadline"`
	Payload  json.RawMessage `json:"payload"`
}

func parsePairingRPC(data string, now time.Time) (pairingRPCRequest, error) {
	var request pairingRPCRequest
	if len(data) > 1024 {
		return request, errors.New("invalid request")
	}
	decoder := json.NewDecoder(bytes.NewBufferString(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(new(any)) != io.EOF {
		return request, errors.New("invalid request")
	}
	if !pairingRequestID.MatchString(request.ID) || request.Reply != pairingChannel+":reply:"+request.ID || request.Deadline <= now.UnixMilli() || request.Deadline > now.Add(15*time.Second).UnixMilli() {
		return request, errors.New("invalid request")
	}
	var payload map[string]any
	if json.Unmarshal(request.Payload, &payload) != nil || payload == nil || len(payload) != 0 {
		return request, errors.New("unexpected arguments")
	}
	if request.Method != "start" && request.Method != "status" && request.Method != "stop" {
		return request, errors.New("invalid action")
	}
	return request, nil
}

func (s *Server) pairingRPC(ctx context.Context, request pairingRPCRequest) (pairingStatus, error) {
	if request.Method == "start" {
		client := s.getRedis()
		if client == nil {
			return pairingStatus{}, errors.New("pairing unavailable")
		}
		state, err := client.GetClient().HGet(ctx, "vehicle", "state").Result()
		if err != nil || state != "parked" && state != "stand-by" {
			return pairingStatus{}, errors.New("scooter must be stationary")
		}
	}
	run := s.pairingRun
	if run == nil {
		run = s.runPairing
	}
	result, err := run(ctx, request.Method)
	if err != nil {
		return pairingStatus{}, errors.New("pairing unavailable")
	}
	return sanitizePairing(result)
}

func (s *Server) runPairingBridge() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-s.doneCh:
			cancel()
		case <-ctx.Done():
		}
	}()
	for ctx.Err() == nil {
		client := s.getRedis()
		if client == nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				continue
			}
		}
		values, err := client.GetClient().BRPop(ctx, time.Second, pairingChannel).Result()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if !errors.Is(err, rdb.Nil) {
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
				}
			}
			continue
		}
		if len(values) != 2 {
			continue
		}
		// A single consumer validates deadlines after dequeue, so old writes never run.
		request, err := parsePairingRPC(values[1], time.Now())
		if err != nil {
			continue
		}
		callCtx, done := context.WithDeadline(ctx, time.UnixMilli(request.Deadline))
		status, err := s.pairingRPC(callCtx, request)
		done()
		var reply any
		if err != nil {
			reply = map[string]any{"ok": false, "error": "pairing unavailable or scooter not stationary"}
		} else {
			reply = map[string]any{"ok": true, "payload": status}
		}
		data, encodeErr := json.Marshal(reply)
		if encodeErr == nil {
			_ = client.GetClient().Publish(ctx, request.Reply, data).Err()
		}
	}
}
