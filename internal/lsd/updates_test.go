package lsd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBoardFromFile(t *testing.T) {
	for _, tc := range []struct {
		name  string
		board string
	}{
		{"librescoot-unu-mdb-v1.3.1.mender", "mdb"},
		{"librescoot-unu-dbc-nightly-20260927T163533.delta", "dbc"},
		{"update-mdb-v1.mender", ""},
		{"librescoot-unu-mdb-.mender", ""},
		{"librescoot-unu-mdb-v1.mender.tmp", ""},
	} {
		if got := boardFromFile(tc.name); got != tc.board {
			t.Errorf("boardFromFile(%q) = %q; want %q", tc.name, got, tc.board)
		}
	}
}

func TestUpdatesUploadBoardSelection(t *testing.T) {
	s := &Server{dataDir: t.TempDir()}
	put := func(board, name string) *httptest.ResponseRecorder {
		t.Helper()
		url := "/api/updates/upload?name=" + name
		if board != "" {
			url += "&board=" + board
		}
		response := httptest.NewRecorder()
		s.handleUpdatesUpload(response, httptest.NewRequest(http.MethodPut, url, strings.NewReader("artifact")))
		return response
	}
	name := "librescoot-unu-dbc-v1.3.1.mender"
	if response := put("", name); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"board":"dbc"`) {
		t.Fatalf("detected DBC: %d %s", response.Code, response.Body.String())
	}
	if data, err := os.ReadFile(filepath.Join(s.otaDir(), "dbc", name)); err != nil || string(data) != "artifact" {
		t.Fatalf("DBC artifact = %q, %v", data, err)
	}
	if response := put("mdb", name); response.Code != http.StatusBadRequest {
		t.Fatalf("conflicting board: %d %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(filepath.Join(s.otaDir(), "mdb", name)); !os.IsNotExist(err) {
		t.Fatalf("conflicting upload wrote MDB artifact: %v", err)
	}
	if response := put("", "custom.delta"); response.Code != http.StatusBadRequest {
		t.Fatalf("unrecognized board: %d %s", response.Code, response.Body.String())
	}
	if response := put("mdb", "custom.delta"); response.Code != http.StatusOK {
		t.Fatalf("explicit board: %d %s", response.Code, response.Body.String())
	}
}
