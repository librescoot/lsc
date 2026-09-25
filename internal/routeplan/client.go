package routeplan

import (
	"fmt"
	"net"
	"strconv"
	"time"

	ipc "github.com/librescoot/redis-ipc"
	"librescoot/lsc/internal/redis"
)

const channel = "settings:route-plan"
const timeout = 5 * time.Second

type StopInput struct {
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
	Label string  `json:"label"`
}
type Stop struct {
	ID      string  `json:"id"`
	Lat     float64 `json:"lat"`
	Lon     float64 `json:"lon"`
	Label   string  `json:"label"`
	Reached bool    `json:"reached"`
}
type Plan struct {
	ID          string `json:"id"`
	Revision    uint64 `json:"revision"`
	Stops       []Stop `json:"stops"`
	CurrentStep int    `json:"current_step"`
}
type Empty struct{}
type ReplaceRequest struct {
	Stops []StopInput `json:"stops"`
}
type AppendRequest struct {
	Stop StopInput `json:"stop"`
}
type RemoveRequest struct {
	Index            int    `json:"index"`
	ExpectedRevision uint64 `json:"expected_revision"`
}
type ProgressRequest struct {
	ExpectedPlanID string `json:"expected_plan_id"`
	ExpectedStopID string `json:"expected_stop_id"`
}
type ClearRequest struct {
	ExpectedPlanID string `json:"expected_plan_id,omitempty"`
}

func Call[Req any](client *redis.Client, method string, request Req) (Plan, error) {
	if client == nil {
		return Plan{}, fmt.Errorf("redis client not initialized")
	}
	host, portText, err := net.SplitHostPort(client.Addr())
	if err != nil {
		return Plan{}, fmt.Errorf("invalid redis address: %w", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return Plan{}, fmt.Errorf("invalid redis port: %w", err)
	}
	rpc, err := ipc.New(ipc.WithAddress(host), ipc.WithPort(port))
	if err != nil {
		return Plan{}, fmt.Errorf("connect to redis: %w", err)
	}
	defer rpc.Close()
	plan, err := ipc.CallMethod[Req, Plan](rpc, channel, method, request, timeout)
	if err != nil {
		return Plan{}, fmt.Errorf("route plan %s: %w", method, err)
	}
	return plan, nil
}
