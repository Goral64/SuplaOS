package panel

import (
	"net/http"

	"github.com/Goral64/SuplaOS/internal/system"
)

// HotspotSettings are the choices of the panel for the hotspot watcher.
func (s *Server) HotspotSettings() system.HotspotSettings {
	st := s.getSettings()
	return system.HotspotSettings{SetupDone: st.SetupDone, Off: st.HotspotOff}
}

// getHotspot shows the Wi-Fi network the device makes when it has none:
// its name, password and whether it is turned off.
func (s *Server) getHotspot(w http.ResponseWriter, r *http.Request) {
	wf, err := system.ReadWifi()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	if wf.Device == "" {
		writeJSON(w, http.StatusOK, map[string]any{"device": ""})
		return
	}
	set := s.HotspotSettings()
	password := system.HotspotPassword
	if set.SetupDone {
		if password, err = system.DeviceHotspotPassword(); err != nil {
			jsonError(w, http.StatusInternalServerError, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"device": wf.Device, "ssid": system.HotspotSSID(), "password": password,
		"off": set.SetupDone && set.Off, "active": wf.Hotspot != "", "setupDone": set.SetupDone,
	})
}

func (s *Server) putHotspot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Off bool `json:"off"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.saveSettings(func(st *Settings) { st.HotspotOff = req.Off }); err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	s.getHotspot(w, r)
}

// newHotspotPassword makes a new password of the device. A hotspot that is
// on keeps the old one until it starts again.
func (s *Server) newHotspotPassword(w http.ResponseWriter, r *http.Request) {
	if _, err := system.NewHotspotPassword(); err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	s.getHotspot(w, r)
}
