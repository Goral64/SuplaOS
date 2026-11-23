package panel

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goral64/SuplaOS/internal/migrate"
	"github.com/Goral64/SuplaOS/internal/stack"
)

func (s *Server) migrationStore() migrate.Store {
	return migrate.Store{Dir: filepath.Join(s.cfg.Root, "migration")}
}

// migrationScope lets the panel set what the move needs in the local Cloud.
// SUPLA Scripts logs in only with channels_ea.
const migrationScope = "accessids_rw account_rw channels_rw channels_ea channelgroups_rw directlinks_rw iodevices_rw locations_rw scenes_rw schedules_rw"

// cloudCaller is the Cloud client of the panel for package migrate.
type cloudCaller struct{ *cloudAPI }

func (c cloudCaller) Call(method, path string, body, out any) error {
	return c.call(method, path, body, out)
}

func (s *Server) getMigration(w http.ResponseWriter, r *http.Request) {
	store := s.migrationStore()
	b, err := store.Bundle()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	if b == nil {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	st, err := store.State()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	type device struct {
		ID       int    `json:"id"`
		Name     string `json:"name"`
		Channels int    `json:"channels"`
		Status   string `json:"status"`
		Error    string `json:"error,omitempty"`
	}
	devices := []device{}
	for _, d := range b.Devices {
		dv := device{ID: d.ID, Channels: len(d.Channels), Status: "waiting"}
		if d.Comment != nil && *d.Comment != "" {
			dv.Name = *d.Comment
		} else if d.Name != nil {
			dv.Name = *d.Name
		}
		if ds := st.Devices[d.ID]; ds != nil {
			dv.Status, dv.Error = ds.Status, ds.Error
		}
		devices = append(devices, dv)
	}
	var prepared *time.Time
	if !st.Prepared.IsZero() {
		prepared = &st.Prepared
	}
	server, scripts := "", false
	var resume *migrate.Resume
	if stk, err := s.openStack(); err == nil && stk.IsInstalled("supla") {
		server = stk.Values(stk.Catalog["supla"])["SUPLA_HOST_ADDRESS"]
		scripts = stk.IsInstalled("supla-scripts")
		// A move ended before: the devices here already.
		if st.Prepared.IsZero() {
			if res, err := migrate.CheckResume(migrate.StackDB{Stack: stk}, b); err == nil && res.Possible() {
				resume = res
			}
		}
	}
	type link struct {
		Caption string `json:"caption"`
		URL     string `json:"url"`
	}
	links := []link{}
	if st.Objects != nil {
		for _, l := range st.Objects.Links {
			links = append(links, link{l.Caption, fmt.Sprintf("https://%s/direct/%d/%s", server, l.ID, l.Slug)})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"source": b.Server, "email": b.Email, "fetched": b.Fetched,
		"locations": len(b.Locations), "accessIds": len(b.AccessIDs), "channels": b.ChannelCount(),
		"objects": map[string]int{
			"icons": len(b.UserIcons), "channelGroups": len(b.ChannelGroups), "scenes": len(b.Scenes),
			"schedules": len(b.Schedules), "reactions": len(b.Reactions), "directLinks": len(b.DirectLinks),
		},
		"made": st.Objects, "links": links, "history": st.History, "scripts": st.Scripts, "scriptsInstalled": scripts,
		"prepared": prepared, "devices": devices, "server": server, "resume": resume, "resumed": st.Resumed,
	})
}

// fetchMigration reads the account of the source Cloud with a personal
// access token made there.
func (s *Server) fetchMigration(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	src, err := migrate.NewSource(req.Token)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	b, err := src.Fetch()
	if err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	s.migration.Lock()
	defer s.migration.Unlock()
	store := s.migrationStore()
	if err := store.SaveBundle(b); err != nil {
		jsonError(w, http.StatusConflict, err)
		return
	}
	// Kept until the move ends, for the measurement logs.
	if st, err := store.State(); err == nil {
		st.SourceToken = strings.TrimSpace(req.Token)
		store.SaveState(st)
	}
	s.getMigration(w, r)
}

// prepareMigration puts the skeleton of the account in the local Cloud.
// The local account has the e-mail of the source one, made with the
// password when there is none.
func (s *Server) prepareMigration(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	store := s.migrationStore()
	s.stackJob(w, "prepare the move from Supla Cloud", func(st *stack.Stack, j *Job) error {
		s.migration.Lock()
		defer s.migration.Unlock()
		b, err := store.Bundle()
		if err != nil || b == nil {
			return errors.New("read the source account first")
		}
		state, err := store.State()
		if err != nil {
			return err
		}
		if !state.Prepared.IsZero() {
			return errors.New("the move is prepared already")
		}
		if !st.IsInstalled("supla") {
			return errors.New("Supla is not installed")
		}
		db := migrate.StackDB{Stack: st}
		created := false
		if _, err := migrate.UserID(db, b.Email); err != nil {
			created = true
			acc := suplaAccount{Email: b.Email, Password: req.Password}
			if err := acc.check(); err != nil {
				return err
			}
			if err := acc.create(st, j); err != nil {
				return err
			}
		}
		cloud, token, err := s.moveToken(st, b, req.Password, j)
		if err != nil {
			return err
		}
		saved := false
		defer func() {
			if !saved {
				token.remove(cloud)
			}
		}()
		if err := migrate.Prepare(db, b, j); err != nil {
			return err
		}
		j.Logf("Adding icons, channel groups, scenes, schedules, reactions and direct links")
		state.Objects = migrate.CreateObjects(cloudCaller{cloud}, db, b, j)
		if created {
			// The location and access ID a new account gets are not needed
			// next to those of the source.
			migrate.RemoveDefaults(cloudCaller{cloud}, b)
		}
		// Times of schedules and of the history are shown in it.
		if b.Timezone != "" {
			if err := cloud.call("PATCH", "/api/v3/users/current",
				map[string]string{"action": "change:userTimezone", "timezone": b.Timezone}, nil); err != nil {
				j.Logf("Time zone of the account: %v", err)
			}
		}
		state.Prepared, state.Token = time.Now().UTC(), token.Token
		if err := store.SaveState(state); err != nil {
			return err
		}
		saved = true
		if err := importHistory(store, state, b, db, j); err != nil {
			return fmt.Errorf("the move is prepared, but the measurement history is not complete, copy it again before you end the move: %w", err)
		}
		j.Logf("Ready. Switch the devices to this server: they keep their IDs and get their settings when they connect.")
		return nil
	})
}

// personalToken is a personal access token made in the local Cloud.
type personalToken struct {
	ID    int    `json:"id"`
	Token string `json:"token"`
}

// remove deletes the token, so that a failed attempt does not leave it in
// the account.
func (t *personalToken) remove(cloud *cloudAPI) {
	if t.ID > 0 {
		cloud.call("DELETE", fmt.Sprintf("/api/v3/oauth-personal-tokens/%d", t.ID), nil, nil)
	}
}

// moveToken logs in to the local Cloud as the account of the move and
// makes the token the panel sets the devices with.
func (s *Server) moveToken(st *stack.Stack, b *migrate.Bundle, password string, j *Job) (*cloudAPI, *personalToken, error) {
	cloud, err := s.cloudClient("https://127.0.0.1:" + st.Values(st.Catalog["supla"])["PORT_HTTPS"])
	if err != nil {
		return nil, nil, err
	}
	j.Logf("Logging in to Supla Cloud as %s", b.Email)
	if err := cloud.login(b.Email, password); err != nil {
		return nil, nil, err
	}
	token := &personalToken{}
	if err := cloud.call("POST", "/api/v3/oauth-personal-tokens",
		map[string]string{"name": "SuplaOS: move from " + strings.TrimPrefix(b.Server, "https://"), "scope": migrationScope}, token); err != nil {
		return nil, nil, err
	}
	return cloud, token, nil
}

// resumeMigration goes on with a move that was ended before all the
// devices were switched: with the devices here with their ID and GUID,
// without making the skeleton and the other objects again.
func (s *Server) resumeMigration(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
		// Overwrite: devices connected here already that get the settings
		// of the source again.
		Overwrite []int `json:"overwrite"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	store := s.migrationStore()
	s.stackJob(w, "resume the move from Supla Cloud", func(st *stack.Stack, j *Job) error {
		s.migration.Lock()
		defer s.migration.Unlock()
		b, err := store.Bundle()
		if err != nil || b == nil {
			return errors.New("read the source account first")
		}
		state, err := store.State()
		if err != nil {
			return err
		}
		if !state.Prepared.IsZero() {
			return errors.New("the move is prepared already")
		}
		if !st.IsInstalled("supla") {
			return errors.New("Supla is not installed")
		}
		db := migrate.StackDB{Stack: st}
		res, err := migrate.CheckResume(db, b)
		if err != nil {
			return err
		}
		if !res.Possible() {
			return errors.New("none of the devices is here with its ID and GUID on the account, there is nothing to resume")
		}
		cloud, token, err := s.moveToken(st, b, req.Password, j)
		if err != nil {
			return err
		}
		state.Devices = res.Start(req.Overwrite)
		state.Resumed, state.SkipChannels = true, res.SkipChannels
		state.Prepared, state.Token = time.Now().UTC(), token.Token
		if err := store.SaveState(state); err != nil {
			token.remove(cloud)
			return err
		}
		j.Logf("Waiting: %d devices, here already: %d, not here: %d, of another device or account: %d",
			len(res.Waiting), len(res.Here), len(res.Missing), len(res.Foreign))
		if err := importHistory(store, state, b, db, j); err != nil {
			return fmt.Errorf("the move is resumed, but the measurement history is not complete, copy it again before you end the move: %w", err)
		}
		j.Logf("Ready. Switch the remaining devices to this server: they get their settings when they connect.")
		return nil
	})
}

// importHistory copies the measurement logs from the source; a failure
// is noted in the state and returned, the move stays prepared.
func importHistory(store migrate.Store, state *migrate.State, b *migrate.Bundle, db migrate.DB, j *Job) error {
	j.Logf("Copying the measurement history")
	run := &migrate.HistoryRun{Time: time.Now().UTC()}
	// Nothing noted as copied: all is read, so the count starts anew.
	again := len(state.Copied) > 0
	if state.Copied == nil {
		state.Copied = map[string]int64{}
	}
	src, err := migrate.NewSource(state.SourceToken)
	if err == nil {
		run.Added, err = migrate.ImportHistory(src, db, state.Moved(b), state.Copied, j)
	}
	run.Items = run.Added
	if again && state.History != nil {
		run.Items += state.History.Items
	}
	if err != nil {
		run.Error = err.Error()
		j.Logf("History: %v", err)
	} else {
		j.Logf("History: %d new log items", run.Added)
	}
	state.History = run
	if serr := store.SaveState(state); err == nil {
		err = serr
	}
	return err
}

// historyMigration copies again the measurement logs the source made
// since the last time, e.g. until the devices were switched.
func (s *Server) historyMigration(w http.ResponseWriter, r *http.Request) {
	store := s.migrationStore()
	s.stackJob(w, "copy the measurement history", func(st *stack.Stack, j *Job) error {
		s.migration.Lock()
		defer s.migration.Unlock()
		b, err := store.Bundle()
		if err != nil || b == nil {
			return errors.New("no move")
		}
		state, err := store.State()
		if err != nil {
			return err
		}
		if state.Prepared.IsZero() {
			return errors.New("prepare the move first")
		}
		return importHistory(store, state, b, migrate.StackDB{Stack: st}, j)
	})
}

// scriptsMigration copies the objects of SUPLA Scripts of the source
// account (scenes, thermostats, notifications) to SUPLA Scripts here.
func (s *Server) scriptsMigration(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	if !strings.HasPrefix(req.URL, "https://") {
		jsonError(w, http.StatusBadRequest, errors.New("the address of SUPLA Scripts starts with https://"))
		return
	}
	store := s.migrationStore()
	s.stackJob(w, "move SUPLA Scripts", func(st *stack.Stack, j *Job) error {
		s.migration.Lock()
		defer s.migration.Unlock()
		state, err := store.State()
		if err != nil || state.Prepared.IsZero() {
			return errors.New("prepare the move first")
		}
		if !st.IsInstalled("supla-scripts") {
			return errors.New("SUPLA Scripts is not installed")
		}
		// The token of the move may lack Channels: execute actions, which
		// SUPLA Scripts asks for.
		token := state.SourceToken
		if req.Token != "" {
			token = req.Token
		}
		j.Logf("Reading %s", req.URL)
		src, err := migrate.ScriptsLogin(req.URL, token, "", "", nil)
		if err != nil {
			return err
		}
		bundle, err := src.ExportScripts()
		if err != nil {
			return err
		}
		j.Logf("%d scenes, %d thermostats, %d notifications", len(bundle.Scenes), len(bundle.Thermostats), len(bundle.Notifications))
		local, err := s.cloudClient("https://127.0.0.1:" + st.Values(st.Catalog["supla-scripts"])["SUPLA_SCRIPTS_PORT"])
		if err != nil {
			return err
		}
		// A move prepared before SUPLA Scripts was moved has a token
		// without the scope SUPLA Scripts asks for.
		if _, err := (migrate.StackDB{Stack: st}).Query(migrate.AddScopeSQL(state.Token, "channels_ea")); err != nil {
			return err
		}
		dst, err := migrate.ScriptsLogin(local.url, state.Token, "", "", local.client)
		if errors.Is(err, migrate.ErrScriptsNoLogin) {
			// An account of SUPLA Scripts here logged in only through the
			// Cloud: a username lets the token in; the Cloud login stays.
			b, berr := store.Bundle()
			if berr != nil || b == nil {
				return err
			}
			dst, err = migrate.ScriptsLogin(local.url, state.Token, b.Email, randomPassword(), local.client)
		}
		if err != nil {
			return fmt.Errorf("SUPLA Scripts here: %w", err)
		}
		state.Scripts = migrate.ImportScripts(dst, bundle, j)
		return store.SaveState(state)
	})
}

// randomPassword is a password nobody types: the Cloud login is used.
func randomPassword() string {
	b := make([]byte, 18)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s *Server) cancelMigration(w http.ResponseWriter, r *http.Request) {
	s.migration.Lock()
	defer s.migration.Unlock()
	store := s.migrationStore()
	// The token the devices were set with is of no use any more, a resumed
	// move makes a new one. SUPLA Scripts moved here logged in with it and
	// may keep using it.
	if state, err := store.State(); err == nil && state.Token != "" && state.Scripts == nil {
		if st, err := s.openStack(); err == nil && st.IsInstalled("supla") {
			if _, err := (migrate.StackDB{Stack: st}).Query(migrate.DeleteTokenSQL(state.Token)); err != nil {
				log.Printf("move from Supla Cloud: deleting its token: %v", err)
			}
		}
	}
	if err := store.Cancel(); err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

// WatchMigration sets the devices of a prepared move as they connect to
// this server; it does not return.
func (s *Server) WatchMigration() {
	for {
		time.Sleep(time.Minute)
		if err := s.applyMigration(); err != nil {
			log.Printf("move from Supla Cloud: %v", err)
		}
	}
}

func (s *Server) applyMigration() error {
	// A job of the move is running: next time.
	if !s.migration.TryLock() {
		return nil
	}
	defer s.migration.Unlock()
	store := s.migrationStore()
	b, err := store.Bundle()
	if err != nil || b == nil {
		return err
	}
	state, err := store.State()
	if err != nil || state.Prepared.IsZero() || state.Token == "" {
		return err
	}
	st, err := s.openStack()
	if err != nil || !st.IsInstalled("supla") {
		return err
	}
	cloud, err := s.cloudClient("https://127.0.0.1:" + st.Values(st.Catalog["supla"])["PORT_HTTPS"])
	if err != nil {
		return err
	}
	cloud.token = state.Token
	if _, err := migrate.Apply(migrate.StackDB{Stack: st}, cloudCaller{cloud}, state.Moved(b), state, log.Writer()); err != nil {
		return err
	}
	return store.SaveState(state)
}
