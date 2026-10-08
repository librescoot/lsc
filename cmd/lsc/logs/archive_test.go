package logs

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestArchivePublication(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "journal.log"), []byte("hello\n"), 0600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "logs-test.tar.gz")
	if err := createTarball(source, "logs-test", dest); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	h, err := tr.Next()
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(tr)
	if err != nil {
		t.Fatal(err)
	}
	if h.Name != "logs-test/journal.log" || string(data) != "hello\n" {
		t.Fatalf("unexpected archive: %s %q", h.Name, data)
	}
	if _, err := tr.Next(); err != io.EOF {
		t.Fatalf("tar footer: %v", err)
	}
	if _, err := io.Copy(io.Discard, gz); err != nil {
		t.Fatalf("gzip footer: %v", err)
	}
	parts, err := filepath.Glob(filepath.Join(root, ".logs-*.part"))
	if err != nil || len(parts) != 0 {
		t.Fatalf("left staging files: %v %v", parts, err)
	}
}

func TestFailedArchiveKeepsPublishedPackage(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "logs-test.tar.gz")
	if err := os.WriteFile(dest, []byte("existing package"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := createTarball(filepath.Join(root, "absent"), "logs-test", dest); err == nil {
		t.Fatal("missing input succeeded")
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "existing package" {
		t.Fatalf("published file replaced: %q %v", data, err)
	}
	parts, _ := filepath.Glob(filepath.Join(root, ".logs-*.part"))
	if len(parts) != 0 {
		t.Fatalf("left staging files: %v", parts)
	}
}
