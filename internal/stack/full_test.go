package stack

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fakeCompose answers docker compose in tests: database dumps and the
// running services, and records the other calls.
func fakeCompose(s *Stack, running ...string) *[]string {
	var calls []string
	s.composeHook = func(stdin io.Reader, stdout io.Writer, args ...string) error {
		switch {
		case len(args) > 2 && args[0] == "exec" && stdin != nil:
			b, _ := io.ReadAll(stdin)
			calls = append(calls, "load "+args[2]+": "+string(b))
		case len(args) > 2 && args[0] == "exec":
			io.WriteString(stdout, "-- dump of "+args[2]+"\n")
		case len(args) > 0 && args[0] == "ps":
			io.WriteString(stdout, strings.Join(running, "\n")+"\n")
		default:
			calls = append(calls, strings.Join(args, " "))
		}
		return nil
	}
	return &calls
}

func write(t *testing.T, path, content string) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Dir(path), 0o755))
	must(t, os.WriteFile(path, []byte(content), 0o640))
}

// readFull returns the entries of a full backup with the content of files.
func readFull(t *testing.T, data []byte, password string) (*Manifest, map[string]string) {
	t.Helper()
	tr, m, err := OpenFullBackup(bytes.NewReader(data), password)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(tr)
		files[hdr.Name] = string(b)
		if hdr.Typeflag == tar.TypeSymlink {
			files[hdr.Name] = "-> " + hdr.Linkname
		}
	}
	return m, files
}

func TestFullBackup(t *testing.T) {
	s := newStack(t)
	must(t, s.Install("supla", nil))
	must(t, s.Install("mosquitto", nil))
	calls := fakeCompose(s, "supla-db", "mosquitto")
	write(t, filepath.Join(s.Dir(), "var/mysql/ibdata1"), "raw")
	write(t, filepath.Join(s.Dir(), "var/cloud-logs/prod.log"), "log")
	write(t, filepath.Join(s.Dir(), "var/cloud-local/uploads/a.png"), "png")
	write(t, filepath.Join(s.Dir(), "var/mosquitto/mosquitto.db"), "db")
	must(t, os.Symlink("mosquitto.db", filepath.Join(s.Dir(), "var/mosquitto/link")))
	write(t, filepath.Join(s.CADir(), "panel/server.crt"), "panel")
	write(t, filepath.Join(s.CADir(), "acme/settings.yaml"), "domain: x\n")

	var buf bytes.Buffer
	_, err := s.Export(&buf, ExportOptions{Version: "0.7.0", Password: "tajne hasło"})
	must(t, err)
	if !bytes.HasPrefix(buf.Bytes(), []byte(ageHeader)) {
		t.Fatal("not encrypted")
	}
	if _, _, err := OpenFullBackup(bytes.NewReader(buf.Bytes()), "inne"); !errors.Is(err, ErrPassword) {
		t.Errorf("wrong password: %v", err)
	}
	if _, _, err := OpenFullBackup(bytes.NewReader(buf.Bytes()), ""); !errors.Is(err, ErrPassword) {
		t.Errorf("no password: %v", err)
	}

	m, files := readFull(t, buf.Bytes(), "tajne hasło")
	if m.Version != "0.7.0" || m.Format != FullFormat || !slices.Equal(m.Addons, []string{"supla", "mosquitto"}) && !slices.Equal(m.Addons, []string{"mosquitto", "supla"}) {
		t.Errorf("manifest %+v", m)
	}
	for _, name := range []string{"state.yaml", "ca/ca.crt", "ca/ca.key", "ca/acme/settings.yaml",
		"stack/var/cloud-local/uploads/a.png", "stack/ssl/server/", "stack/var/mosquitto/mosquitto.db",
		"stack/ssl/mosquitto/server.key"} {
		if _, ok := files[name]; !ok {
			t.Errorf("%s missing", name)
		}
	}
	for name := range files {
		for _, bad := range []string{"ca/panel", "stack/var/mysql", "stack/var/cloud-logs", "stack/.env", "stack/docker-compose", "cache/", "backups/"} {
			if strings.HasPrefix(name, bad) {
				t.Errorf("%s in the backup", name)
			}
		}
	}
	if files["dumps/supla.sql"] != "-- dump of supla-db\n" || files["stack/var/mosquitto/link"] != "-> mosquitto.db" {
		t.Errorf("dump %q, link %q", files["dumps/supla.sql"], files["stack/var/mosquitto/link"])
	}
	if !strings.Contains(files["state.yaml"], "mosquitto") {
		t.Errorf("state.yaml: %s", files["state.yaml"])
	}
	if !slices.Equal(*calls, []string{"stop mosquitto", "start mosquitto"}) {
		t.Errorf("compose calls %v", *calls)
	}
	if left, _ := filepath.Glob(filepath.Join(s.BackupDir(), ".dump-*")); len(left) > 0 {
		t.Errorf("temporary files left: %v", left)
	}
}

func TestFullBackupPlain(t *testing.T) {
	s := newStack(t)
	must(t, s.Install("mosquitto", nil))
	calls := fakeCompose(s) // mosquitto not running: nothing to stop
	path, err := s.SaveFullBackup(ExportOptions{Version: "0.7.0"})
	must(t, err)
	if !strings.HasSuffix(path, ".tar.gz") || !strings.HasPrefix(filepath.Base(path), "suplaos-") {
		t.Errorf("name %s", path)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", st.Mode())
	}
	data, err := os.ReadFile(path)
	must(t, err)
	if _, files := readFull(t, data, ""); files["state.yaml"] == "" {
		t.Error("no state.yaml")
	}
	if len(*calls) != 0 {
		t.Errorf("compose calls %v", *calls)
	}
}

func TestFullBackupFormat(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	must(t, addBytes(tw, "manifest.json", []byte(`{"format": 2}`)))
	tw.Close()
	gz.Close()
	if _, _, err := OpenFullBackup(&buf, ""); err == nil || !strings.Contains(err.Error(), "update it first") {
		t.Errorf("newer format: %v", err)
	}
	if _, _, err := OpenFullBackup(strings.NewReader("not a backup"), ""); err == nil {
		t.Error("garbage accepted")
	}
}
