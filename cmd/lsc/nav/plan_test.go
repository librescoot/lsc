package nav

import (
	"strconv"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	ipc "github.com/librescoot/redis-ipc"
	"librescoot/lsc/internal/redis"
	"librescoot/lsc/internal/routeplan"
)

func TestRapidPlanAddUsesAtomicAppend(t *testing.T) {
	mr := miniredis.RunT(t)
	previous := RedisClient
	RedisClient = redis.NewClient(mr.Addr())
	t.Cleanup(func() { RedisClient.Close(); RedisClient = previous })
	port, _ := strconv.Atoi(mr.Port())
	serverClient, err := ipc.New(ipc.WithAddress("127.0.0.1"), ipc.WithPort(port))
	if err != nil {
		t.Fatal(err)
	}
	defer serverClient.Close()
	server := ipc.NewCallServer(serverClient, "settings:route-plan", ipc.WithCallServerConcurrency(1))
	plan := routeplan.Plan{Stops: []routeplan.Stop{}}
	ipc.RegisterCall[routeplan.AppendRequest, routeplan.Plan](server, "plan.append", func(req routeplan.AppendRequest) (routeplan.Plan, error) {
		if plan.ID == "" {
			plan.ID = "plan-1"
		}
		plan.Revision++
		plan.Stops = append(plan.Stops, routeplan.Stop{ID: "stop", Lat: req.Stop.Lat, Lon: req.Stop.Lon, Label: req.Stop.Label})
		return plan, nil
	})
	ipc.RegisterCall[routeplan.Empty, routeplan.Plan](server, "plan.get", func(routeplan.Empty) (routeplan.Plan, error) { return plan, nil })
	server.Start()
	defer server.Stop()
	for _, coord := range []string{"52.5,13.4", "52.6,13.5"} {
		if err := navPlanAddCmd.RunE(navPlanAddCmd, []string{coord}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := routeplan.Call(RedisClient, "plan.get", routeplan.Empty{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Stops) != 2 || got.Stops[0].Lat != 52.5 || got.Stops[1].Lat != 52.6 {
		t.Fatalf("plan = %+v", got)
	}
	var calls sync.WaitGroup
	for _, lat := range []float64{52.7, 52.8} {
		calls.Add(1)
		go func(lat float64) {
			defer calls.Done()
			if _, err := routeplan.Call(RedisClient, "plan.append", routeplan.AppendRequest{
				Stop: routeplan.StopInput{Lat: lat, Lon: 13.4},
			}); err != nil {
				t.Errorf("concurrent append: %v", err)
			}
		}(lat)
	}
	calls.Wait()
	got, err = routeplan.Call(RedisClient, "plan.get", routeplan.Empty{})
	if err != nil || len(got.Stops) != 4 {
		t.Fatalf("concurrent appends: plan=%+v err=%v", got, err)
	}
	if fields, _ := RedisClient.HGetAll("navigation"); len(fields) != 0 {
		t.Fatalf("CLI wrote navigation: %v", fields)
	}
}

func TestPlanAddFailsWhenOwnerUnavailable(t *testing.T) {
	mr := miniredis.RunT(t)
	previousClient, previousJSON := RedisClient, JSONOutput
	RedisClient = redis.NewClient(mr.Addr())
	jsonOutput := false
	JSONOutput = &jsonOutput
	t.Cleanup(func() { RedisClient.Close(); RedisClient = previousClient; JSONOutput = previousJSON })
	// The idle Redis server has no route-plan RPC handler.
	err := navPlanAddCmd.RunE(navPlanAddCmd, []string{"52.5,13.4"})
	if err == nil {
		t.Fatal("add succeeded without owner")
	}
	fields, _ := RedisClient.HGetAll("navigation")
	if len(fields) != 0 {
		t.Fatal("fallback navigation write")
	}
}
