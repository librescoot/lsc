package locations

import (
	"fmt"
	"net"
	"strconv"
	"time"

	ipc "github.com/librescoot/redis-ipc"
)

// Wire types for settings-service's destination API.
const destinationChannel = "settings:destinations"
const destinationCallTimeout = 5 * time.Second

type saveRequest struct {
	ID        *int    `json:"id,omitempty"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Label     string  `json:"label"`
}

type saveResponse struct {
	ID   int    `json:"id"`
	UUID string `json:"uuid"`
}

type idRequest struct {
	ID int `json:"id"`
}

type emptyResponse struct{}

// destinationCall performs one RPC against settings-service. lsc runs one
// command per process, so the client lives only for the call.
func destinationCall[Req, Resp any](method string, request Req, response *Resp) error {
	if RedisClient == nil {
		return fmt.Errorf("redis client not initialised")
	}
	host, portStr, err := net.SplitHostPort(RedisClient.Addr())
	if err != nil {
		return fmt.Errorf("invalid redis address: %w", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid redis port: %w", err)
	}
	client, err := ipc.New(ipc.WithAddress(host), ipc.WithPort(port))
	if err != nil {
		return fmt.Errorf("connect to redis: %w", err)
	}
	defer client.Close()

	result, err := ipc.CallMethod[Req, Resp](client, destinationChannel, method,
		request, destinationCallTimeout)
	if err != nil {
		return err
	}
	*response = result
	return nil
}
