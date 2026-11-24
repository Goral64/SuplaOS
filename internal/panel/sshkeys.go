package panel

import (
	"net/http"

	"github.com/Goral64/SuplaOS/internal/system"
)

func (s *Server) getSSHKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := system.SSHKeys()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

func (s *Server) addSSHKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key string `json:"key"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	k, err := system.AddSSHKey(req.Key)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, k)
}

// deleteSSHKey takes the fingerprint in the body: it has "/" and "+".
func (s *Server) deleteSSHKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Fingerprint string `json:"fingerprint"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	if err := system.DeleteSSHKey(req.Fingerprint); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

// getSSHPassword tells whether SSH takes the password or only keys.
func (s *Server) getSSHPassword(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"enabled": system.SSHPasswordLogin()})
}

func (s *Server) putSSHPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	if err := system.SetSSHPasswordLogin(req.Enabled); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	s.getSSHPassword(w, r)
}
