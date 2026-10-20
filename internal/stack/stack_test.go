package stack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newStack opens a stack in a temporary directory with the repository
// catalog. Files the supla add-on downloads are taken from testdata.
func newStack(t *testing.T) *Stack {
	t.Helper()
	root := t.TempDir()
	s, err := Open(root, "../../addons")
	if err != nil {
		t.Fatal(err)
	}
	s.TZ = "Europe/Warsaw"
	cache := s.cacheDir("supla")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range s.Catalog["supla"].Fetch.Files {
		data, err := os.ReadFile(filepath.Join("testdata", "supla", f))
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(cache, f), data, 0o644)
	}
	os.WriteFile(filepath.Join(cache, sourceFile), []byte(s.Catalog["supla"].Fetch.URL), 0o644)
	return s
}

func readEnv(t *testing.T, s *Stack, name string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(s.Dir(), name))
	if err != nil {
		t.Fatal(err)
	}
	kvs, err := parseEnv(data)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]string{}
	for _, kv := range kvs {
		m[kv.Key] = kv.Value
	}
	return m
}

func TestInstallSupla(t *testing.T) {
	s := newStack(t)
	if err := s.Install("supla", map[string]string{"SUPLA_HOST_ADDRESS": "192.168.1.5"}); err != nil {
		t.Fatal(err)
	}
	env := readEnv(t, s, ".env")
	want := map[string]string{
		"COMPOSE_PROJECT_NAME": "supla",
		"COMPOSE_FILE":         "docker-compose.yml:docker-compose.standalone.yml:docker-compose.supla-suplaos.yml",
		"VOLUME_DATA":          "./var",
		"PORT_HTTP":            "8081",
		"PORT_HTTPS":           "443",
		"SUPLA_HOST_ADDRESS":   "192.168.1.5",
		"DATABASE_IMAGE":       "mariadb:12.2.2",
		"TZ":                   "Europe/Warsaw",
	}
	host, _ := os.Hostname()
	want["SUPLA_MAILER_FROM"] = "supla@" + host + ".local"
	want["SUPLA_ACCOUNTS_REGISTRATION_ENABLED"] = "false"
	for k, v := range want {
		if env[k] != v {
			t.Errorf("%s = %q, want %q", k, env[k], v)
		}
	}
	if len(env["DATABASE_PASSWORD"]) != 32 || len(env["APP_SECRET"]) != 64 {
		t.Errorf("secrets not generated: %q %q", env["DATABASE_PASSWORD"], env["APP_SECRET"])
	}
	for _, f := range []string{".env.default", "docker-compose.yml", "ssl/server", "var"} {
		if _, err := os.Stat(filepath.Join(s.Dir(), f)); err != nil {
			t.Error(err)
		}
	}
	own := readEnv(t, s, "supla.env")
	if own["DATABASE_PASSWORD"] != env["DATABASE_PASSWORD"] || own["TZ"] != "Europe/Warsaw" || own["COMPOSE_FILE"] != "" {
		t.Errorf("supla.env = %v", own)
	}
	st, _ := os.Stat(filepath.Join(s.Dir(), ".env"))
	if st.Mode().Perm() != 0o600 {
		t.Errorf(".env mode %v", st.Mode())
	}

	// Secrets survive reopening.
	s2, err := Open(s.Root, "../../addons")
	if err != nil {
		t.Fatal(err)
	}
	if s2.State.Addons["supla"].Options["APP_SECRET"] != env["APP_SECRET"] {
		t.Error("secret not saved")
	}
}

