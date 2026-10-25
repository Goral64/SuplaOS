package migrate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DB runs SQL in the database of the local Supla Cloud and returns the
// rows of the last statement, fields split.
type DB interface {
	Query(sql string) ([][]string, error)
}

// Cloud calls the API of the local Supla Cloud.
type Cloud interface {
	Call(method, path string, body, out any) error
}

// DeviceState is how far the move of a device is.
type DeviceState struct {
	// Status: waiting (not connected to this server yet), done, error.
	Status string    `json:"status"`
	Error  string    `json:"error,omitempty"`
	Done   time.Time `json:"done,omitzero"`
}

// State of a move, kept next to the bundle.
type State struct {
	Prepared time.Time `json:"prepared,omitzero"`
	// Token is a personal access token of the local Cloud, to set the
	// channels of devices that connect later.
	Token   string               `json:"token,omitempty"`
	Devices map[int]*DeviceState `json:"devices"`
	// Objects made by CreateObjects.
	Objects *Objects `json:"objects,omitempty"`
	// SourceToken reads the source again for its measurement logs, until
	// the move ends.
	SourceToken string      `json:"sourceToken,omitempty"`
	History     *HistoryRun `json:"history,omitempty"`
}

// HistoryRun is the last import of measurement logs: Items of all of
// them, Added by the last one.
type HistoryRun struct {
	Time  time.Time `json:"time"`
	Items int       `json:"items"`
	Added int       `json:"added"`
	Error string    `json:"error,omitempty"`
}

// Store is the directory of a move: the bundle read from the source and
// the state. Both hold passwords, readable only by root.
type Store struct{ Dir string }

func (st Store) bundleFile() string { return filepath.Join(st.Dir, "source.json") }
func (st Store) stateFile() string  { return filepath.Join(st.Dir, "state.json") }

func (st Store) save(path string, v any) error {
	if err := os.MkdirAll(st.Dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (st Store) load(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// SaveBundle stores a newly read bundle and starts the state anew; a move
// already prepared is not replaced.
func (st Store) SaveBundle(b *Bundle) error {
	if s, err := st.State(); err == nil && !s.Prepared.IsZero() {
		return errors.New("a move is in progress, cancel it first")
	}
	if err := st.save(st.bundleFile(), b); err != nil {
		return err
	}
	return st.SaveState(&State{Devices: map[int]*DeviceState{}})
}

// Bundle returns the stored bundle, nil without one.
func (st Store) Bundle() (*Bundle, error) {
	var b Bundle
	if err := st.load(st.bundleFile(), &b); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return &b, nil
}

// State returns the state of the move.
func (st Store) State() (*State, error) {
	s := &State{}
	if err := st.load(st.stateFile(), s); err != nil {
		return nil, err
	}
	if s.Devices == nil {
		s.Devices = map[int]*DeviceState{}
	}
	return s, nil
}

func (st Store) SaveState(s *State) error { return st.save(st.stateFile(), s) }

// Cancel removes the move. What was put in the database stays.
func (st Store) Cancel() error { return os.RemoveAll(st.Dir) }

// UserID returns the ID of the local account with the e-mail of the source
// account: devices register with the e-mail, so it has to be the same.
func UserID(db DB, email string) (int, error) {
	rows, err := db.Query("SELECT id FROM supla_user WHERE email = " + str(email) + ";")
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("there is no account %s in the Supla Cloud of this device: devices register with the e-mail, make the account with it first", email)
	}
	return strconv.Atoi(rows[0][0])
}

// Conflicts lists objects of this server with the IDs of the bundle, e.g.
// "device 123".
func Conflicts(db DB, b *Bundle) ([]string, error) {
	q, err := ConflictSQL(b)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(q)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, r := range rows {
		out = append(out, strings.Join(r, " "))
	}
	return out, nil
}

// Prepare puts the skeleton of the account in the database: locations,
// access IDs, devices and channels with their IDs.
func Prepare(db DB, b *Bundle, log io.Writer) error {
	user, err := UserID(db, b.Email)
	if err != nil {
		return err
	}
	conflicts, err := Conflicts(db, b)
	if err != nil {
		return err
	}
	if len(conflicts) > 0 {
		return fmt.Errorf("the Supla Cloud of this device already has objects with the same IDs (%s): the move needs a Supla without them", strings.Join(conflicts, ", "))
	}
	q, err := SkeletonSQL(b, user)
	if err != nil {
		return err
	}
	fmt.Fprintf(log, "Adding %d locations, %d access IDs, %d devices and %d channels\n",
		len(b.Locations), len(b.AccessIDs), len(b.Devices), b.ChannelCount())
	_, err = db.Query(q)
	return err
}

// Connected returns the devices of the bundle that have connected to this
// server.
func Connected(db DB, b *Bundle) (map[int]bool, error) {
	rows, err := db.Query(fmt.Sprintf("SELECT id FROM supla_iodevice WHERE last_connected IS NOT NULL AND id IN (%s);",
		ids(b.Devices, func(d Device) int { return d.ID })))
	if err != nil {
		return nil, err
	}
	out := map[int]bool{}
	for _, r := range rows {
		if id, err := strconv.Atoi(r[0]); err == nil {
			out[id] = true
		}
	}
	return out, nil
}

// emptyConfig tells whether a configuration has nothing to set.
func emptyConfig(c json.RawMessage) bool {
	s := strings.TrimSpace(string(c))
	return s == "" || s == "null" || s == "{}" || s == "[]"
}

// ApplyDevice sets the settings that need the device connected: the
// configuration of the device and of its channels, through the Cloud, which
// checks them and passes them on to the device.
func ApplyDevice(cloud Cloud, d Device) error {
	body := map[string]any{"comment": d.Comment}
	if !emptyConfig(d.Config) {
		body["config"] = d.Config
	}
	if err := cloud.Call("PUT", fmt.Sprintf("/api/v3/iodevices/%d?safe=false", d.ID), body, nil); err != nil {
		return err
	}
	var errs []string
	for _, c := range d.Channels {
		body := map[string]any{
			"caption": c.Caption, "altIcon": c.AltIcon, "hidden": c.Hidden,
			"inheritedLocation": c.InheritedLocation,
		}
		if !c.InheritedLocation {
			body["locationId"] = c.LocationID
		}
		if !emptyConfig(c.Config) {
			body["config"] = c.Config
		}
		if err := cloud.Call("PUT", fmt.Sprintf("/api/v3/channels/%d?safe=false", c.ID), body, nil); err != nil {
			errs = append(errs, fmt.Sprintf("channel %d: %v", c.Number, err))
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// Apply sets the devices that have connected since the last time and
// returns how many are left.
func Apply(db DB, cloud Cloud, b *Bundle, s *State, log io.Writer) (int, error) {
	connected, err := Connected(db, b)
	if err != nil {
		return 0, err
	}
	left := 0
	for _, d := range b.Devices {
		ds := s.Devices[d.ID]
		if ds == nil {
			ds = &DeviceState{Status: "waiting"}
			s.Devices[d.ID] = ds
		}
		if ds.Status == "done" {
			continue
		}
		if !connected[d.ID] {
			left++
			continue
		}
		if err := ApplyDevice(cloud, d); err != nil {
			ds.Status, ds.Error = "error", err.Error()
			fmt.Fprintf(log, "Device %d: %v\n", d.ID, err)
			continue
		}
		ds.Status, ds.Error, ds.Done = "done", "", time.Now().UTC()
		fmt.Fprintf(log, "Device %d moved\n", d.ID)
	}
	return left, nil
}
