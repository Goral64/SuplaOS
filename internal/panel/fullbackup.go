package panel

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goral64/SuplaOS/internal/stack"
	"github.com/Goral64/SuplaOS/internal/system"
)

// createFullBackup makes a full backup of the device in the backup
// directory, encrypted with the password when one is given.
func (s *Server) createFullBackup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	s.stackJob(w, "full backup", func(st *stack.Stack, j *Job) error {
		path, err := st.SaveFullBackup(stack.ExportOptions{Version: system.Version(), Password: req.Password})
		if err != nil {
			return err
		}
		j.Logf("Backup saved: %s", filepath.Base(path))
		return nil
	})
}

// uploadFullBackup stores a full backup sent from the browser in the
// backup directory, under its own name when that is free, and returns the
// name.
func (s *Server) uploadFullBackup(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.URL.Query().Get("name"))
	ext := ".tar.gz"
	if strings.HasSuffix(name, ".age") {
		ext += ".age"
	}
	if !strings.HasSuffix(name, ".tar.gz") && !strings.HasSuffix(name, ".tar.gz.age") {
		jsonError(w, http.StatusBadRequest, errors.New("a full backup is a .tar.gz or .tar.gz.age file"))
		return
	}
	if !stack.IsFullBackup(name) || strings.HasPrefix(name, ".") {
		name = "suplaos-upload-" + time.Now().Format("20060102150405") + ext
	}
	dir := filepath.Join(s.cfg.Root, "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	f, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	defer os.Remove(f.Name())
	_, err = io.Copy(f, http.MaxBytesReader(w, r.Body, maxBackup))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	os.Chmod(f.Name(), 0o600)
	base := strings.TrimSuffix(name, ext)
	for n := 1; ; n++ {
		if _, err := os.Stat(filepath.Join(dir, name)); errors.Is(err, os.ErrNotExist) {
			break
		}
		name = fmt.Sprintf("%s-%d%s", base, n, ext)
	}
	if err := os.Rename(f.Name(), filepath.Join(dir, name)); err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": name})
}

type fullBackupRequest struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

// fullBackup reads the request naming a full backup in the backup
// directory.
func (s *Server) fullBackup(w http.ResponseWriter, r *http.Request) (string, *fullBackupRequest, bool) {
	var req fullBackupRequest
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return "", nil, false
	}
	p, err := s.backupPath(req.Name)
	if err == nil && !stack.IsFullBackup(req.Name) {
		err = errors.New("not a full backup")
	}
	if err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return "", nil, false
	}
	return p, &req, true
}

// checkFullBackup reads a full backup through and returns its manifest,
// to be shown before it is restored.
func (s *Server) checkFullBackup(w http.ResponseWriter, r *http.Request) {
	p, req, ok := s.fullBackup(w, r)
	if !ok {
		return
	}
	st, err := s.openStack()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	m, err := st.CheckFullBackup(p, stack.ImportOptions{Version: system.Version(), Password: req.Password})
	if err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// restoreFullBackup replaces the add-ons and their data with a full
// backup. The settings of the device stay as they are; the setup is done
// afterwards, the backup comes from a device that was set up.
func (s *Server) restoreFullBackup(w http.ResponseWriter, r *http.Request) {
	p, req, ok := s.fullBackup(w, r)
	if !ok {
		return
	}
	s.stackJob(w, "restore full backup", func(st *stack.Stack, j *Job) error {
		if err := st.Import(p, stack.ImportOptions{Version: system.Version(), Password: req.Password}); err != nil {
			return err
		}
		j.Logf("Restored from %s", req.Name)
		// The certificate of the panel for the CA of the backup, once the
		// job lets go of the lock.
		go func() {
			time.Sleep(2 * time.Second)
			if err := s.renewCertificates(false); err != nil {
				log.Printf("certificate after restore: %v", err)
			}
		}()
		return nil
	})
}
