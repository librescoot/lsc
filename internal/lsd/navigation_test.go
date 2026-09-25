package lsd

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	ipc "github.com/librescoot/redis-ipc"
	"librescoot/lsc/internal/redis"
	"librescoot/lsc/internal/routeplan"
)

func TestSetDestinationCallsOwner(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(mr.Addr())
	defer client.Close()
	port, _ := strconv.Atoi(mr.Port())
	serverClient, err := ipc.New(ipc.WithAddress("127.0.0.1"), ipc.WithPort(port))
	if err != nil {
		t.Fatal(err)
	}
	defer serverClient.Close()
	server := ipc.NewCallServer(serverClient, "settings:route-plan")
	var received []routeplan.StopInput
	ipc.RegisterCall[routeplan.ReplaceRequest, routeplan.Plan](server, "plan.replace", func(req routeplan.ReplaceRequest) (routeplan.Plan, error) {
		received = req.Stops
		// Owner publishes the compatibility projection before replying.
		if err := client.HSet("navigation", "destination", "52.500000,13.400000"); err != nil {
			return routeplan.Plan{}, err
		}
		return routeplan.Plan{ID: "plan", Stops: []routeplan.Stop{}}, nil
	})
	server.Start()
	defer server.Stop()
	s := &Server{rdb: client}
	request := httptest.NewRequest(http.MethodPost, "/api/navigation", strings.NewReader(`{"latitude":52.5,"longitude":13.4,"address":"Home"}`))
	response := httptest.NewRecorder()
	s.handleNavigation(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	if len(received) != 1 || received[0].Lat != 52.5 || received[0].Label != "Home" {
		t.Fatalf("request stops = %+v", received)
	}
	if !strings.Contains(response.Body.String(), "52.500000,13.400000") {
		t.Fatalf("response = %s", response.Body.String())
	}
}

func TestSetDestinationFailsWithoutOwner(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(mr.Addr())
	defer client.Close()
	s := &Server{rdb: client}
	request := httptest.NewRequest(http.MethodPost, "/api/navigation", strings.NewReader(`{"latitude":52.5,"longitude":13.4}`))
	response := httptest.NewRecorder()
	s.handleNavigation(response, request)
	if response.Code != http.StatusBadGateway {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	fields, err := client.HGetAll("navigation")
	if err != nil || len(fields) != 0 {
		t.Fatalf("unexpected fallback navigation write: %v, %v", fields, err)
	}
}
