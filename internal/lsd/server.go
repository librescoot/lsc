// Package lsd implements the Librescoot Daemon web server: a small HTTP
// front end over the Redis interfaces the Librescoot services already expose.
//
// The daemon is deliberately boring: net/http, an embedded single-page UI and
// one Redis connection. It runs on the MDB and, by default, listens only on
// the usb0 management address (192.168.7.1), so its reachability matches the
// usb0 link, the same exposure the data-server has.
package lsd

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"librescoot/lsc/internal/redis"
	"librescoot/lsc/internal/schema"
)

//go:embed static
var staticFiles embed.FS

// Options configures the daemon.
type Options struct {
	Version     string
	DataDir     string
	Token       string
	SunshineURL string
}

// Server holds the daemon state.
type Server struct {
	version     string
	dataDir     string
	token       string
	sunshineURL string

	mu       sync.RWMutex
	rdb      *redis.Client
	schema   *schema.Schema
	stopping bool

	hub        *hub
	doneCh     chan struct{}
	httpServer *http.Server
}

// New creates a Server. Redis is attached later via SetRedis.
func New(opts Options) (*Server, error) {
	if opts.DataDir == "" {
		opts.DataDir = "/data"
	}
	if opts.SunshineURL == "" {
		opts.SunshineURL = defaultSunshineURL
	}
	u, err := url.Parse(opts.SunshineURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid Sunshine URL %q", opts.SunshineURL)
	}
	s := &Server{
		version:     opts.Version,
		dataDir:     opts.DataDir,
		token:       opts.Token,
		sunshineURL: strings.TrimRight(opts.SunshineURL, "/"),
		doneCh:      make(chan struct{}),
		hub:         newHub(),
	}
	return s, nil
}

// SetRedis attaches a connected Redis client and loads the settings schema.
func (s *Server) SetRedis(client *redis.Client) error {
	s.mu.Lock()
	s.rdb = client
	s.mu.Unlock()
	if err := s.reloadSchema(); err != nil {
		// Not fatal: settings editing degrades, everything else works.
		log.Printf("Settings schema not available yet: %v", err)
	}
	return nil
}

// reloadSchema re-reads settings:schema from Redis.
func (s *Server) reloadSchema() error {
	client := s.getRedis()
	if client == nil {
		return errors.New("redis not connected")
	}
	raw, err := client.Get("settings:schema")
	if err != nil {
		return fmt.Errorf("read settings:schema: %w", err)
	}
	sch, err := schema.Parse([]byte(raw))
	if err != nil {
		return fmt.Errorf("parse settings:schema: %w", err)
	}
	s.mu.Lock()
	s.schema = sch
	s.mu.Unlock()
	return nil
}

// getRedis returns the current client, or nil when not connected.
func (s *Server) getRedis() *redis.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rdb
}

// getSchema returns the cached settings schema, or nil.
func (s *Server) getSchema() *schema.Schema {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.schema
}

// ListenAndServe binds addr and serves until Shutdown. If the address cannot
// be bound, usually because usb0 is not up yet, it retries every 5 seconds
// rather than exiting: the daemon is expected to be available exactly when
// the management network comes back.
func (s *Server) ListenAndServe(addr string) error {
	var ln net.Listener
	logged := false
	for {
		var err error
		ln, err = net.Listen("tcp", addr)
		if err == nil {
			break
		}
		if !logged {
			log.Printf("Cannot bind %s yet (%v); retrying every 5s", addr, err)
			logged = true
		}
		select {
		case <-s.doneCh:
			return nil
		case <-time.After(5 * time.Second):
		}
	}
	log.Printf("Listening on http://%s", addr)

	go s.runStreamBridge()

	srv := &http.Server{
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	s.mu.Lock()
	s.httpServer = srv
	stopping := s.stopping
	s.mu.Unlock()
	if stopping {
		_ = ln.Close()
		return nil
	}
	err := srv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown stops the HTTP server and closes Redis. Safe to call twice.
func (s *Server) Shutdown() {
	s.mu.Lock()
	if s.stopping {
		s.mu.Unlock()
		return
	}
	s.stopping = true
	client := s.rdb
	srv := s.httpServer
	s.mu.Unlock()

	close(s.doneCh)
	s.hub.close()
	if srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = srv.Shutdown(ctx)
		cancel()
	}
	if client != nil {
		_ = client.Close()
	}
}

// routes builds the handler tree.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	// The UI routes itself with #hash fragments, so every non-API path is
	// the same document and no SPA fallback is needed.
	if sub, err := fs.Sub(staticFiles, "static"); err == nil {
		mux.Handle("/static/", http.StripPrefix("/static/", s.cacheable(http.FileServer(http.FS(sub)))))
		if indexHTML, e := staticFiles.ReadFile("static/index.html"); e == nil {
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("Cache-Control", "no-cache")
				_, _ = w.Write(indexHTML)
			})
		}
	}

	api := func(pattern string, h http.HandlerFunc) {
		mux.HandleFunc(pattern, s.auth(s.guard(h)))
	}

	api("/api/info", s.handleInfo)
	api("/api/status", s.handleStatus)
	api("/api/stream", s.handleStream)
	api("/api/faults", s.handleFaults)
	api("/api/events", s.handleEvents)

	api("/api/settings/schema", s.handleSettingsSchema)
	api("/api/settings", s.handleSettings)
	api("/api/settings/set", s.handleSettingsSet)

	api("/api/control", s.handleControl)

	api("/api/files", s.handleFiles)
	api("/api/files/mkdir", s.handleFilesMkdir)
	api("/files/", s.handleFileDownload)

	api("/api/services", s.handleServices)
	api("/api/services/action", s.handleServiceAction)

	api("/api/updates", s.handleUpdates)
	api("/api/updates/upload", s.handleUpdatesUpload)
	api("/api/updates/action", s.handleUpdatesAction)
	api("/api/system/logs", s.handleLogBundles)
	api("/api/system/journal", s.handleJournal)

	api("/api/navigation", s.handleNavigation)
	api("/api/navigation/locations", s.handleLocations)

	api("/api/keycards", s.handleKeycards)
	api("/api/keycards/command", s.handleKeycardCommand)

	api("/api/cloud", s.handleCloudStatus)
	api("/api/cloud/bootstrap", s.handleCloudBootstrap)
	api("/api/cloud/config", s.handleCloudConfig)

	return s.logRequests(mux)
}

