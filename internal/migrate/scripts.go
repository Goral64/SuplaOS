package migrate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// SUPLA Scripts (fracz/supla-scripts) keeps its own objects: scenes and
// their groups, thermostats with rooms and profiles, notifications. They
// refer to channels by the IDs the move keeps, and to each other by UUIDs,
// which are new in the target and replaced in the copies.

// ScriptsAPI is the API of a SUPLA Scripts instance, logged in.
type ScriptsAPI struct {
	URL    string
	jwt    string
	client *http.Client
}

// ErrScriptsNoLogin: the account of the source SUPLA Scripts logs in only
// through Supla Cloud; a personal token logs in once it has a username.
var ErrScriptsNoLogin = errors.New("the SUPLA Scripts account has no username yet: set a username and password there (Other ways to log in) and try again")

// ScriptsLogin logs in with a personal access token of the Supla Cloud the
// instance works with (it needs the scope to execute actions). With
// username and password an account is made when there is none.
func ScriptsLogin(url, personalToken, username, password string, client *http.Client) (*ScriptsAPI, error) {
	if client == nil {
		client = &http.Client{Timeout: time.Minute}
	}
	a := &ScriptsAPI{URL: strings.TrimSuffix(url, "/"), client: client}
	body := map[string]string{"personalToken": personalToken}
	if username != "" {
		body["username"], body["password"] = username, password
	}
	var res struct {
		Token string `json:"token"`
	}
	if err := a.call("POST", "/api/tokens/new", body, &res); err != nil {
		if strings.Contains(err.Error(), "username") {
			return nil, ErrScriptsNoLogin
		}
		return nil, err
	}
	a.jwt = res.Token
	return a, nil
}

func (a *ScriptsAPI) call(method, path string, body, out any) error {
	var in io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		in = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, a.URL+path, in)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if a.jwt != "" {
		req.Header.Set("Authorization", "Bearer "+a.jwt)
	}
	res, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if res.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		json.Unmarshal(data, &e)
		if e.Message == "" {
			e.Message = res.Status
		}
		return fmt.Errorf("SUPLA Scripts: %s %s: %s", method, path, e.Message)
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// object is an object of SUPLA Scripts as its API gives it.
type object = map[string]any

// ScriptsThermostat is a thermostat with its rooms and profiles.
type ScriptsThermostat struct {
	Thermostat object   `json:"thermostat"`
	Rooms      []object `json:"rooms"`
	Profiles   []object `json:"profiles"`
}

// ScriptsBundle is what is read from the source SUPLA Scripts.
type ScriptsBundle struct {
	URL           string              `json:"url"`
	SceneGroups   []object            `json:"sceneGroups"`
	Scenes        []object            `json:"scenes"`
	Thermostats   []ScriptsThermostat `json:"thermostats"`
	Notifications []object            `json:"notifications"`
}

// ExportScripts reads the objects of the account.
func (a *ScriptsAPI) ExportScripts() (*ScriptsBundle, error) {
	b := &ScriptsBundle{URL: a.URL}
	for path, out := range map[string]*[]object{
		"/api/scene-groups": &b.SceneGroups, "/api/scenes": &b.Scenes, "/api/notifications": &b.Notifications,
	} {
		if err := a.call("GET", path, nil, out); err != nil {
			return nil, err
		}
	}
	var thermostats []object
	if err := a.call("GET", "/api/thermostats", nil, &thermostats); err != nil {
		return nil, err
	}
	for _, t := range thermostats {
		id := fmt.Sprint(t["id"])
		st := ScriptsThermostat{Thermostat: t}
		if err := a.call("GET", "/api/thermostats/"+id+"/thermostat-rooms", nil, &st.Rooms); err != nil {
			return nil, err
		}
		if err := a.call("GET", "/api/thermostats/"+id+"/thermostat-profiles", nil, &st.Profiles); err != nil {
			return nil, err
		}
		b.Thermostats = append(b.Thermostats, st)
	}
	return b, nil
}

// ScriptsResult is what ImportScripts made and what failed.
type ScriptsResult struct {
	Source string         `json:"source"`
	Time   time.Time      `json:"time"`
	Counts map[string]int `json:"counts"`
	Errors []string       `json:"errors"`
}

// pick copies the fields an object is made with.
func pick(o object, fields ...string) object {
	out := object{}
	for _, f := range fields {
		if v, ok := o[f]; ok {
			out[f] = v
		}
	}
	return out
}

