package panel

import (
	"errors"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/Goral64/SuplaOS/internal/autobackup"
	"github.com/Goral64/SuplaOS/internal/stack"
	"github.com/Goral64/SuplaOS/internal/system"
)

// autoBackupRequest are the settings from the browser. Passwords left
// empty stay as they are.
type autoBackupRequest struct {
	autobackup.Settings
	Password    string `json:"password"`
	SMBPassword string `json:"smbPassword"`
}

// merge returns the settings of the request on top of the saved ones.
func (req *autoBackupRequest) merge(old *autobackup.Settings) *autobackup.Settings {
	s := req.Settings
	switch {
	case s.Plain:
		s.Password = ""
	case req.Password != "":
		s.Password = req.Password
	default:
		s.Password = old.Password
	}
	s.SMB.Password = req.SMBPassword
	if s.SMB.Password == "" && s.SMB.Share == old.SMB.Share && s.SMB.Username == old.SMB.Username {
		s.SMB.Password = old.SMB.Password
	}
	return &s
}

func (s *Server) getAutoBackup(w http.ResponseWriter, r *http.Request) {
	set, err := autobackup.Load(s.cfg.Root)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	var next *time.Time
	if n := set.Next(time.Now()); !n.IsZero() {
		next = &n
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"settings":       set,
		"hasPassword":    set.Password != "",
		"smbHasPassword": set.SMB.Password != "",
		"status":         autobackup.LoadStatus(s.cfg.Root),
		"next":           next,
	})
}

// readAutoBackup reads and checks settings from the request.
func (s *Server) readAutoBackup(w http.ResponseWriter, r *http.Request) *autobackup.Settings {
	var req autoBackupRequest
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return nil
	}
	old, err := autobackup.Load(s.cfg.Root)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return nil
	}
	set := req.merge(old)
	if err := set.Validate(); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return nil
	}
	return set
}

func (s *Server) putAutoBackup(w http.ResponseWriter, r *http.Request) {
	set := s.readAutoBackup(w, r)
	if set == nil {
		return
	}
	// A time that passed before is not due: no backup right after saving.
	set.Since = time.Now()
	if err := set.Save(s.cfg.Root); err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	s.getAutoBackup(w, r)
}

// testAutoBackup checks that the target of the settings in the request
// (not saved yet) can be written to.
func (s *Server) testAutoBackup(w http.ResponseWriter, r *http.Request) {
	set := s.readAutoBackup(w, r)
	if set == nil {
		return
	}
	if err := set.Test(filepath.Join(s.cfg.Root, "backups")); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

// runAutoBackup makes an automatic backup now, with the saved settings.
func (s *Server) runAutoBackup(w http.ResponseWriter, r *http.Request) {
	j, err := s.startAutoBackup()
	writeJob(w, j, err)
}

func (s *Server) startAutoBackup() (*Job, error) {
	set, err := autobackup.Load(s.cfg.Root)
	if err != nil {
		return nil, err
	}
	return s.startStackJob("automatic backup", func(st *stack.Stack, j *Job) error {
		status := set.Run(st, system.Version(), j)
		if !status.OK {
			return errors.New(status.Error)
		}
		return nil
	})
}

// WatchBackups makes the automatic backups when they are due; it does not
// return. While another job runs it tries again a minute later.
func (s *Server) WatchBackups() {
	for {
		time.Sleep(time.Minute)
		set, err := autobackup.Load(s.cfg.Root)
		if err != nil {
			log.Printf("automatic backup: %v", err)
			continue
		}
		if !set.Due(time.Now(), autobackup.LoadStatus(s.cfg.Root)) {
			continue
		}
		var busy errBusy
		if _, err := s.startAutoBackup(); err != nil && !errors.As(err, &busy) {
			log.Printf("automatic backup: %v", err)
		}
	}
}