func TestZigbeeSetup(t *testing.T) {
	s := newStack(t)
	if err := s.Install("zigbee2supla", map[string]string{"Z2S_SUPLA_EMAIL": "a@b.c"}); err == nil {
		t.Fatal("installed without Z2S_SUPLA_SERVER")
	}
	if err := s.Install("zigbee2mqtt", nil); err == nil || !strings.Contains(err.Error(), "requires mosquitto") {
		t.Fatalf("zigbee2mqtt without mosquitto: %v", err)
	}
	must(t, s.Install("supla", nil))
	must(t, s.Install("mosquitto", nil))
	if err := s.Install("zigbee2mqtt", nil); err == nil || !strings.Contains(err.Error(), "ZIGBEE_ADAPTER is required") {
		t.Fatalf("zigbee2mqtt without adapter: %v", err)
	}
	must(t, s.Install("zigbee2mqtt", map[string]string{"ZIGBEE_ADAPTER": "/dev/serial/by-id/usb-x"}))
	must(t, s.Install("zigbee2supla", map[string]string{"Z2S_SUPLA_EMAIL": "a@b.c"}))

	env := readEnv(t, s, ".env")
	if got, want := env["COMPOSE_FILE"], "docker-compose.mosquitto.yml:docker-compose.yml:docker-compose.standalone.yml:docker-compose.supla-suplaos.yml:"+
		"docker-compose.zigbee2mqtt.yml:docker-compose.zigbee2mqtt-usb.yml:"+
		"docker-compose.zigbee2supla.yml:docker-compose.zigbee2supla-supla.yml:docker-compose.zigbee2supla-mosquitto.yml"; got != want {
		t.Errorf("COMPOSE_FILE = %s", got)
	}
	if own := readEnv(t, s, "supla.env"); own["ZIGBEE_ADAPTER"] != "" {
		t.Error("supla.env has variables of other add-ons")
	}
	z := readEnv(t, s, "zigbee2supla.env")
	if z["Z2S_SUPLA_EMAIL"] != "a@b.c" || len(z) != 1 {
		t.Errorf("zigbee2supla.env = %v", z)
	}

	// Network coordinator: the USB file goes away.
	must(t, s.Configure("zigbee2mqtt", map[string]string{
		"ZIGBEE2MQTT_COORDINATOR": "network", "ZIGBEE2MQTT_SERIAL_PORT": "tcp://10.0.0.2:6638"}))
	env = readEnv(t, s, ".env")
	if strings.Contains(env["COMPOSE_FILE"], "-usb") || !strings.Contains(env["COMPOSE_FILE"], "-network") {
		t.Errorf("COMPOSE_FILE = %s", env["COMPOSE_FILE"])
	}
	if env["ZIGBEE2MQTT_SERIAL_ADAPTER"] != "zstack" || env["ZIGBEE_ADAPTER"] != "" {
		t.Errorf("adapter vars: %q %q", env["ZIGBEE2MQTT_SERIAL_ADAPTER"], env["ZIGBEE_ADAPTER"])
	}
	if _, err := os.Stat(filepath.Join(s.Dir(), "docker-compose.zigbee2mqtt-usb.yml")); !os.IsNotExist(err) {
		t.Error("stale compose file left")
	}

	if err := s.Remove("mosquitto"); err == nil {
		t.Error("removed mosquitto needed by zigbee2mqtt")
	}
	must(t, s.Remove("zigbee2mqtt"))
	must(t, s.Remove("mosquitto"))
	z = readEnv(t, s, "zigbee2supla.env")
	if z["Z2S_MQTT_PORT"] != "1883" {
		t.Errorf("MQTT settings missing without mosquitto: %v", z)
	}
	must(t, s.Purge("zigbee2mqtt"))
}

func TestPortConflict(t *testing.T) {
	s := newStack(t)
	must(t, s.Install("supla", nil))
	must(t, s.Install("mosquitto", nil))
	err := s.Install("zigbee2mqtt", map[string]string{"ZIGBEE_ADAPTER": "/dev/ttyUSB0", "ZIGBEE2MQTT_PORT": "8081"})
	if err == nil || !strings.Contains(err.Error(), "port 8081 is already used by supla") {
		t.Fatalf("conflict not found: %v", err)
	}
	if s.IsInstalled("zigbee2mqtt") {
		t.Error("failed install left in state")
	}
	if err := s.Configure("supla", map[string]string{"PORT_HTTP": "80"}); err == nil {
		t.Error("panel port accepted")
	}
	if s.Values(s.Catalog["supla"])["PORT_HTTP"] != "8081" {
		t.Error("failed config changed the state")
	}
}

