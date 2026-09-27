package lsd

import (
	"fmt"
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

func TestNavigationPlanActions(t *testing.T) {
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
	plan := routeplan.Plan{ID: "route", Revision: 1, Stops: []routeplan.Stop{{ID: "first", Lat: 52.5, Lon: 13.4, Label: "First"}}, CurrentStep: 0}
	ipc.RegisterCall[routeplan.Empty, routeplan.Plan](server, "plan.get", func(routeplan.Empty) (routeplan.Plan, error) { return plan, nil })
	ipc.RegisterCall[routeplan.AppendRequest, routeplan.Plan](server, "plan.append", func(req routeplan.AppendRequest) (routeplan.Plan, error) {
		plan.Stops = append(plan.Stops, routeplan.Stop{ID: "second", Lat: req.Stop.Lat, Lon: req.Stop.Lon, Label: req.Stop.Label})
		plan.Revision++
		return plan, nil
	})
	ipc.RegisterCall[routeplan.ProgressRequest, routeplan.Plan](server, "plan.advance", func(req routeplan.ProgressRequest) (routeplan.Plan, error) {
		if req.ExpectedPlanID != plan.ID || req.ExpectedStopID != plan.Stops[plan.CurrentStep].ID {
			return routeplan.Plan{}, fmt.Errorf("stale plan or stop")
		}
		plan.CurrentStep++
		plan.Revision++
		return plan, nil
	})
	ipc.RegisterCall[routeplan.RemoveRequest, routeplan.Plan](server, "plan.remove", func(req routeplan.RemoveRequest) (routeplan.Plan, error) {
		if req.ExpectedRevision != plan.Revision {
			return routeplan.Plan{}, fmt.Errorf("stale revision")
		}
		plan.Stops = append(plan.Stops[:req.Index], plan.Stops[req.Index+1:]...)
		plan.CurrentStep = 0
		plan.Revision++
		return plan, nil
	})
	server.Start()
	defer server.Stop()
	s := &Server{rdb: client}

	get := httptest.NewRecorder()
	s.handleNavigation(get, httptest.NewRequest(http.MethodGet, "/api/navigation", nil))
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"plan":{"id":"route"`) {
		t.Fatalf("get plan: %d %s", get.Code, get.Body.String())
	}
	post := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		s.handleNavigationPlan(response, httptest.NewRequest(http.MethodPost, "/api/navigation/plan", strings.NewReader(body)))
		return response
	}
	appended := post(`{"action":"append","stop":{"lat":52.6,"lon":13.5,"label":" Second "}}`)
	if appended.Code != http.StatusOK || len(plan.Stops) != 2 || plan.Stops[1].Label != "Second" {
		t.Fatalf("append: %d %s, plan=%+v", appended.Code, appended.Body.String(), plan)
	}
	for _, body := range []string{
		`{"action":"append","stop":{"lat":91,"lon":13}}`,
		`{"action":"skip"}`,
		`{"action":"remove","index":-1,"expected_revision":2}`,
	} {
		if response := post(body); response.Code != http.StatusBadRequest {
			t.Fatalf("invalid %s: %d %s", body, response.Code, response.Body.String())
		}
	}
	if response := post(`{"action":"skip","expected_plan_id":"route","expected_stop_id":"wrong"}`); response.Code != http.StatusBadGateway {
		t.Fatalf("stale skip: %d %s", response.Code, response.Body.String())
	}
	if response := post(`{"action":"skip","expected_plan_id":"route","expected_stop_id":"first"}`); response.Code != http.StatusOK || plan.CurrentStep != 1 {
		t.Fatalf("skip: %d %s, plan=%+v", response.Code, response.Body.String(), plan)
	}
	if response := post(`{"action":"remove","index":0,"expected_revision":2}`); response.Code != http.StatusBadGateway {
		t.Fatalf("stale remove: %d %s", response.Code, response.Body.String())
	}
	if response := post(`{"action":"remove","index":0,"expected_revision":3}`); response.Code != http.StatusOK || len(plan.Stops) != 1 {
		t.Fatalf("remove: %d %s, plan=%+v", response.Code, response.Body.String(), plan)
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
