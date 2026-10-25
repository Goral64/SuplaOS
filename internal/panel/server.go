// Package panel is the web panel of SuplaOS: a JSON API over the stack and
// system packages and the embedded web interface.
package panel

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"

	"gopkg.in/yaml.v3"

	"github.com/Goral64/SuplaOS/internal/acme"
	"github.com/Goral64/SuplaOS/internal/stack"
)

type Config struct {
	// Root and Addons as in the suplaos command.
	Root, Addons string
	// Settings is the panel settings file.
	Settings string
	// Sessions keeps the login sessions across restarts; empty: memory only.
	Sessions string
	// Web is the built web interface.
	Web fs.FS
}

// Settings are kept in Config.Settings.
type Settings struct {
	Lang      string `yaml:"lang" json:"lang"`
	SetupDone bool   `yaml:"setup_done" json:"setupDone"`
}

type Server struct {
	cfg      Config
	sessions sessions
	jobs     jobs
	images   imageState
	cert     atomic.Pointer[tls.Certificate]
	// domainCert is the Let's Encrypt certificate of the domain set in
	// the panel, served when a browser asks for that name.
	domainCert atomic.Pointer[tls.Certificate]
	acme       *acme.Manager
	proxies    proxies

	mu       sync.Mutex
	settings Settings
}

func New(cfg Config) *Server {
	s := &Server{cfg: cfg, acme: acme.New(cfg.Root)}
	s.sessions.load(cfg.Sessions)
	if data, err := os.ReadFile(cfg.Settings); err == nil {
		if err := yaml.Unmarshal(data, &s.settings); err != nil {
			log.Printf("%s: %v", cfg.Settings, err)
		}
	}
	return s
}

func (s *Server) getSettings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settings
}

func (s *Server) saveSettings(change func(*Settings)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.settings
	change(&next)
	data, err := yaml.Marshal(next)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(path.Dir(s.cfg.Settings), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(s.cfg.Settings, data, 0o644); err != nil {
		return err
	}
	s.settings = next
	return nil
}

// openStack reads the catalog and the state anew for every request, so
// that changes made with the suplaos command show up.
func (s *Server) openStack() (*stack.Stack, error) {
	return stack.Open(s.cfg.Root, s.cfg.Addons)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	api := func(pattern string, h http.HandlerFunc) {
		mux.Handle(pattern, s.auth(h))
	}
	mux.HandleFunc("GET /api/session", s.getSession)
	mux.HandleFunc("POST /api/login", s.login)
	mux.HandleFunc("POST /api/logout", s.logout)
	mux.HandleFunc("GET /api/ca.crt", s.getCA)
	api("GET /api/ca", s.getCAInfo)
	api("GET /api/domain", s.getDomain)
	api("PUT /api/domain", s.putDomain)
	api("POST /api/domain/renew", s.renewDomainNow)
	api("POST /api/domain/addresses", s.useDomain)
	api("DELETE /api/domain", s.deleteDomain)
	api("POST /api/password", s.changePassword)

	api("GET /api/system", s.getSystem)
	api("POST /api/system/{action}", s.power)
	api("GET /api/settings", s.getSettingsAPI)
	api("PUT /api/settings", s.putSettings)
	api("GET /api/timezones", s.getTimezones)
	api("GET /api/network", s.getNetwork)
	api("PUT /api/network", s.putNetwork)
	api("GET /api/wifi", s.getWifi)
	api("POST /api/wifi/scan", s.scanWifi)
	api("POST /api/wifi/connect", s.connectWifi)
	api("DELETE /api/wifi/{name}", s.forgetWifi)
	api("GET /api/serial", s.getSerial)

	api("GET /api/addons", s.getAddons)
	api("POST /api/addons/{id}/install", s.installAddon)
	api("PUT /api/addons/{id}/config", s.configAddon)
	api("POST /api/addons/{id}/remove", s.removeAddon)
	api("POST /api/addons/{id}/restore", s.restoreAddon)
	api("POST /api/addons/{id}/backups", s.uploadBackup)
	api("POST /api/addons/{id}/{action}", s.addonAction)
	api("GET /api/addons/{id}/logs", s.addonLogs)
	api("GET /api/addons/{id}/downloads/{n}", s.downloadFile)
	api("GET /api/supla/users", s.suplaUsers)
	api("POST /api/supla/users", s.createSuplaUser)
	api("POST /api/supla-scripts/oauth", s.connectScripts)

	api("POST /api/addons/apply", s.applyAddons)

	api("GET /api/updates", s.getUpdates)
	api("POST /api/updates/check", s.checkUpdates)
	api("POST /api/updates/upgrade", s.startUpgrade)
	api("GET /api/updates/log", s.upgradeLog)
	api("PUT /api/updates/auto", s.setAutoUpdates)

	api("GET /api/backups", s.getBackups)
	api("GET /api/backups/{name}", s.downloadBackup)
	api("DELETE /api/backups/{name}", s.deleteBackup)

	api("GET /api/jobs/current", s.currentJob)
	api("GET /api/jobs/{id}", s.getJob)
	api("GET /api/jobs/{id}/events", s.jobEvents)

	mux.Handle("/", s.static())
	return mux
}

// auth lets through requests of logged-in users. Changes also need the
// X-SuplaOS header, which other sites cannot send without CORS.
func (s *Server) auth(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ses := s.sessions.get(r)
		if ses == nil {
			jsonError(w, http.StatusUnauthorized, errors.New("not logged in"))
			return
		}
		if r.Method != http.MethodGet && r.Header.Get("X-SuplaOS") == "" {
			jsonError(w, http.StatusForbidden, errors.New("missing X-SuplaOS header"))
			return
		}
		if ses.MustChange && r.URL.Path != "/api/password" && r.URL.Path != "/api/settings" {
			jsonError(w, http.StatusForbidden, errors.New("change the default password first"))
			return
		}
		h(w, r)
	})
}

// static serves the web interface; unknown paths get index.html, the
// interface routes them itself.
func (s *Server) static() http.Handler {
	files := http.FileServerFS(s.cfg.Web)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(s.cfg.Web, p); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func readJSON(r *http.Request, v any) error {
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(v)
}
