package lsd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	shellMaxCmd    = 8 << 10
	shellMaxOutput = 4 << 20
	shellTimeout   = 10 * time.Minute
)

// shellRun tracks one running command so a second request can signal it: the
// output stream occupies the request that started it.
type shellRun struct {
	pgid int
}

type shellRegistry struct {
	mu   sync.Mutex
	runs map[string]*shellRun
}

func newShellRegistry() *shellRegistry { return &shellRegistry{runs: map[string]*shellRun{}} }

func (reg *shellRegistry) add(id string, run *shellRun) {
	reg.mu.Lock()
	reg.runs[id] = run
	reg.mu.Unlock()
}

func (reg *shellRegistry) remove(id string) {
	reg.mu.Lock()
	delete(reg.runs, id)
	reg.mu.Unlock()
}

func (reg *shellRegistry) get(id string) *shellRun {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	return reg.runs[id]
}

// shellScript wraps the user's command so the shell starts in the client's
// working directory and reports where it ended up. The trailing pwd goes to
// fd 3, a pipe of ours, so it can never be confused with the command's own
// output.
const shellScript = `cd "$LSD_CWD" 2>/dev/null || cd /
%s
__lsd_rc=$?
pwd >&3
exit $__lsd_rc
`

// shellAllowed gates both shell endpoints. s.guard already rejects requests a
// browser marks as cross-site, but it lets through requests with no Origin at
// all, which is how curl and every pre-Fetch-metadata browser look. A root
// shell deserves the stricter test, so these two endpoints also demand a JSON
// content type and a header of our own: an HTML form can send neither, and a
// script that sets either one triggers a preflight lsd never answers. That
// leaves the endpoints reachable from this page and from a deliberate curl,
// and unreachable from a page the user merely happens to have open.
func (s *Server) shellAllowed(w http.ResponseWriter, r *http.Request) bool {
	if !s.shell {
		writeErr(w, http.StatusForbidden, "shell is disabled")
		return false
	}
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		writeErr(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return false
	}
	if r.Header.Get("X-Lsd-Shell") != "1" {
		writeErr(w, http.StatusForbidden, "missing X-Lsd-Shell header")
		return false
	}
	// The browser tells us what kind of request this is; anything but a
	// same-origin fetch from a page has no business here.
	switch r.Header.Get("Sec-Fetch-Mode") {
	case "", "cors", "same-origin":
	default:
		writeErr(w, http.StatusForbidden, "cross-origin request rejected")
		return false
	}
	if dest := r.Header.Get("Sec-Fetch-Dest"); dest != "" && dest != "empty" {
		writeErr(w, http.StatusForbidden, "cross-origin request rejected")
		return false
	}
	return true
}