// cacheable adds validators to the embedded assets. Files in an embed.FS
// carry no modification time, so without this browsers would either refetch
// everything or, worse, keep a stale UI across a firmware update. The ETag
// is a digest of the embedded files: any change to the UI invalidates every
// asset at once, including between dev builds that share a version string.
func (s *Server) cacheable(next http.Handler) http.Handler {
	etag := `"` + staticDigest() + `"`
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}

// staticDigest hashes every embedded asset in path order.
func staticDigest() string {
	h := sha256.New()
	_ = fs.WalkDir(staticFiles, "static", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := staticFiles.ReadFile(path)
		if err != nil {
			return err
		}
		h.Write([]byte(path))
		h.Write(b)
		return nil
	})
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// handleInfo answers GET /api/info with daemon metadata.
func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":      s.version,
		"data-dir":     s.dataDir,
		"sunshine-url": s.sunshineURL,
		"auth":         s.token != "",
		"redis-ok":     s.getRedis() != nil,
	})
}

// statusWriter records the response code for the request log.
type statusWriter struct {
	http.ResponseWriter
	code int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.code = code
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Flush() {
	if f, ok := sw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// logRequests emits one journal line per request that changes something or
// fails. Successful reads are not logged: the dashboard reads constantly,
// and a journal that scrolls at one line per second hides everything else.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/stream") {
			next.ServeHTTP(w, r)
			return
		}
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(sw, r)
		if r.Method == http.MethodGet && sw.code < 400 {
			return
		}
		log.Printf("%s %s %s %d (%s)", r.RemoteAddr, r.Method, r.URL.Path, sw.code, time.Since(start).Round(time.Millisecond))
	})
}

// auth enforces the optional bearer token.
func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.token != "" {
			got := ""
			if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
				got = strings.TrimPrefix(h, "Bearer ")
			} else {
				got = r.URL.Query().Get("token")
			}
			if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
				w.Header().Set("WWW-Authenticate", `Bearer realm="lsd"`)
				writeErr(w, http.StatusUnauthorized, "unauthorized")
				return
			}
		}
		next(w, r)
	}
}

// guard rejects state-changing requests that did not originate from this
// page. A laptop on usb0 has lsd at a fixed, guessable address, so any site
// open in another tab could otherwise POST an unlock. Browsers attach Origin
// (and Sec-Fetch-Site) to cross-site requests; tools like curl send neither
// and pass.
func (s *Server) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if !sameOrigin(r) {
				writeErr(w, http.StatusForbidden, "cross-origin request rejected")
				return
			}
		}
		next(w, r)
	}
}

// sameOrigin reports whether the request's Origin (if any) names this host.
func sameOrigin(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" || origin == "null" {
		return origin == ""
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// writeJSON is the standard JSON success response.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// writeErr is the standard JSON error response.
func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// readJSON decodes a small JSON request body into v.
func readJSON(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "bad JSON: "+err.Error())
		return false
	}
	return true
}

func (s *Server) isStopping() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stopping
}

// done reports whether the server is shutting down.
func (s *Server) done() <-chan struct{} {
	return s.doneCh
}

// method ensures a handler only answers one HTTP method.
func method(w http.ResponseWriter, r *http.Request, want string) bool {
	if r.Method != want {
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return false
	}
	return true
}
