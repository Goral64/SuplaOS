package panel

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goral64/SuplaOS/internal/stack"
)

func TestFullBackupUpload(t *testing.T) {
	s := newServer(t)
	st, err := stack.Open(t.TempDir(), "../../addons")
	if err != nil {
		t.Fatal(err)
	}
	path, err := st.SaveFullBackup(stack.ExportOptions{Version: "0.7.0"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)

	upload := func(name string) (int, string) {
		r := httptest.NewRequest("POST", "/api/system-backup/upload?name="+name, bytes.NewReader(data))
		w := httptest.NewRecorder()
		s.uploadFullBackup(w, r)
		var res map[string]string
		json.Unmarshal(w.Body.Bytes(), &res)
		return w.Code, res["name"]
	}
	base := filepath.Base(path)
	if code, name := upload(base); code != 200 || name != base {
		t.Errorf("upload: %d %s", code, name)
	}
	if code, name := upload(base); code != 200 || name != strings.TrimSuffix(base, ".tar.gz")+"-1.tar.gz" {
		t.Errorf("second upload: %d %s", code, name)
	}
	if code, name := upload("kopia.tar.gz"); code != 200 || !strings.HasPrefix(name, "suplaos-upload-") {
		t.Errorf("other name: %d %s", code, name)
	}
	if code, _ := upload("supla-1.sql.gz"); code != http.StatusBadRequest {
		t.Errorf("not a full backup: %d", code)
	}

	check := func(name string) (int, string) {
		body, _ := json.Marshal(fullBackupRequest{Name: name})
		w := httptest.NewRecorder()
		s.checkFullBackup(w, httptest.NewRequest("POST", "/api/system-backup/check", bytes.NewReader(body)))
		return w.Code, w.Body.String()
	}
	if code, body := check(base); code != 200 || !strings.Contains(body, `"version":"0.7.0"`) {
		t.Errorf("check: %d %s", code, body)
	}
	for _, name := range []string{"../x.tar.gz", "supla-1.sql.gz", "suplaos-none.tar.gz"} {
		if code, _ := check(name); code != http.StatusBadRequest {
			t.Errorf("check %s: %d", name, code)
		}
	}
}