// ids replaces the UUIDs of the source with those of the target in v, a
// JSON value: in the commands of scenes ("<uuid>;sceneExecute") and in the
// rooms of thermostat profiles.
type idMap map[string]string

func (m idMap) apply(v any) any {
	data, err := json.Marshal(v)
	if err != nil || len(m) == 0 {
		return v
	}
	s := string(data)
	// The longest first, so that no ID is a part of another replaced one.
	olds := make([]string, 0, len(m))
	for old := range m {
		olds = append(olds, old)
	}
	sort.Slice(olds, func(i, j int) bool { return len(olds[i]) > len(olds[j]) })
	for _, old := range olds {
		s = strings.ReplaceAll(s, old, m[old])
	}
	var out any
	if json.Unmarshal([]byte(s), &out) != nil {
		return v
	}
	return out
}

func (m idMap) add(old any, made object) {
	if o, n := fmt.Sprint(old), fmt.Sprint(made["id"]); o != "" && o != "<nil>" && n != "<nil>" {
		m[o] = n
	}
}

// byLabel finds an object of the target with the same label (or name).
func byLabel(list []object, key string, v any) object {
	for _, o := range list {
		if o[key] != nil && o[key] == v {
			return o
		}
	}
	return nil
}

// ImportScripts makes the objects of the bundle in the target SUPLA
// Scripts. Objects with the label of one already there are not made again,
// so it can be run again, e.g. for the notifications once a phone is
// registered: phones (clients) are not moved, they are registered again in
// the app, and notifications go to all phones of the target.
func ImportScripts(a *ScriptsAPI, b *ScriptsBundle, log io.Writer) *ScriptsResult {
	r := &ScriptsResult{Source: b.URL, Time: time.Now().UTC(), Counts: map[string]int{}}
	fail := func(format string, args ...any) { r.Errors = append(r.Errors, fmt.Sprintf(format, args...)) }
	ids := idMap{}
	have, err := a.ExportScripts()
	if err != nil {
		fail("reading SUPLA Scripts here: %v", err)
		return r
	}
	var clients []object
	if err := a.call("GET", "/api/clients", nil, &clients); err != nil {
		fail("phones of SUPLA Scripts here: %v", err)
	}
	clientIDs := []string{}
	for _, c := range clients {
		clientIDs = append(clientIDs, fmt.Sprint(c["id"]))
	}

	for _, g := range b.SceneGroups {
		if old := byLabel(have.SceneGroups, "label", g["label"]); old != nil {
			ids.add(g["id"], old)
			continue
		}
		var made object
		if err := a.call("POST", "/api/scene-groups", pick(g, "label", "collapsed"), &made); err != nil {
			fail("scene group %v: %v", g["label"], err)
			continue
		}
		ids.add(g["id"], made)
		r.Counts["sceneGroups"]++
	}

	for _, t := range b.Thermostats {
		if old := thermostatByLabel(have.Thermostats, t.Thermostat["label"]); old != nil {
			ids.add(t.Thermostat["id"], old.Thermostat)
			for _, room := range t.Rooms {
				if o := byLabel(old.Rooms, "name", room["name"]); o != nil {
					ids.add(room["id"], o)
				}
			}
			for _, p := range t.Profiles {
				if o := byLabel(old.Profiles, "name", p["name"]); o != nil {
					ids.add(p["id"], o)
				}
			}
			continue
		}
		var made object
		if err := a.call("POST", "/api/thermostats", pick(t.Thermostat, "label", "target"), &made); err != nil {
			fail("thermostat %v: %v", t.Thermostat["label"], err)
			continue
		}
		ids.add(t.Thermostat["id"], made)
		tid := fmt.Sprint(made["id"])
		for _, room := range t.Rooms {
			var mr object
			if err := a.call("POST", "/api/thermostats/"+tid+"/thermostat-rooms", pick(room, "name", "thermometers", "heaters", "coolers"), &mr); err != nil {
				fail("room %v: %v", room["name"], err)
				continue
			}
			ids.add(room["id"], mr)
		}
		for _, p := range t.Profiles {
			var mp object
			body := ids.apply(pick(p, "name", "roomsConfig", "activeOn"))
			if err := a.call("POST", "/api/thermostats/"+tid+"/thermostat-profiles", body, &mp); err != nil {
				fail("thermostat profile %v: %v", p["name"], err)
				continue
			}
			ids.add(p["id"], mp)
		}
		patch := object{"enabled": t.Thermostat["enabled"]}
		if active, ok := ids[fmt.Sprint(t.Thermostat["activeProfileId"])]; ok {
			patch["activeProfileId"] = active
		}
		if err := a.call("PATCH", "/api/thermostats/"+tid, patch, nil); err != nil {
			fail("thermostat %v: %v", t.Thermostat["label"], err)
		}
		r.Counts["thermostats"]++
	}

	// A scene may run other scenes: made after them, in passes.
	made := map[string]string{}
	left := []object{}
	for _, s := range b.Scenes {
		if old := byLabel(have.Scenes, "label", s["label"]); old != nil {
			ids.add(s["id"], old)
			continue
		}
		left = append(left, s)
	}
	for len(left) > 0 {
		var next []object
		for _, s := range left {
			if runsSceneOf(s, left, ids) {
				next = append(next, s)
				continue
			}
			body := pick(s, "label", "feedback", "voiceTriggers", "condition", "trigger", "intervals", "notifications", "enabled", "actionTriggers")
			if s["actions"] != nil {
				body["actions"] = ids.apply(s["actions"])
			}
			if s["slug"] != nil && s["slug"] != "" {
				body["generateSlug"] = true
			}
			var m object
			if err := a.call("POST", "/api/scenes", body, &m); err != nil {
				fail("scene %v: %v", s["label"], err)
				continue
			}
			ids.add(s["id"], m)
			made[fmt.Sprint(s["id"])] = fmt.Sprint(m["id"])
			r.Counts["scenes"]++
		}
		if len(next) == len(left) {
			for _, s := range next {
				fail("scene %v: it runs a scene that runs it back", s["label"])
			}
			break
		}
		left = next
	}
	// The order of scenes and their groups.
	if len(made) > 0 {
		groups := map[string][]object{}
		for _, s := range b.Scenes {
			if _, ok := made[fmt.Sprint(s["id"])]; ok {
				g := "default"
				if s["groupId"] != nil {
					g = fmt.Sprint(s["groupId"])
				}
				groups[g] = append(groups[g], s)
			}
		}
		var order [][]string
		keys := []string{}
		for _, g := range b.SceneGroups {
			keys = append(keys, fmt.Sprint(g["id"]))
		}
		keys = append(keys, "default")
		for _, g := range keys {
			newG := "default"
			if g != "default" {
				var ok bool
				if newG, ok = ids[g]; !ok {
					continue
				}
			}
			scenes := groups[g]
			sort.SliceStable(scenes, func(i, j int) bool { return num(scenes[i]["ordinalNumber"]) < num(scenes[j]["ordinalNumber"]) })
			row := []string{newG}
			for _, s := range scenes {
				row = append(row, made[fmt.Sprint(s["id"])])
			}
			order = append(order, row)
		}
		if err := a.call("PATCH", "/api/scene-groups", object{"map": order}, nil); err != nil {
			fail("order of scenes: %v", err)
		}
	}

	for _, n := range b.Notifications {
		if byLabel(have.Notifications, "label", n["label"]) != nil {
			continue
		}
		if len(clientIDs) == 0 {
			fail("notification %v: no phone is registered in SUPLA Scripts here; register the app and move again", n["label"])
			continue
		}
		body := ids.apply(pick(n, "label", "condition", "intervals", "header", "message", "sound", "vibrate", "flash",
			"cancellable", "ongoing", "awake", "actions", "retryInterval", "icon", "speech", "displayIfDisconnected", "onlyInTime")).(object)
		body["clientIds"] = clientIDs
		if err := a.call("POST", "/api/notifications", body, nil); err != nil {
			fail("notification %v: %v", n["label"], err)
			continue
		}
		r.Counts["notifications"]++
	}
	for _, e := range r.Errors {
		fmt.Fprintln(log, "SUPLA Scripts, not moved:", e)
	}
	return r
}

// runsSceneOf tells whether scene s runs one of the scenes in waiting
// (other than itself) that is not made yet.
func runsSceneOf(s object, waiting []object, ids idMap) bool {
	acts, _ := json.Marshal(s["actions"])
	for _, o := range waiting {
		id := fmt.Sprint(o["id"])
		if id == fmt.Sprint(s["id"]) {
			continue
		}
		if _, ok := ids[id]; !ok && strings.Contains(string(acts), id) {
			return true
		}
	}
	return false
}

func thermostatByLabel(list []ScriptsThermostat, label any) *ScriptsThermostat {
	for i := range list {
		if list[i].Thermostat["label"] == label {
			return &list[i]
		}
	}
	return nil
}

func num(v any) float64 {
	f, _ := v.(float64)
	return f
}
