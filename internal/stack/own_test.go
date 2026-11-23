package stack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Goral64/SuplaOS/internal/addon"
)

func TestOwnEnvAndCompose(t *testing.T) {
	s := newStack(t)
	must(t, s.Install("supla", map[string]string{"SUPLA_HOST_ADDRESS": "192.168.1.5"}))
	inst := s.State.Addons["supla"]
	inst.OwnEnv = "# own\nSUPLA_PROTOCOL_VERSION=99\nMAILER_DSN_X=x\n"
	inst.OwnCompose = "services:\n  supla-cloud:\n    image: example/other:1\n"
	must(t, s.Apply())
	env, own := readEnv(t, s, ".env"), readEnv(t, s, "supla.env")
	if env["SUPLA_PROTOCOL_VERSION"] != "99" || own["SUPLA_PROTOCOL_VERSION"] != "99" || own["MAILER_DSN_X"] != "x" {
		t.Errorf("own variables: %v", own)
	}
	// Before the pinned images, which win.
	files := strings.Split(env["COMPOSE_FILE"], ":")
	if files[len(files)-2] != "docker-compose.supla-own.yml" || files[len(files)-1] != "docker-compose.supla-images.yml" {
		t.Errorf("COMPOSE_FILE = %s", env["COMPOSE_FILE"])
	}
	if data, _ := os.ReadFile(filepath.Join(s.Dir(), "docker-compose.supla-own.yml")); string(data) != inst.OwnCompose {
		t.Errorf("own compose file: %q", data)
	}
	// Without them the file goes away.
	inst.OwnEnv, inst.OwnCompose = "", ""
	must(t, s.Apply())
	if _, err := os.Stat(filepath.Join(s.Dir(), "docker-compose.supla-own.yml")); err == nil {
		t.Error("own compose file left behind")
	}
	if readEnv(t, s, ".env")["SUPLA_PROTOCOL_VERSION"] == "99" {
		t.Error("own variable left behind")
	}

	a := s.Catalog["supla"]
	for _, bad := range []string{"DATABASE_IMAGE=mariadb:13", "PORT_HTTPS=444", "COMPOSE_FILE=x", "TZ=UTC", "1A=b", "A=b'c", "no equals"} {
		if _, err := s.ownEnv(a, bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if _, err := s.ownEnv(a, "SUPLA_X=1\n\n# c\nexport SUPLA_Y=\"2\""); err != nil {
		t.Error(err)
	}
}

func TestEnvChanges(t *testing.T) {
	s := newStack(t)
	must(t, s.Install("supla", nil))
	s.State.Addons["supla"].OwnEnv = "B=own"
	a := s.Catalog["supla"]
	data, _ := s.source(a, a.EnvBase)
	installed, _ := parseEnv(data)
	release := append(installed[:0:0], installed...)
	// One removed, one changed, one new, the database left out.
	removed := release[0]
	release = release[1:]
	for i := range release {
		if release[i].Key == "DATABASE_IMAGE" {
			release[i].Value = "mariadb:99"
		}
	}
	release = append(release, addon.KV{Key: "NEW_ONE", Value: "1"})
	changes := s.envChanges(a, release)
	if len(changes) != 2 || !changes[0].New || changes[0].Key != "NEW_ONE" || !changes[1].Removed || changes[1].Key != removed.Key {
		t.Errorf("%+v", changes)
	}
}