// handleShell runs one command and streams its output as newline-delimited
// JSON frames: {"o":...} stdout, {"e":...} stderr, {"x":code,"cwd":...} last.
// Each request is its own shell, so state other than the working directory
// (variables, shell options) does not carry over, like Sunshine's admin shell.
func (s *Server) handleShell(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	if !s.shellAllowed(w, r) {
		return
	}
	var req struct {
		ID  string `json:"id"`
		Cmd string `json:"cmd"`
		Cwd string `json:"cwd"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if req.Cmd == "" {
		writeErr(w, http.StatusBadRequest, "cmd is required")
		return
	}
	if len(req.Cmd) > shellMaxCmd {
		writeErr(w, http.StatusBadRequest, "command too long")
		return
	}
	if req.Cwd == "" {
		req.Cwd = s.dataDir
	}
	// The console is a root shell: leave a trail in the journal.
	log.Printf("shell: %s (cwd %s)", req.Cmd, req.Cwd)

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), shellTimeout)
	defer cancel()

	cwdRead, cwdWrite, err := os.Pipe()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer cwdRead.Close()

	cmd := exec.CommandContext(ctx, "sh", "-c", fmt.Sprintf(shellScript, req.Cmd))
	cmd.Env = append(os.Environ(), "LSD_CWD="+req.Cwd, "TERM=dumb", "PAGER=cat", "NO_COLOR=1")
	cmd.Stdin = nil
	cmd.ExtraFiles = []*os.File{cwdWrite}
	// Own process group: a signal then reaches the whole pipeline, not just
	// the shell, and the shell's children die with it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cwdWrite.Close()
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cwdWrite.Close()
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := cmd.Start(); err != nil {
		cwdWrite.Close()
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	// The child holds its own copy; ours has to go or the read below blocks
	// until the whole process group exits.
	cwdWrite.Close()

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	run := &shellRun{pgid: cmd.Process.Pid}
	if req.ID != "" {
		s.shells.add(req.ID, run)
		defer s.shells.remove(req.ID)
	}

	var (
		mu       sync.Mutex
		written  int
		overflow bool
	)
	emit := func(frame map[string]interface{}) {
		mu.Lock()
		defer mu.Unlock()
		b, err := json.Marshal(frame)
		if err != nil {
			return
		}
		_, _ = w.Write(append(b, '\n'))
		flusher.Flush()
	}
	pump := func(key string, rc io.Reader, wg *sync.WaitGroup) {
		defer wg.Done()
		buf := make([]byte, 8<<10)
		for {
			n, err := rc.Read(buf)
			if n > 0 {
				mu.Lock()
				room := shellMaxOutput - written
				if room <= 0 {
					overflow = true
					mu.Unlock()
					_ = syscall.Kill(-run.pgid, syscall.SIGKILL)
					return
				}
				chunk := buf[:n]
				if n > room {
					chunk = buf[:room]
					overflow = true
				}
				written += len(chunk)
				mu.Unlock()
				emit(map[string]interface{}{key: string(chunk)})
			}
			if err != nil {
				return
			}
		}
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go pump("o", stdout, &wg)
	go pump("e", stderr, &wg)
	wg.Wait()

	waitErr := cmd.Wait()
	code := 0
	if waitErr != nil {
		if ee, ok := waitErr.(*exec.ExitError); ok {
			code = ee.ExitCode()
			// A killed process reports -1; report the shell convention instead.
			if code < 0 {
				if st, ok := ee.Sys().(syscall.WaitStatus); ok && st.Signaled() {
					code = 128 + int(st.Signal())
				}
			}
		} else {
			code = -1
		}
	}
	final := map[string]interface{}{"x": code, "cwd": shellCwd(cwdRead, req.Cwd)}
	if overflow {
		final["trunc"] = true
	}
	if ctx.Err() == context.DeadlineExceeded {
		final["err"] = "command timed out"
	}
	emit(final)
}

// shellCwd reads the working directory the script reported on fd 3, falling
// back to where it started when the command died before printing it.
func shellCwd(r *os.File, fallback string) string {
	line, err := bufio.NewReader(io.LimitReader(r, 4096)).ReadString('\n')
	if err != nil && line == "" {
		return fallback
	}
	if n := len(line); n > 0 && line[n-1] == '\n' {
		line = line[:n-1]
	}
	if line == "" {
		return fallback
	}
	return line
}

// handleShellSignal interrupts or kills a running command. The stream that
// started it stays open and reports the exit code, like a terminal would.
func (s *Server) handleShellSignal(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	if !s.shellAllowed(w, r) {
		return
	}
	var req struct {
		ID     string `json:"id"`
		Signal string `json:"signal"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	run := s.shells.get(req.ID)
	if run == nil {
		writeErr(w, http.StatusNotFound, "no such command")
		return
	}
	sig := syscall.SIGINT
	switch req.Signal {
	case "", "int":
	case "term":
		sig = syscall.SIGTERM
	case "kill":
		sig = syscall.SIGKILL
	default:
		writeErr(w, http.StatusBadRequest, "signal must be int, term or kill")
		return
	}
	if err := syscall.Kill(-run.pgid, sig); err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "signalled"})
}
