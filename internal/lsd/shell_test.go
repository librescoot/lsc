package lsd

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// shellReq builds a request the way the page does: JSON, plus the header the
// shell endpoints demand.
func shellReq(path, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Lsd-Shell", "1")
	return r
}

// runShell posts one command and returns the frames the handler streamed.
func runShell(t *testing.T, s *Server, body string) []map[string]interface{} {
	t.Helper()
	r := shellReq("/api/shell", body)
	w := httptest.NewRecorder()
	s.handleShell(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var frames []map[string]interface{}
	sc := bufio.NewScanner(w.Body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if sc.Text() == "" {
			continue
		}
		var f map[string]interface{}
		if err := json.Unmarshal(sc.Bytes(), &f); err != nil {
			t.Fatalf("bad frame %q: %v", sc.Text(), err)
		}
		frames = append(frames, f)
	}
	return frames
}

func TestShellStreamsOutputAndExitCode(t *testing.T) {
	s := &Server{shell: true, dataDir: "/tmp", shells: newShellRegistry()}
	frames := runShell(t, s, `{"cmd":"echo out; echo err >&2; exit 7","cwd":"/tmp"}`)
	if len(frames) == 0 {
		t.Fatal("no frames")
	}
	var out, errOut string
	for _, f := range frames[:len(frames)-1] {
		if v, ok := f["o"].(string); ok {
			out += v
		}
		if v, ok := f["e"].(string); ok {
			errOut += v
		}
	}
	if !strings.Contains(out, "out") {
		t.Errorf("stdout = %q, want it to contain out", out)
	}
	if !strings.Contains(errOut, "err") {
		t.Errorf("stderr = %q, want it to contain err", errOut)
	}
	last := frames[len(frames)-1]
	if code, _ := last["x"].(float64); code != 7 {
		t.Errorf("exit code = %v, want 7", last["x"])
	}
}

// The working directory is the only thing that carries between commands, and
// it comes back on fd 3 rather than in the output stream.
func TestShellReportsWorkingDirectory(t *testing.T) {
	s := &Server{shell: true, dataDir: "/tmp", shells: newShellRegistry()}
	frames := runShell(t, s, `{"cmd":"cd /; echo done","cwd":"/tmp"}`)
	last := frames[len(frames)-1]
	if last["cwd"] != "/" {
		t.Errorf("cwd = %v, want /", last["cwd"])
	}

	// A command that exits before the trailer leaves the caller where it was.
	frames = runShell(t, s, `{"cmd":"cd /; exit 2","cwd":"/tmp"}`)
	last = frames[len(frames)-1]
	if last["cwd"] != "/tmp" {
		t.Errorf("cwd after early exit = %v, want /tmp", last["cwd"])
	}
}

func TestShellDisabled(t *testing.T) {
	s := &Server{shell: false, dataDir: "/tmp", shells: newShellRegistry()}
	w := httptest.NewRecorder()
	s.handleShell(w, shellReq("/api/shell", `{"cmd":"echo hi"}`))
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
	w = httptest.NewRecorder()
	s.handleShellSignal(w, shellReq("/api/shell/signal", `{"id":"x"}`))
	if w.Code != http.StatusForbidden {
		t.Errorf("signal status = %d, want 403", w.Code)
	}
}

func TestShellRejectsEmptyAndOversizedCommands(t *testing.T) {
	s := &Server{shell: true, dataDir: "/tmp", shells: newShellRegistry()}
	for _, body := range []string{`{"cmd":""}`, `{"cmd":"` + strings.Repeat("x", shellMaxCmd+1) + `"}`} {
		w := httptest.NewRecorder()
		s.handleShell(w, shellReq("/api/shell", body))
		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want 400", w.Code)
		}
	}
}

func TestShellSignalUnknownID(t *testing.T) {
	s := &Server{shell: true, dataDir: "/tmp", shells: newShellRegistry()}
	w := httptest.NewRecorder()
	s.handleShellSignal(w, shellReq("/api/shell/signal", `{"id":"nope","signal":"int"}`))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// A cross-site page can only send a simple request: a form post, or a fetch
// without custom headers. Both must bounce before the command runs.
func TestShellRejectsForgedRequests(t *testing.T) {
	s := &Server{shell: true, dataDir: "/tmp", shells: newShellRegistry()}
	body := `{"cmd":"touch /tmp/lsd-csrf-must-not-exist"}`
	tests := []struct {
		name string
		hdr  map[string]string
		want int
	}{
		{"form post", map[string]string{"Content-Type": "text/plain;charset=UTF-8"}, http.StatusUnsupportedMediaType},
		{"urlencoded form", map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, http.StatusUnsupportedMediaType},
		{"json without our header", map[string]string{"Content-Type": "application/json"}, http.StatusForbidden},
		{"navigation", map[string]string{"Content-Type": "application/json", "X-Lsd-Shell": "1", "Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document"}, http.StatusForbidden},
	}
	for _, tt := range tests {
		r := httptest.NewRequest(http.MethodPost, "/api/shell", strings.NewReader(body))
		for k, v := range tt.hdr {
			r.Header.Set(k, v)
		}
		w := httptest.NewRecorder()
		s.handleShell(w, r)
		if w.Code != tt.want {
			t.Errorf("%s: status = %d, want %d", tt.name, w.Code, tt.want)
		}
	}
}
