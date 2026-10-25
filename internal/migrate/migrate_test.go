package migrate

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func sp(s string) *string { return &s }

func bundle() *Bundle {
	return &Bundle{
		Email: "jan@example.com",
		Locations: []Location{{ID: 50, Caption: sp("Dom 'główny'"), Password: "abc", Enabled: true,
			AccessIDs: []struct {
				ID int `json:"id"`
			}{{ID: 60}}}},
		AccessIDs: []AccessID{{ID: 60, Caption: sp("Telefon"), Password: "pwd", Enabled: true}},
		Devices: []Device{{ID: 700, GUID: "0011aabb-ccdd-eeff-0011-223344556677", Name: sp("Przekaźnik"), Enabled: true,
			LocationID: 50, Config: json.RawMessage(`{"statusLed":"ON_WHEN_CONNECTED"}`),
			Channels: []Channel{
				{ID: 9001, Number: 0, TypeID: 2900, FunctionID: 130, Caption: sp("Lampa"), InheritedLocation: true,
					Config: json.RawMessage(`{"relatedMeterChannelId":9002}`)},
				{ID: 9002, Number: 1, TypeID: 5000, FunctionID: 310, LocationID: 50, Config: json.RawMessage(`{}`)},
			}}},
	}
}

func TestSkeletonSQL(t *testing.T) {
	q, err := SkeletonSQL(bundle(), 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"START TRANSACTION;\n",
		"INSERT INTO supla_location (id, user_id, password, caption, enabled) VALUES (50, 3, _utf8mb4 X'616263', _utf8mb4 X'446f6d202767c582c3b3776e7927', 1);",
		"INSERT INTO supla_rel_aidloc (location_id, access_id) VALUES (50, 60);",
		"(700, 50, NULL, 3, X'0011AABBCCDDEEFF0011223344556677', _utf8mb4 X'",
		"UTC_TIMESTAMP(), NULL, 0, NULL, NULL);",
		"(9001, 700, 3, 0, _utf8mb4 X'4c616d7061', 2900, 130, 0, 0, 0, 0, 0, NULL, 0);",
		"(9002, 700, 3, 1, NULL, 5000, 310, 0, 0, 0, 0, 0, 50, 0);",
		"COMMIT;\n",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("missing %q in\n%s", want, q)
		}
	}
	if strings.Contains(q, "'główny'") {
		t.Error("a string not as hex")
	}
	b := bundle()
	b.Devices[0].GUID = "nie-guid"
	if _, err := SkeletonSQL(b, 3); err == nil {
		t.Error("bad GUID accepted")
	}
}

// fakeDB answers the queries of the tests by their start.
type fakeDB struct {
	rows map[string][][]string
	ran  []string
}

func (f *fakeDB) Query(sql string) ([][]string, error) {
	f.ran = append(f.ran, sql)
	for prefix, rows := range f.rows {
		if strings.HasPrefix(sql, prefix) {
			return rows, nil
		}
	}
	return nil, nil
}

func TestPrepare(t *testing.T) {
	db := &fakeDB{rows: map[string][][]string{"SELECT id FROM supla_user": {{"3"}}}}
	if err := Prepare(db, bundle(), io.Discard); err != nil {
		t.Fatal(err)
	}
	if last := db.ran[len(db.ran)-1]; !strings.HasPrefix(last, "START TRANSACTION") {
		t.Errorf("skeleton not run: %q", last)
	}

	db = &fakeDB{rows: map[string][][]string{"SELECT id FROM supla_user": {{"3"}}, "SELECT 'location'": {{"device", "700"}}}}
	if err := Prepare(db, bundle(), io.Discard); err == nil || !strings.Contains(err.Error(), "device 700") {
		t.Errorf("conflict: %v", err)
	}
	db = &fakeDB{}
	if err := Prepare(db, bundle(), io.Discard); err == nil || !strings.Contains(err.Error(), "jan@example.com") {
		t.Errorf("no account: %v", err)
	}
}

type fakeCloud struct {
	calls []string
	fail  string
}

func (f *fakeCloud) Call(method, path string, body, out any) error {
	data, _ := json.Marshal(body)
	f.calls = append(f.calls, method+" "+path+" "+string(data))
	if f.fail != "" && strings.Contains(path, f.fail) {
		return errors.New("refused")
	}
	return nil
}

func TestApply(t *testing.T) {
	b := bundle()
	s := &State{Devices: map[int]*DeviceState{}}
	cloud := &fakeCloud{}
	db := &fakeDB{}
	left, err := Apply(db, cloud, b, s, io.Discard)
	if err != nil || left != 1 || len(cloud.calls) != 0 || s.Devices[700].Status != "waiting" {
		t.Fatalf("not connected: %d %v %v", left, err, cloud.calls)
	}
	db.rows = map[string][][]string{"SELECT id FROM supla_iodevice WHERE last_connected": {{"700"}}}
	left, err = Apply(db, cloud, b, s, io.Discard)
	if err != nil || left != 0 || s.Devices[700].Status != "done" {
		t.Fatalf("connected: %d %v %+v", left, err, s.Devices[700])
	}
	want := []string{
		`PUT /api/v3/iodevices/700?safe=false {"comment":null,"config":{"statusLed":"ON_WHEN_CONNECTED"}}`,
		`PUT /api/v3/channels/9001?safe=false {"altIcon":0,"caption":"Lampa","config":{"relatedMeterChannelId":9002},"hidden":false,"inheritedLocation":true}`,
		`PUT /api/v3/channels/9002?safe=false {"altIcon":0,"caption":null,"hidden":false,"inheritedLocation":false,"locationId":50}`,
	}
	if strings.Join(cloud.calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("calls\n%s\nwant\n%s", strings.Join(cloud.calls, "\n"), strings.Join(want, "\n"))
	}
	// Done devices are not set again.
	cloud.calls = nil
	Apply(db, cloud, b, s, io.Discard)
	if len(cloud.calls) != 0 {
		t.Errorf("set again: %v", cloud.calls)
	}
	s = &State{Devices: map[int]*DeviceState{}}
	Apply(db, &fakeCloud{fail: "channels/9002"}, b, s, io.Discard)
	if s.Devices[700].Status != "error" || !strings.Contains(s.Devices[700].Error, "channel 1") {
		t.Errorf("error state %+v", s.Devices[700])
	}
}

func TestNewSource(t *testing.T) {
	tok := "abc123." + base64.StdEncoding.EncodeToString([]byte("https://svr99.supla.org"))
	s, err := NewSource(tok)
	if err != nil || s.Server != "https://svr99.supla.org" {
		t.Errorf("%v %v", s, err)
	}
	for _, bad := range []string{"abc", "abc.!!!", "abc." + base64.StdEncoding.EncodeToString([]byte("http://x"))} {
		if _, err := NewSource(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
