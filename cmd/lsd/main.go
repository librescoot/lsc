// lsd, the Librescoot Daemon.
//
// A small web server that runs on the MDB and exposes scooter configuration,
// control and status over the usb0 management network and, when configured,
// WireGuard. It is the web-based complement to the lsc CLI: same Redis
// interfaces, browser front end.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"librescoot/lsc/internal/lsd"
	"librescoot/lsc/internal/redis"
)

var version = "dev"

func main() {
	var (
		addr     = flag.String("addr", "192.168.7.1:8090", "HTTP listen address (default is the MDB usb0 address)")
		wgIface  = flag.String("wg-interface", "", "Also listen on this WireGuard interface (default wg0 with the default -addr)")
		redisAdr = flag.String("redis-addr", "localhost:6379", "Redis server address (host:port)")
		dataDir  = flag.String("data", "/data", "Data directory to expose in the file browser")
		token    = flag.String("token", "", "Require a bearer token for all requests (empty disables auth)")
		sunshine = flag.String("sunshine-url", "", "Sunshine instance for the Cloud page (default https://sunshine.rescoot.org)")
		noShell  = flag.Bool("no-shell", false, "Disable the shell page and its API")
		showVer  = flag.Bool("version", false, "Print version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Println(version)
		return
	}

	// journald stamps every line itself; only add timestamps when running
	// in a terminal or into a plain log file.
	if os.Getenv("JOURNAL_STREAM") != "" {
		log.SetFlags(0)
	} else {
		log.SetFlags(log.Ldate | log.Ltime)
	}

	srv, err := lsd.New(lsd.Options{
		Version:     version,
		DataDir:     *dataDir,
		Token:       *token,
		SunshineURL: *sunshine,
		Shell:       !*noShell,
	})
	if err != nil {
		log.Fatalf("Failed to initialize: %v", err)
	}
	log.Printf("lsd %s starting", version)

	// Connect to Redis in the background: the HTTP server comes up even if
	// Redis is not ready yet, and API handlers report the connection state.
	client := redis.NewClient(*redisAdr)
	go func() {
		for {
			if err := client.Connect(); err == nil {
				log.Printf("Connected to Redis at %s", *redisAdr)
				if err := srv.SetRedis(client); err != nil {
					log.Printf("Redis setup failed: %v", err)
				}
				return
			}
			log.Printf("Redis not reachable at %s yet, retrying", *redisAdr)
			time.Sleep(3 * time.Second)
		}
	}()

	iface := *wgIface
	if iface == "" && *addr == "192.168.7.1:8090" {
		iface = "wg0"
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServeOn(*addr, iface)
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	select {
	case s := <-sig:
		log.Printf("Received %s, shutting down", s)
		srv.Shutdown()
	case err := <-errCh:
		if err != nil {
			log.Fatalf("HTTP server: %v", err)
		}
	}
}
