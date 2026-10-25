package panel

import (
	"errors"
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
const migrationScope = "accessids_rw account_rw channels_rw channelgroups_rw iodevices_rw locations_rw scenes_rw schedules_rw"

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
	server := ""
	if stk, err := s.openStack(); err == nil && stk.IsInstalled("supla") {
		server = stk.Values(stk.Catalog["supla"])["SUPLA_HOST_ADDRESS"]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"source": b.Server, "email": b.Email, "fetched": b.Fetched,
		"locations": len(b.Locations), "accessIds": len(b.AccessIDs), "channels": b.ChannelCount(),
		"other": b.Other, "prepared": prepared, "devices": devices, "server": server,
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
	if err := s.migrationStore().SaveBundle(b); err != nil {
		jsonError(w, http.StatusConflict, err)
		return
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
		if _, err := migrate.UserID(db, b.Email); err != nil {
			acc := suplaAccount{Email: b.Email, Password: req.Password}
			if err := acc.check(); err != nil {
				return err
			}
			if err := acc.create(st, j); err != nil {
				return err
			}
		}
		cloud, err := s.cloudClient("https://127.0.0.1:" + st.Values(st.Catalog["supla"])["PORT_HTTPS"])
		if err != nil {
			return err
		}
		j.Logf("Logging in to Supla Cloud as %s", b.Email)
		if err := cloud.login(b.Email, req.Password); err != nil {
			return err
		}
		var token struct {
			Token string `json:"token"`
		}
		if err := cloud.call("POST", "/api/v3/oauth-personal-tokens",
			map[string]string{"name": "SuplaOS: move from " + strings.TrimPrefix(b.Server, "https://"), "scope": migrationScope}, &token); err != nil {
			return err
		}
		if err := migrate.Prepare(db, b, j); err != nil {
			return err
		}
		state.Prepared, state.Token = time.Now().UTC(), token.Token
		if err := store.SaveState(state); err != nil {
			return err
		}
		j.Logf("Ready. Switch the devices to this server: they keep their IDs and get their settings when they connect.")
		return nil
	})
}

func (s *Server) cancelMigration(w http.ResponseWriter, r *http.Request) {
	if err := s.migrationStore().Cancel(); err != nil {
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
	if _, err := migrate.Apply(migrate.StackDB{Stack: st}, cloudCaller{cloud}, b, state, log.Writer()); err != nil {
		return err
	}
	return store.SaveState(state)
}
