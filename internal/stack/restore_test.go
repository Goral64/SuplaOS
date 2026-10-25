package stack

import (
	"bytes"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestFullRestore(t *testing.T) {
	loadRetry = 0
	a := newStack(t)
	must(t, a.Install("supla", map[string]string{"SUPLA_HOST_ADDRESS": "192.168.1.20"}))
	must(t, a.Install("mosquitto", nil))
	fakeCompose(a, "supla-db", "mosquitto")
	a.Stdout = io.Discard
	write(t, filepath.Join(a.Dir(), "var/mysql/ibdata1"), "raw")
	write(t, filepath.Join(a.Dir(), "var/cloud-local/uploads/a.png"), "png")
	write(t, filepath.Join(a.Dir(), "var/mosquitto/mosquitto.db"), "db")
	must(t, os.Symlink("mosquitto.db", filepath.Join(a.Dir(), "var/mosquitto/link")))
	write(t, filepath.Join(a.CADir(), "acme/settings.yaml"), "domain: x\n")
	file := filepath.Join(t.TempDir(), "full.tar.gz.age")
	must(t, a.ExportFile(file, ExportOptions{Version: "0.7.0", Password: "hasło"}))

	// Another device, at another address, with mosquitto of its own.
	b := newStack(t)
	deviceNames = func() ([]string, []net.IP) {
		return []string{"nowy.local", "localhost"}, []net.IP{net.ParseIP("192.168.1.30"), net.ParseIP("127.0.0.1")}
	}
	must(t, b.Install("mosquitto", nil))
	write(t, filepath.Join(b.Dir(), "var/mosquitto/old.db"), "old")
	calls := fakeCompose(b, "mosquitto")
	var out bytes.Buffer
	b.Stdout = &out

	if err := b.Import(file, ImportOptions{Password: "zle", Version: "0.7.0"}); err == nil {
		t.Fatal("wrong password accepted")
	}
	if err := b.Import(file, ImportOptions{Password: "hasło", Version: "0.6.0"}); err == nil || !strings.Contains(err.Error(), "update first") {
		t.Fatalf("older SuplaOS: %v", err)
	}
	if _, err := os.Stat(filepath.Join(b.Dir(), "var/mosquitto/old.db")); err != nil || len(*calls) != 0 {
		t.Fatalf("changed before the checks: %v %v", err, *calls)
	}

	must(t, b.Import(file, ImportOptions{Password: "hasło", Version: "0.7.1~dev.1"}))
	if b.State.Addons["supla"] == nil || b.State.Addons["mosquitto"].Options["MOSQUITTO_PASSWORD"] != a.State.Addons["mosquitto"].Options["MOSQUITTO_PASSWORD"] {
		t.Errorf("state %+v", b.State.Addons)
	}
	caA, _ := os.ReadFile(a.CACertFile())
	caB, _ := os.ReadFile(b.CACertFile())
	if !bytes.Equal(caA, caB) {
		t.Error("CA not restored")
	}
	for name, want := range map[string]string{
		"var/cloud-local/uploads/a.png": "png",
		"var/mosquitto/mosquitto.db":    "db",
	} {
		if got, err := os.ReadFile(filepath.Join(b.Dir(), name)); err != nil || string(got) != want {
			t.Errorf("%s: %q %v", name, got, err)
		}
	}
	if l, err := os.Readlink(filepath.Join(b.Dir(), "var/mosquitto/link")); err != nil || l != "mosquitto.db" {
		t.Errorf("link %q %v", l, err)
	}
	for _, gone := range []string{"var/mosquitto/old.db", "var/mysql/ibdata1"} {
		if _, err := os.Stat(filepath.Join(b.Dir(), gone)); err == nil {
			t.Errorf("%s is there", gone)
		}
	}
	if _, err := os.Stat(filepath.Join(b.CADir(), "acme/settings.yaml")); err != nil {
		t.Error(err)
	}
	// The certificates are made anew for this device, by the same CA.
	if err := verify(t, b, filepath.Join(b.Dir(), "ssl/cloud"), "192.168.1.30"); err != nil {
		t.Errorf("certificate: %v", err)
	}
	want := []string{"stop mosquitto", "start mosquitto", "down --remove-orphans", "up -d supla-db",
		"load supla-db: -- dump of supla-db\n", "up -d --remove-orphans"}
	if !slices.Equal(*calls, want) {
		t.Errorf("compose calls\n%q\nwant\n%q", *calls, want)
	}
	if saved, _ := filepath.Glob(filepath.Join(b.BackupDir(), "suplaos-*.tar.gz")); len(saved) != 1 {
		t.Errorf("backup of the current state: %v", saved)
	}
	if left, _ := filepath.Glob(filepath.Join(b.BackupDir(), ".restore-*")); len(left) > 0 {
		t.Errorf("left: %v", left)
	}
	if !strings.Contains(out.String(), "SUPLA_HOST_ADDRESS is 192.168.1.20") {
		t.Errorf("no warning about the old address:\n%s", out.String())
	}
	if env := readEnv(t, b, ".env"); env["SUPLA_HOST_ADDRESS"] != "192.168.1.20" {
		t.Errorf(".env: %v", env)
	}
}

func TestRestorePath(t *testing.T) {
	s := newStack(t)
	m := &Manifest{Addons: []string{"supla", "mosquitto"}}
	for name, want := range map[string]string{
		"state.yaml":                 "state.yaml",
		"ca/":                        "ca",
		"ca/acme/cert.crt":           "ca/acme/cert.crt",
		"dumps/supla.sql":            "backups/.restore-supla.sql",
		"stack/var/mosquitto/a.db":   "stack/var/mosquitto/a.db",
		"stack/var/cloud-local/":     "stack/var/cloud-local",
		"stack/ssl/server/cert.crt":  "stack/ssl/server/cert.crt",
		"stack/docker-compose.yml":   "",
		"stack/.env":                 "",
		"stack/var/zigbee2mqtt/a":    "",
		"stack/var/mosquitto/../../": "",
		"../etc/passwd":              "",
		"dumps/mosquitto.sql":        "",
		"cache/supla/x":              "",
	} {
		got, err := restorePath(name, m, s.Catalog)
		if got != want || (want == "") != (err != nil) {
			t.Errorf("%s: %q %v", name, got, err)
		}
	}
}

func TestVersionNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.7.0", "0.6.0", true}, {"0.6.0", "0.7.0", false}, {"0.7.0", "0.7.0", false},
		{"0.7.1~dev.202610251219", "0.7.1", false}, {"0.7.1", "0.7.0~dev.1", true}, {"0.10.0", "0.9.9", true},
		{"1.0", "1.0.0", false},
	} {
		if got := versionNewer(c.a, c.b); got != c.want {
			t.Errorf("versionNewer(%s, %s) = %v", c.a, c.b, got)
		}
	}
}

func TestContainsAddress(t *testing.T) {
	for v, want := range map[string]bool{
		"192.168.1.2":                true,
		"https://192.168.1.2":        true,
		"192.168.1.2:8444":           true,
		"192.168.1.25":               false,
		"10.192.168.1.2":             false,
		"x 192.168.1.25 192.168.1.2": true,
	} {
		if containsAddress(v, "192.168.1.2") != want {
			t.Errorf("%q", v)
		}
	}
}
