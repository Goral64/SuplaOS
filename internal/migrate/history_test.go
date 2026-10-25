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

func items(t *testing.T, s string) []map[string]json.RawMessage {
	t.Helper()
	var out []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestInsertSQL(t *testing.T) {
	ic := logKinds(320)[0]
	q, err := insertSQL(ic, 5, items(t, `[{"date_timestamp":1700000000,"counter":1234,"calculated_value":1.234}]`))
	if err != nil || q != "INSERT IGNORE INTO supla_ic_log (channel_id, `date`, `counter`, `calculated_value`) VALUES\n(5, FROM_UNIXTIME(1700000000), 1234, 1234);\n" {
		t.Errorf("ic: %v\n%s", err, q)
	}
	th := logKinds(400)[0]
	q, _ = insertSQL(th, 6, items(t, `[{"date_timestamp":1,"on":true,"measured_temperature":21.5,"preset_temperature":null}]`))
	if !strings.Contains(q, "(6, FROM_UNIXTIME(1), 1, 21.5, NULL);") || !strings.Contains(q, "`on`") {
		t.Errorf("thermostat:\n%s", q)
	}
	ab := logKinds(310)[1]
	q, _ = insertSQL(ab, 7, items(t, `[{"date_timestamp":2,"phaseNo":1,"countTotal":3,"countAbove":0,"countBelow":0,"secAbove":0,"secBelow":0,"maxSecAbove":0,"maxSecBelow":0,"minVoltage":229.5,"maxVoltage":231,"avgVoltage":230.1,"measurementTimeSec":600}]`))
	if !strings.Contains(q, "`phase_no`, `count_total`") || !strings.Contains(q, "(7, FROM_UNIXTIME(2), 1, 3, 0, 0, 0, 0, 0, 0, 229.5, 231, 230.1, 600);") {
		t.Errorf("aberrations:\n%s", q)
	}
	if _, err := insertSQL(th, 6, items(t, `[{"date_timestamp":1,"on":"x"}]`)); err == nil {
		t.Error("a string accepted as a value")
	}
	// 2500 items: three statements.
	var many []string
	for i := 0; i < 2500; i++ {
		many = append(many, fmt.Sprintf(`{"date_timestamp":%d,"temperature":20}`, i+1))
	}
	q, _ = insertSQL(logKinds(40)[0], 8, items(t, "["+strings.Join(many, ",")+"]"))
	if strings.Count(q, "INSERT IGNORE") != 3 || strings.Count(q, ";\n") != 3 {
		t.Errorf("batches: %d inserts", strings.Count(q, "INSERT IGNORE"))
	}
}

func TestImportHistory(t *testing.T) {
	var asked []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.URL.Path+"?"+r.URL.RawQuery)
		if strings.Contains(r.URL.Path, "/channels/9/") {
			http.NotFound(w, r) // deleted in the source
			return
		}
		if r.URL.Query().Get("offset") != "0" {
			io.WriteString(w, "[]")
			return
		}
		io.WriteString(w, `[{"date_timestamp":1700000600,"temperature":21.25},{"date_timestamp":1700001200,"temperature":21.5}]`)
	}))
	defer ts.Close()
	src := &Source{Server: ts.URL, token: "t", client: ts.Client()}
	b := &Bundle{Devices: []Device{{ID: 1, Channels: []Channel{
		{ID: 9, FunctionID: 40},   // thermometer deleted in the source
		{ID: 10, FunctionID: 40},  // thermometer: logs
		{ID: 11, FunctionID: 130}, // power switch: none
	}}}}
	db := &fakeDB{rows: map[string][][]string{"SET time_zone = '+00:00'; SELECT": {{"1700000000"}}}}
	n, err := ImportHistory(src, db, b, io.Discard)
	if err != nil || n != 2 {
		t.Fatalf("%d %v", n, err)
	}
	if len(asked) != 2 || !strings.Contains(asked[1], "/api/v3/channels/10/measurement-logs?") ||
		!strings.Contains(asked[1], "afterTimestamp=1700000000") || !strings.Contains(asked[1], "order=ASC") {
		t.Errorf("asked %v", asked)
	}
	last := db.ran[len(db.ran)-1]
	if !strings.HasPrefix(last, "SET time_zone = '+00:00';\nINSERT IGNORE INTO supla_temperature_log") ||
		!strings.Contains(last, "(10, FROM_UNIXTIME(1700001200), 21.5);") {
		t.Errorf("insert:\n%s", last)
	}
}
