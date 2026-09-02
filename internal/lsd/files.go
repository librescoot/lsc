package lsd

import (
	"archive/tar"
	"errors"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// maxUpload bounds a single upload so a stray tab cannot fill /data.
const maxUpload = 2 << 30 // 2 GiB

// fileEntry is one row in the browser listing.
type fileEntry struct {
	Name  string `json:"name"`
	Dir   bool   `json:"dir,omitempty"`
	Size  int64  `json:"size,omitempty"`
	Mode  string `json:"mode,omitempty"`
	MTime int64  `json:"mtime,omitempty"`
}

// cleanRelPath resolves a user-supplied path to a slash path relative to
// the data dir. Prefixing "/" before Clean collapses any traversal ("../x"
// becomes "x"), so the result is always inside the data dir. The empty
// result means "the data dir itself".
func (s *Server) cleanRelPath(name string) string {
	name = strings.TrimPrefix(path.Clean("/"+strings.TrimPrefix(name, "/")), "/")
	if name == "." {
		return ""
	}
	return name
}

// fullDataPath maps a request path to an absolute filesystem path.
func (s *Server) fullDataPath(name string) string {
	return filepath.Join(s.dataDir, filepath.FromSlash(s.cleanRelPath(name)))
}

// handleFiles implements GET (listing), PUT (upload) and DELETE on /data.
func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.filesList(w, r)
	case http.MethodPut:
		s.filesUpload(w, r)
	case http.MethodDelete:
		s.filesDelete(w, r)
	default:
		writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// filesList returns the direct children of a directory.
func (s *Server) filesList(w http.ResponseWriter, r *http.Request) {
	rel := s.cleanRelPath(r.URL.Query().Get("path"))
	full := s.fullDataPath(rel)
	fi, err := os.Stat(full)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no such path")
		return
	}
	if !fi.IsDir() {
		writeErr(w, http.StatusBadRequest, "not a directory")
		return
	}
	dirEntries, err := os.ReadDir(full)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]fileEntry, 0, len(dirEntries))
	for _, de := range dirEntries {
		entry := fileEntry{Name: de.Name(), Dir: de.IsDir()}
		if info, err := de.Info(); err == nil {
			entry.Size = info.Size()
			entry.MTime = info.ModTime().Unix()
			entry.Mode = info.Mode().String()
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return out[i].Name < out[j].Name
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":    rel,
		"entries": out,
	})
}

// filesUpload accepts a full-file upload and writes it atomically:
// temporary file in the target directory, fsync, rename, parent fsync,
// the same durability sequence data-server uses.
func (s *Server) filesUpload(w http.ResponseWriter, r *http.Request) {
	full := s.fullDataPath(r.URL.Query().Get("path"))
	if full == filepath.Clean(s.dataDir) {
		writeErr(w, http.StatusBadRequest, "refusing to overwrite the data directory")
		return
	}
	if fi, err := os.Stat(full); err == nil && fi.IsDir() {
		writeErr(w, http.StatusBadRequest, "target is a directory")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	if err := writeFileAtomic(full, r.Body, 0o644); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeErr(w, http.StatusRequestEntityTooLarge, "upload exceeds 2 GiB")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "uploaded", "path": s.cleanRelPath(r.URL.Query().Get("path"))})
}

// writeFileAtomic streams src into a temporary file next to dst, fsyncs,
// renames it over dst and fsyncs the directory. Callers see either the old
// file or the complete new one, never a partial write.
func writeFileAtomic(dst string, src io.Reader, perm os.FileMode) error {
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".lsd-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func(err error) error {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if _, err := io.Copy(tmp, src); err != nil {
		return cleanup(err)
	}
	if err := tmp.Chmod(perm); err != nil {
		return cleanup(err)
	}
	if err := tmp.Sync(); err != nil {
		return cleanup(err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if dirf, err := os.Open(dir); err == nil {
		_ = dirf.Sync()
		_ = dirf.Close()
	}
	return nil
}

// filesDelete removes a file, or a directory tree when recursive=1.
func (s *Server) filesDelete(w http.ResponseWriter, r *http.Request) {
	full := s.fullDataPath(r.URL.Query().Get("path"))
	if full == filepath.Clean(s.dataDir) {
		writeErr(w, http.StatusBadRequest, "refusing to delete the data directory")
		return
	}
	fi, err := os.Stat(full)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no such path")
		return
	}
	if fi.IsDir() {
		if r.URL.Query().Get("recursive") != "1" {
			writeErr(w, http.StatusBadRequest, "directory: pass recursive=1 to delete")
			return
		}
		if err := os.RemoveAll(full); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else if err := os.Remove(full); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleFilesMkdir creates a directory (parents included).
func (s *Server) handleFilesMkdir(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodPost) {
		return
	}
	var req struct {
		Path string `json:"path"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if s.cleanRelPath(req.Path) == "" {
		writeErr(w, http.StatusBadRequest, "folder name required")
		return
	}
	if err := os.MkdirAll(s.fullDataPath(req.Path), 0o755); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "created"})
}

// handleFileDownload streams a file, with a download-friendly
// Content-Disposition when ?download=1 is set. Directories are only served
// as downloads, streamed as a tar archive.
func (s *Server) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	if !method(w, r, http.MethodGet) {
		return
	}
	rel := strings.TrimPrefix(r.URL.Path, "/files/")
	full := s.fullDataPath(rel)
	fi, err := os.Stat(full)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no such path")
		return
	}

	name := filepath.Base(full)
	if fi.IsDir() {
		if r.URL.Query().Get("download") != "1" {
			writeErr(w, http.StatusBadRequest, "not a file")
			return
		}
		w.Header().Set("Content-Type", "application/x-tar")
		w.Header().Set("Content-Disposition", contentDisposition(name+".tar"))
		if err := writeTarDir(w, full); err != nil {
			// Headers are already out; the truncated archive tells the story.
			log.Printf("tar of %s failed: %v", full, err)
		}
		return
	}

	f, err := os.Open(full)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer f.Close()

	if r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", contentDisposition(name))
	}
	// http.ServeContent adds size, modification time and range support.
	http.ServeContent(w, r, name, fi.ModTime(), f)
}

// contentDisposition builds an attachment header value. Filenames from /data
// are simple ASCII in practice; quote defensively anyway.
func contentDisposition(name string) string {
	return "attachment; filename=\"" + strings.ReplaceAll(name, "\"", "_") + "\""
}

// writeTarDir streams dir as a tar archive. Symlinks are skipped: /data
// should not contain any, and a tar full of absolute-link escapes is exactly
// the kind of surprise a management interface must not produce.
func writeTarDir(w io.Writer, dir string) error {
	tw := tar.NewWriter(w)
	defer tw.Close()

	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if hdr.Name == "." {
			return nil
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
}