func TestQuote(t *testing.T) {
	for in, want := range map[string]string{
		"abc":                 "abc",
		"null://null":         "null://null",
		"smtp://u:p@h:25?x=1": "'smtp://u:p@h:25?x=1'",
		"a b":                 "'a b'",
		"$x":                  "'$x'",
	} {
		if got := quote(in); got != want {
			t.Errorf("quote(%q) = %s, want %s", in, got, want)
		}
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestMosquitto(t *testing.T) {
	s := newStack(t)
	must(t, s.Install("mosquitto", nil))
	env := readEnv(t, s, ".env")
	if len(env["MOSQUITTO_PASSWORD"]) != 16 || env["MOSQUITTO_USERNAME"] != "mqtt" || env["MOSQUITTO_LAN"] != "off" {
		t.Errorf("mosquitto env: %v", env)
	}
	if env["COMPOSE_FILE"] != "docker-compose.mosquitto.yml" {
		t.Errorf("COMPOSE_FILE = %s", env["COMPOSE_FILE"])
	}
	crt, err := os.ReadFile(filepath.Join(s.Dir(), "ssl/mosquitto/server.crt"))
	if err != nil || !strings.Contains(string(crt), "BEGIN CERTIFICATE") {
		t.Fatalf("certificate: %v", err)
	}
	if st, _ := os.Stat(filepath.Join(s.Dir(), "ssl/mosquitto/server.key")); st.Mode().Perm() != 0o600 {
		t.Errorf("key mode %v", st.Mode())
	}

	must(t, s.Configure("mosquitto", map[string]string{"MOSQUITTO_LAN": "both"}))
	env = readEnv(t, s, ".env")
	if !strings.Contains(env["COMPOSE_FILE"], "mosquitto-lan") || !strings.Contains(env["COMPOSE_FILE"], "mosquitto-tls") {
		t.Errorf("COMPOSE_FILE = %s", env["COMPOSE_FILE"])
	}
	// The certificate is made once.
	if again, _ := os.ReadFile(filepath.Join(s.Dir(), "ssl/mosquitto/server.crt")); string(again) != string(crt) {
		t.Error("certificate regenerated")
	}
	if err := s.Configure("mosquitto", map[string]string{"MOSQUITTO_LAN": "maybe"}); err == nil {
		t.Error("invalid choice accepted")
	}

	// A password missing from an older installation is generated.
	delete(s.State.Addons["mosquitto"].Options, "MOSQUITTO_PASSWORD")
	must(t, s.Apply())
	if len(readEnv(t, s, ".env")["MOSQUITTO_PASSWORD"]) != 16 {
		t.Error("password not filled in")
	}
}

func TestSuplaMQTT(t *testing.T) {
	s := newStack(t)
	must(t, s.Install("supla", nil))
	if v := s.Values(s.Catalog["supla"]); s.Active(s.Catalog["supla"].Option("SUPLA_MQTT_BROKER_ENABLED"), v) {
		t.Error("MQTT option active without mosquitto")
	}
	must(t, s.Install("mosquitto", map[string]string{"MOSQUITTO_LAN": "tls"}))
	files := readEnv(t, s, ".env")["COMPOSE_FILE"]
	if strings.Contains(files, "supla-mqtt.yml") || !strings.Contains(files, "supla-mqtt-tls.yml") {
		t.Errorf("COMPOSE_FILE = %s", files)
	}
	must(t, s.Configure("supla", map[string]string{"SUPLA_MQTT_BROKER_ENABLED": "true"}))
	files = readEnv(t, s, ".env")["COMPOSE_FILE"]
	if !strings.Contains(files, "supla-mqtt.yml:docker-compose.supla-mqtt-tls.yml") {
		t.Errorf("COMPOSE_FILE = %s", files)
	}
	if readEnv(t, s, "supla.env")["SUPLA_MQTT_BROKER_ENABLED"] != "true" {
		t.Error("supla.env without SUPLA_MQTT_BROKER_ENABLED")
	}
	must(t, s.Configure("mosquitto", map[string]string{"MOSQUITTO_LAN": "off"}))
	if files = readEnv(t, s, ".env")["COMPOSE_FILE"]; strings.Contains(files, "mqtt-tls") {
		t.Errorf("COMPOSE_FILE = %s", files)
	}
}

func TestAfterQueue(t *testing.T) {
	s := newStack(t)
	must(t, s.Install("supla", nil))
	must(t, s.Install("mosquitto", nil))
	if len(s.after) != 0 {
		t.Fatalf("after on install with MQTT off: %v", s.after)
	}
	must(t, s.Configure("supla", map[string]string{"SUPLA_MQTT_BROKER_ENABLED": "true"}))
	if len(s.after) != 1 || s.after[0].Service != "supla-db" || s.after[0].Restart[0] != "supla-server" {
		t.Fatalf("after = %v", s.after)
	}
	s.after = nil
	must(t, s.Configure("supla", map[string]string{"PORT_HTTP": "8082"}))
	if len(s.after) != 0 {
		t.Errorf("after without a change of the option: %v", s.after)
	}
	s.QueueAfter("supla", "SUPLA_MQTT_BROKER_ENABLED")
	if len(s.after) != 1 {
		t.Errorf("QueueAfter: %v", s.after)
	}
}
