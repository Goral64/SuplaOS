package migrate

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeScripts makes objects with IDs new-1, new-2... and records the
// requests.
type fakeScripts struct {
	n    int
	reqs []string
}

func (f *fakeScripts) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.reqs = append(f.reqs, r.Method+" "+r.URL.Path+" "+string(body))
	switch {
	case r.URL.Path == "/api/tokens/new":
		io.WriteString(w, `{"token":"jwt"}`)
	case r.Method == "POST":
		f.n++
		fmt.Fprintf(w, `{"id":"new-%d"}`, f.n)
	case r.URL.Path == "/api/clients":
		io.WriteString(w, `[{"id":"phone-1"}]`)
	case r.Method == "GET":
		io.WriteString(w, `[]`)
	default:
		io.WriteString(w, `{}`)
	}
}

func TestImportScripts(t *testing.T) {
	f := &fakeScripts{}
	ts := httptest.NewServer(f)
	defer ts.Close()
	a, err := ScriptsLogin(ts.URL, "tok", "", "", ts.Client())
	if err != nil {
		t.Fatal(err)
	}
	var b ScriptsBundle
	json.Unmarshal([]byte(`{
	  "sceneGroups": [{"id":"g-old","label":"Dom","collapsed":false}],
	  "thermostats": [{"thermostat":{"id":"t-old","label":"Salon","enabled":true,"activeProfileId":"p-old","target":"heat"},
	    "rooms":[{"id":"r-old","name":"Salon","thermometers":[12085],"heaters":[35990],"coolers":[]}],
	    "profiles":[{"id":"p-old","name":"Dzień","roomsConfig":{"r-old":{"heatFrom":20,"heatTo":21}},"activeOn":[]}]}],
	  "scenes": [
	    {"id":"s-old-2","label":"Wszystko","actions":{"0":"s-old-1;sceneExecute|t-old;thermostatSetProfile,p-old"},"groupId":"g-old","ordinalNumber":1},
	    {"id":"s-old-1","label":"Lampa","actions":{"0":"35990;turnOn","5":"35990;turnOff"},"feedback":"{{state(12085).temperature}}","groupId":null,"ordinalNumber":0,"slug":"abc"}
	  ],
	  "notifications": [{"id":"n-old","label":"Drzwi","condition":"{{state(36146).hi}}","actions":[{"action":"s-old-1;sceneExecute"}],"clientIds":["c1"]}]
	}`), &b)
	r := ImportScripts(a, &b, io.Discard)
	if len(r.Errors) > 0 {
		t.Fatalf("errors %v", r.Errors)
	}
	all := strings.Join(f.reqs, "\n")
	for _, want := range []string{
		`POST /api/scene-groups {"collapsed":false,"label":"Dom"}`,
		`POST /api/thermostats {"label":"Salon","target":"heat"}`,
		`POST /api/thermostats/new-2/thermostat-rooms {"coolers":[],"heaters":[35990],"name":"Salon","thermometers":[12085]}`,
		`POST /api/thermostats/new-2/thermostat-profiles {"activeOn":[],"name":"Dzień","roomsConfig":{"new-3":{"heatFrom":20,"heatTo":21}}}`,
		`PATCH /api/thermostats/new-2 {"activeProfileId":"new-4","enabled":true}`,
		`POST /api/scenes {"actions":{"0":"35990;turnOn","5":"35990;turnOff"},"feedback":"{{state(12085).temperature}}","generateSlug":true,"label":"Lampa"}`,
		`POST /api/scenes {"actions":{"0":"new-5;sceneExecute|new-2;thermostatSetProfile,new-4"},"label":"Wszystko"}`,
		`PATCH /api/scene-groups {"map":[["new-1","new-6"],["default","new-5"]]}`,
		`"actions":[{"action":"new-5;sceneExecute"}]`,
		`"clientIds":["phone-1"]`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %s in\n%s", want, all)
		}
	}
	if r.Counts["scenes"] != 2 || r.Counts["thermostats"] != 1 || r.Counts["notifications"] != 1 {
		t.Errorf("counts %v", r.Counts)
	}
}

func TestScriptsLoginNoUsername(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		io.WriteString(w, `{"message":"Array does not contain an element with key \"username\""}`)
	}))
	defer ts.Close()
	if _, err := ScriptsLogin(ts.URL, "tok", "", "", ts.Client()); err != ErrScriptsNoLogin {
		t.Errorf("%v", err)
	}
}
