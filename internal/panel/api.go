package panel

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Goral64/SuplaOS/internal/addon"
	"github.com/Goral64/SuplaOS/internal/stack"
	"github.com/Goral64/SuplaOS/internal/system"
)

// --- Session ---------------------------------------------------------------

func (s *Server) getSession(w http.ResponseWriter, r *http.Request) {
	ses := s.sessions.get(r)
	st := s.getSettings()
	writeJSON(w, http.StatusOK, map[string]any{
		"loggedIn":   ses != nil,
		"mustChange": ses != nil && ses.MustChange,
		"lang":       st.Lang,
		"setupDone":  st.SetupDone,
		"user":       system.User,
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct{ Password string }
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	if s.sessions.throttled() {
		jsonError(w, http.StatusTooManyRequests, errors.New("too many attempts, wait a moment"))
		return
	}
	ok, err := system.CheckPassword(req.Password)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	s.sessions.result(ok)
	if !ok {
		jsonError(w, http.StatusUnauthorized, errors.New("wrong password"))
		return
	}
	mustChange := req.Password == system.DefaultPassword
	setCookie(w, s.sessions.create(mustChange), int(sessionTimeout.Seconds()))
	writeJSON(w, http.StatusOK, map[string]any{"mustChange": mustChange})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	s.sessions.delete(r)
	setCookie(w, "", -1)
	writeJSON(w, http.StatusOK, map[string]any{})
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request) {
	var req struct{ Current, New string }
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	ok, err := system.CheckPassword(req.Current)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	if !ok {
		jsonError(w, http.StatusBadRequest, errors.New("wrong current password"))
		return
	}
	if req.New == system.DefaultPassword || req.New == req.Current {
		jsonError(w, http.StatusBadRequest, errors.New("choose a new password"))
		return
	}
	if err := system.SetPassword(req.New); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	s.sessions.passwordChanged(r)
	writeJSON(w, http.StatusOK, map[string]any{})
}

// --- System ----------------------------------------------------------------

func (s *Server) getSystem(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, system.ReadInfo(s.cfg.Root))
}

func (s *Server) power(w http.ResponseWriter, r *http.Request) {
	switch r.PathValue("action") {
	case "reboot":
		system.Reboot()
	case "poweroff":
		system.Poweroff()
	default:
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

func (s *Server) getSettingsAPI(w http.ResponseWriter, r *http.Request) {
	st := s.getSettings()
	host, _ := os.Hostname()
	writeJSON(w, http.StatusOK, map[string]any{
		"lang": st.Lang, "setupDone": st.SetupDone,
		"timezone": system.Timezone(), "hostname": host,
	})
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Lang      *string `json:"lang"`
		SetupDone *bool   `json:"setupDone"`
		Timezone  *string `json:"timezone"`
		Hostname  *string `json:"hostname"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	// Only the language may be changed before the default password.
	if ses := s.sessions.get(r); ses.MustChange && (req.SetupDone != nil || req.Timezone != nil || req.Hostname != nil) {
		jsonError(w, http.StatusForbidden, errors.New("change the default password first"))
		return
	}
	if req.Lang != nil {
		if err := system.SetLanguage(*req.Lang); err != nil {
			jsonError(w, http.StatusBadRequest, err)
			return
		}
	}
	if req.Timezone != nil && *req.Timezone != system.Timezone() {
		if err := system.SetTimezone(*req.Timezone); err != nil {
			jsonError(w, http.StatusBadRequest, err)
			return
		}
	}
	if req.Hostname != nil {
		if host, _ := os.Hostname(); host != *req.Hostname {
			if err := system.SetHostname(*req.Hostname); err != nil {
				jsonError(w, http.StatusBadRequest, err)
				return
			}
		}
	}
	err := s.saveSettings(func(st *Settings) {
		if req.Lang != nil {
			st.Lang = *req.Lang
		}
		if req.SetupDone != nil {
			st.SetupDone = *req.SetupDone
		}
	})
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	s.getSettingsAPI(w, r)
}

func (s *Server) getTimezones(w http.ResponseWriter, r *http.Request) {
	list, err := system.Timezones()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) getNetwork(w http.ResponseWriter, r *http.Request) {
	n, err := system.ReadNetwork()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, n)
}

func (s *Server) putNetwork(w http.ResponseWriter, r *http.Request) {
	var n system.Network
	if err := readJSON(r, &n); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	if err := n.Validate(); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	// The address may change: answer first, apply a moment later.
	go func() {
		time.Sleep(time.Second)
		if err := system.ApplyNetwork(&n); err != nil {
			log.Printf("network: %v", err)
		}
	}()
	writeJSON(w, http.StatusOK, map[string]any{})
}

// getSerial lists serial devices for the ZigBee coordinator, by id first.
func (s *Server) getSerial(w http.ResponseWriter, r *http.Request) {
	type dev struct {
		Path   string `json:"path"`
		Target string `json:"target"`
	}
	list := []dev{}
	byID, _ := filepath.Glob("/dev/serial/by-id/*")
	seen := map[string]bool{}
	for _, p := range byID {
		t, _ := filepath.EvalSymlinks(p)
		seen[t] = true
		list = append(list, dev{p, t})
	}
	for _, pattern := range []string{"/dev/ttyUSB*", "/dev/ttyACM*"} {
		m, _ := filepath.Glob(pattern)
		for _, p := range m {
			if !seen[p] {
				list = append(list, dev{p, p})
			}
		}
	}
	writeJSON(w, http.StatusOK, list)
}

// --- Add-ons ---------------------------------------------------------------

type optionDTO struct {
	addon.Option
	Default string `json:"default"`
}

type addonDTO struct {
	ID          string            `json:"id"`
	Name        addon.Text        `json:"name"`
	Description addon.Text        `json:"description"`
	Installed   bool              `json:"installed"`
	Requires    []string          `json:"requires"`
	Recommends  []string          `json:"recommends"`
	Options     []optionDTO       `json:"options"`
	Values      map[string]string `json:"values"`
	Web         *webDTO           `json:"web"`
	Backup      bool              `json:"backup"`
	Downloads   []addon.Download  `json:"downloads"`
	Containers  []stack.Container `json:"containers"`
}

type webDTO struct {
	Scheme string `json:"scheme"`
	Port   string `json:"port"`
	Path   string `json:"path"`
}

func (s *Server) getAddons(w http.ResponseWriter, r *http.Request) {
	st, err := s.openStack()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	containers, err := st.Containers(ctx)
	if err != nil {
		log.Printf("containers: %v", err)
	}
	byService := map[string]stack.Container{}
	for _, c := range containers {
		byService[c.Service] = c
	}
	list := []addonDTO{}
	for _, id := range st.Catalog.IDs() {
		a := st.Catalog[id]
		d := addonDTO{ID: id, Name: a.Name, Description: a.Description, Installed: st.IsInstalled(id),
			Requires: nonNil(a.Requires), Recommends: nonNil(a.Recommends), Backup: a.Backup != nil,
			Values: map[string]string{}, Options: []optionDTO{}, Containers: []stack.Container{},
			Downloads: []addon.Download{}}
		values := st.Values(a)
		for i := range a.Options {
			o := &a.Options[i]
			if o.Type == addon.TypeSecret {
				continue
			}
			od := optionDTO{Option: *o, Default: stack.DefaultValue(o)}
			// Passwords are sent too: the panel is for the device owner, who
			// needs e.g. the MQTT password for other devices.
			if d.Installed {
				d.Values[o.Key] = values[o.Key]
			}
			d.Options = append(d.Options, od)
		}
		if d.Installed {
			for _, svc := range a.Services {
				if c, ok := byService[svc]; ok {
					d.Containers = append(d.Containers, c)
				} else {
					d.Containers = append(d.Containers, stack.Container{Service: svc, State: "missing"})
				}
			}
			for _, dl := range a.Downloads {
				if _, err := os.Stat(filepath.Join(st.Dir(), dl.File)); err == nil {
					d.Downloads = append(d.Downloads, dl)
				}
			}
			if a.Web != nil {
				scheme := a.Web.Scheme
				if scheme == "" {
					scheme = "http"
				}
				d.Web = &webDTO{Scheme: scheme, Port: st.Expand(a.Web.Port), Path: a.Web.Path}
			}
		}
		list = append(list, d)
	}
	writeJSON(w, http.StatusOK, list)
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// stackJob runs fn on a freshly opened, locked stack as a job, with the
// docker compose output in the job log.
func (s *Server) stackJob(w http.ResponseWriter, title string, fn func(st *stack.Stack, j *Job) error) {
	j, err := s.jobs.start(title, func(j *Job) error {
		st, err := s.openStack()
		if err != nil {
			return err
		}
		st.Stdout, st.Stderr, st.Stdin = j, j, strings.NewReader("")
		if err := st.Lock(); err != nil {
			return err
		}
		defer st.Unlock()
		if err := fn(st, j); err != nil {
			j.Logf("Error: %v", err)
			return err
		}
		return nil
	})
	if err != nil {
		var busy errBusy
		if errors.As(err, &busy) {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "job": busy.job.snapshot()})
			return
		}
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusAccepted, j.snapshot())
}

// readValues reads option values from the request.
func readValues(r *http.Request) (map[string]string, error) {
	var req struct {
		Values map[string]string `json:"values"`
	}
	if err := readJSON(r, &req); err != nil {
		return nil, err
	}
	if req.Values == nil {
		req.Values = map[string]string{}
	}
	return req.Values, nil
}

// dropUnchanged removes values equal to the stored ones, so that only real
// changes are applied.
func dropUnchanged(st *stack.Stack, a *addon.Addon, set map[string]string) {
	if !st.IsInstalled(a.ID) {
		return
	}
	values := st.Values(a)
	for k, v := range set {
		if values[k] == v {
			delete(set, k)
		}
	}
}

// summary ends a job with the add-on state and the address of its
// interface, as seen from the browser.
func summary(st *stack.Stack, j *Job, id, host string) {
	a := st.Catalog[id]
	j.Logf("")
	j.Logf("%s is running.", a.Name.In("en"))
	if a.Web != nil {
		scheme := a.Web.Scheme
		if scheme == "" {
			scheme = "http"
		}
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		j.Logf("Open: %s://%s:%s%s", scheme, host, st.Expand(a.Web.Port), a.Web.Path)
	}
}

func (s *Server) installAddon(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	set, err := readValues(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	s.stackJob(w, "install "+id, func(st *stack.Stack, j *Job) error {
		a := st.Catalog[id]
		if a == nil {
			return fmt.Errorf("unknown add-on %q", id)
		}
		dropUnchanged(st, a, set)
		j.Logf("Installing %s", id)
		if err := st.Install(id, set); err != nil {
			return err
		}
		if err := st.Up(); err != nil {
			return err
		}
		summary(st, j, id, r.Host)
		return nil
	})
}

func (s *Server) configAddon(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	set, err := readValues(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	s.stackJob(w, "configure "+id, func(st *stack.Stack, j *Job) error {
		a := st.Catalog[id]
		if a == nil || !st.IsInstalled(id) {
			return fmt.Errorf("%s is not installed", id)
		}
		dropUnchanged(st, a, set)
		if len(set) == 0 {
			j.Logf("No changes.")
			return nil
		}
		if err := st.Configure(id, set); err != nil {
			return err
		}
		if err := st.Up(); err != nil {
			return err
		}
		summary(st, j, id, r.Host)
		return nil
	})
}

func (s *Server) removeAddon(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req struct{ Purge bool }
	if err := readJSON(r, &req); err != nil && !errors.Is(err, io.EOF) {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	s.stackJob(w, "remove "+id, func(st *stack.Stack, j *Job) error {
		if d := st.Dependents(id); len(d) > 0 {
			return fmt.Errorf("%s is required by %s, remove it first", id, strings.Join(d, ", "))
		}
		services, err := st.Services(id)
		if err != nil {
			return err
		}
		if err := st.Compose(append([]string{"rm", "--stop", "--force"}, services...)...); err != nil {
			return err
		}
		if err := st.Remove(id); err != nil {
			return err
		}
		if len(st.State.Addons) > 0 {
			if err := st.Up(); err != nil {
				return err
			}
		} else {
			st.RemoveNetwork()
		}
		if req.Purge {
			j.Logf("Deleting the data of %s", id)
			return st.Purge(id)
		}
		return nil
	})
}

func (s *Server) addonAction(w http.ResponseWriter, r *http.Request) {
	id, action := r.PathValue("id"), r.PathValue("action")
	switch action {
	case "start", "stop", "restart", "update", "backup":
	default:
		http.NotFound(w, r)
		return
	}
	s.stackJob(w, action+" "+id, func(st *stack.Stack, j *Job) error {
		services, err := st.Services(id)
		if err != nil {
			return err
		}
		switch action {
		case "start":
			return st.Compose(append([]string{"up", "-d"}, services...)...)
		case "stop", "restart":
			return st.Compose(append([]string{action}, services...)...)
		case "update":
			if st.Catalog[id].Backup != nil {
				path, err := st.Backup(id)
				if err != nil {
					return err
				}
				j.Logf("Backup saved to %s", path)
			}
			if err := st.Compose(append([]string{"pull"}, services...)...); err != nil {
				return err
			}
			return st.Compose(append([]string{"up", "-d"}, services...)...)
		default: // backup
			path, err := st.Backup(id)
			if err != nil {
				return err
			}
			j.Logf("Backup saved to %s", path)
			return nil
		}
	})
}

// addonLogs streams docker compose logs of the add-on as server-sent
// events until the browser goes away.
func (s *Server) addonLogs(w http.ResponseWriter, r *http.Request) {
	st, err := s.openStack()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	services, err := st.Services(r.PathValue("id"))
	if err != nil {
		jsonError(w, http.StatusNotFound, err)
		return
	}
	tail := r.URL.Query().Get("tail")
	if _, err := strconv.Atoi(tail); err != nil {
		tail = "200"
	}
	cmd := st.Command(r.Context(), append([]string{"logs", "-f", "--no-color", "--tail", tail}, services...)...)
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	go func() {
		cmd.Wait()
		pw.Close()
	}()
	sse := newSSE(w)
	sc := bufio.NewScanner(pr)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		sse.send("line", stripANSI(sc.Text()))
	}
	sse.send("end", "")
}

// downloadFile serves a file listed in downloads of an installed add-on.
func (s *Server) downloadFile(w http.ResponseWriter, r *http.Request) {
	st, err := s.openStack()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	a := st.Catalog[r.PathValue("id")]
	if a == nil || !st.IsInstalled(a.ID) {
		jsonError(w, http.StatusNotFound, errors.New("add-on not installed"))
		return
	}
	i, err := strconv.Atoi(r.PathValue("n"))
	if err != nil || i < 0 || i >= len(a.Downloads) {
		jsonError(w, http.StatusNotFound, errors.New("no such file"))
		return
	}
	p := filepath.Join(st.Dir(), a.Downloads[i].File)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", a.ID+"-"+filepath.Base(p)))
	http.ServeFile(w, r, p)
}

// suplaUsers tells how many accounts the Supla database has; the panel
// offers creating one only when there is none (or registration is on).
func (s *Server) suplaUsers(w http.ResponseWriter, r *http.Request) {
	st, err := s.openStack()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	if !st.IsInstalled("supla") {
		jsonError(w, http.StatusNotFound, errors.New("supla is not installed"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	cmd := st.Command(ctx, "exec", "-T", "supla-db", "sh", "-c",
		`exec mariadb -N -B --user=supla --password="$MYSQL_PASSWORD" supla -e "select count(*) from supla_user"`)
	out, err := cmd.Output()
	if err != nil {
		jsonError(w, http.StatusServiceUnavailable, fmt.Errorf("database not available: %w", err))
		return
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"count": n})
}

func (s *Server) createSuplaUser(w http.ResponseWriter, r *http.Request) {
	var req struct{ Email, Password string }
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	if !strings.Contains(req.Email, "@") || len(req.Password) < 8 {
		jsonError(w, http.StatusBadRequest, errors.New("e-mail and a password of at least 8 characters are required"))
		return
	}
	s.stackJob(w, "create Supla account", func(st *stack.Stack, j *Job) error {
		j.Logf("Creating the account %s", req.Email)
		if err := st.Run("supla", "console", []string{"supla:user:create", req.Email, req.Password}); err != nil {
			return err
		}
		// E.g. MQTT turned on for the new account.
		st.QueueAfter("supla", "SUPLA_MQTT_BROKER_ENABLED")
		return st.RunAfter()
	})
}

// --- Updates ---------------------------------------------------------------

func (s *Server) getUpdates(w http.ResponseWriter, r *http.Request) {
	u, err := system.ReadUpdates()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	pending := []string{}
	if st, err := s.openStack(); err == nil {
		if p, err := st.Pending(); err == nil && p != nil {
			pending = p
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"system": u, "addons": pending})
}

// checkUpdates downloads the package lists as a job.
func (s *Server) checkUpdates(w http.ResponseWriter, r *http.Request) {
	j, err := s.jobs.start("check for updates", func(j *Job) error {
		return system.CheckUpdates(j)
	})
	if err != nil {
		jsonError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusAccepted, j.snapshot())
}

// startUpgrade starts the system update. It runs apart from the panel,
// which it may restart; the browser follows /api/updates/log.
func (s *Server) startUpgrade(w http.ResponseWriter, r *http.Request) {
	if j := s.jobs.current(); j != nil {
		jsonError(w, http.StatusConflict, errBusy{j})
		return
	}
	if err := system.StartUpgrade(); err != nil {
		jsonError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{})
}

func (s *Server) setAutoUpdates(w http.ResponseWriter, r *http.Request) {
	var req struct{ Enabled bool }
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	if err := system.SetAutoUpgrades(req.Enabled); err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": system.AutoUpgrades()})
}

func (s *Server) upgradeLog(w http.ResponseWriter, r *http.Request) {
	offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	text, next, err := system.UpgradeLog(offset)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	state := ""
	if u, err := system.ReadUpdates(); err == nil {
		state = u.State
	}
	writeJSON(w, http.StatusOK, map[string]any{"text": stripANSI(text), "offset": next, "state": state})
}

// applyAddons puts new versions of add-on files in place, after a backup
// of the add-ons that have one.
func (s *Server) applyAddons(w http.ResponseWriter, r *http.Request) {
	s.stackJob(w, "apply add-on updates", func(st *stack.Stack, j *Job) error {
		pending, err := st.Pending()
		if err != nil {
			return err
		}
		for _, id := range pending {
			if st.Catalog[id].Backup != nil {
				path, err := st.Backup(id)
				if err != nil {
					return err
				}
				j.Logf("Backup saved to %s", path)
			}
		}
		j.Logf("Applying: %s", strings.Join(pending, ", "))
		if err := st.Refetch(); err != nil {
			return err
		}
		if err := st.Apply(); err != nil {
			return err
		}
		return st.Up()
	})
}

// --- Backups ---------------------------------------------------------------

func (s *Server) backupPath(name string) (string, error) {
	if name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return "", errors.New("invalid name")
	}
	return filepath.Join(s.cfg.Root, "backups", name), nil
}

func (s *Server) getBackups(w http.ResponseWriter, r *http.Request) {
	type entry struct {
		Name string    `json:"name"`
		Size int64     `json:"size"`
		Time time.Time `json:"time"`
	}
	list := []entry{}
	entries, _ := os.ReadDir(filepath.Join(s.cfg.Root, "backups"))
	for _, e := range entries {
		if info, err := e.Info(); err == nil && info.Mode().IsRegular() {
			list = append(list, entry{e.Name(), info.Size(), info.ModTime()})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Time.After(list[j].Time) })
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	p, err := s.backupPath(r.PathValue("name"))
	if err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(p)))
	http.ServeFile(w, r, p)
}

func (s *Server) deleteBackup(w http.ResponseWriter, r *http.Request) {
	p, err := s.backupPath(r.PathValue("name"))
	if err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	if err := os.Remove(p); err != nil {
		jsonError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

// --- Jobs ------------------------------------------------------------------

func (s *Server) currentJob(w http.ResponseWriter, r *http.Request) {
	if j := s.jobs.current(); j != nil {
		writeJSON(w, http.StatusOK, j.snapshot())
		return
	}
	writeJSON(w, http.StatusOK, nil)
}

func (s *Server) job(w http.ResponseWriter, r *http.Request) *Job {
	id, _ := strconv.Atoi(r.PathValue("id"))
	j := s.jobs.get(id)
	if j == nil {
		jsonError(w, http.StatusNotFound, errors.New("no such job"))
	}
	return j
}

func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	if j := s.job(w, r); j != nil {
		writeJSON(w, http.StatusOK, j.snapshot())
	}
}

// jobEvents streams the job output: "line" events, then "end" with the
// job state.
func (s *Server) jobEvents(w http.ResponseWriter, r *http.Request) {
	j := s.job(w, r)
	if j == nil {
		return
	}
	sse := newSSE(w)
	if !j.follow(r.Context(), func(lines []string) {
		for _, l := range lines {
			sse.send("line", l)
		}
	}) {
		return
	}
	data, _ := json.Marshal(j.snapshot())
	sse.send("end", string(data))
}

type sseWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

func newSSE(w http.ResponseWriter) *sseWriter {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	f, _ := w.(http.Flusher)
	s := &sseWriter{w, f}
	s.flush()
	return s
}

func (s *sseWriter) send(event, data string) {
	fmt.Fprintf(s.w, "event: %s\n", event)
	for _, l := range strings.Split(data, "\n") {
		fmt.Fprintf(s.w, "data: %s\n", l)
	}
	fmt.Fprint(s.w, "\n")
	s.flush()
}

func (s *sseWriter) flush() {
	if s.f != nil {
		s.f.Flush()
	}
}
