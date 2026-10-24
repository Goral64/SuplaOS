package stack

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	names := deviceNames
	deviceNames = func() ([]string, []net.IP) {
		return []string{"suplaos.local", "localhost"}, []net.IP{net.ParseIP("192.168.1.20"), net.ParseIP("127.0.0.1")}
	}
	t.Cleanup(func() { deviceNames = names })
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
	err := s.Install("zigbee2mqtt", map[string]string{"ZIGBEE_ADAPTER": "/dev/ttyUSB0", "ZIGBEE2MQTT_PORT": "443"})
	if err == nil || !strings.Contains(err.Error(), "port 443 is already used by supla") {
		t.Fatalf("conflict not found: %v", err)
	}
	if s.IsInstalled("zigbee2mqtt") {
		t.Error("failed install left in state")
	}
	if err := s.Configure("supla", map[string]string{"PORT_HTTPS": "8443"}); err == nil {
		t.Error("panel port accepted")
	}
	if s.Values(s.Catalog["supla"])["PORT_HTTPS"] != "443" {
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

// verify checks server.crt in dir against the CA of the device.
func verify(t *testing.T, s *Stack, dir, name string) error {
	t.Helper()
	roots := x509.NewCertPool()
	ca, err := os.ReadFile(s.CACertFile())
	if err != nil || !roots.AppendCertsFromPEM(ca) {
		t.Fatalf("CA: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "server.crt"))
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(data)
	crt, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	_, err = crt.Verify(x509.VerifyOptions{Roots: roots, DNSName: name})
	return err
}

func TestCertificate(t *testing.T) {
	s := newStack(t)
	must(t, s.Install("mosquitto", nil))
	dir := filepath.Join(s.Dir(), "ssl/mosquitto")
	for _, name := range []string{"suplaos.local", "192.168.1.20", "localhost"} {
		if err := verify(t, s, dir, name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if verify(t, s, dir, "suplaos") == nil {
		t.Error("certificate valid for a name it was not made for")
	}
	if st, _ := os.Stat(filepath.Join(s.CADir(), "ca.key")); st.Mode().Perm() != 0o600 {
		t.Errorf("CA key mode %v", st.Mode())
	}
	if len(s.after) != 0 {
		t.Errorf("restart queued for a new certificate: %v", s.after)
	}

	// A new address renews the certificate and restarts the add-on.
	deviceNames = func() ([]string, []net.IP) {
		return []string{"suplaos.local", "localhost"}, []net.IP{net.ParseIP("192.168.1.30"), net.ParseIP("127.0.0.1")}
	}
	must(t, s.Apply())
	if err := verify(t, s, dir, "192.168.1.30"); err != nil {
		t.Error(err)
	}
	if len(s.after) != 1 || s.after[0].Restart[0] != "mosquitto" || len(s.after[0].Command) != 0 {
		t.Errorf("after = %v", s.after)
	}
	s.after = nil
	crt, _ := os.ReadFile(filepath.Join(dir, "server.crt"))
	must(t, s.Apply())
	if again, _ := os.ReadFile(filepath.Join(dir, "server.crt")); string(again) != string(crt) || len(s.after) != 0 {
		t.Error("valid certificate replaced")
	}

	// A public address is left out, the CA cannot sign for it.
	deviceNames = func() ([]string, []net.IP) {
		return []string{"suplaos.local"}, []net.IP{net.ParseIP("203.0.113.5")}
	}
	must(t, s.Apply())
	if verify(t, s, dir, "suplaos.local") != nil {
		t.Error("certificate with a public address")
	}
}

func TestCAConstraints(t *testing.T) {
	s := newStack(t)
	ca, caKey, err := s.loadCA()
	if err != nil {
		t.Fatal(err)
	}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: serialNumber(),
		Subject:      pkix.Name{CommonName: "example.com"},
		NotBefore:    ca.NotBefore,
		NotAfter:     ca.NotBefore.Add(leafValidity),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"example.com"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	crt, _ := x509.ParseCertificate(der)
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	if _, err := crt.Verify(x509.VerifyOptions{Roots: roots, DNSName: "example.com"}); err == nil {
		t.Error("CA accepted for example.com")
	}
}

// Every CA has its own name, also on devices with the same name, and a CA
// of an older SuplaOS (without the ID) is made anew, with its certificates.
func TestCAName(t *testing.T) {
	a, b := newStack(t), newStack(t)
	caA, _, err := a.loadCA()
	must(t, err)
	caB, _, err := b.loadCA()
	must(t, err)
	if caA.Subject.String() == caB.Subject.String() || caA.Subject.SerialNumber == "" {
		t.Errorf("CA names %q and %q", caA.Subject, caB.Subject)
	}

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: serialNumber(), Subject: pkix.Name{CommonName: "SuplaOS suplaos", Organization: []string{"SuplaOS"}},
		NotBefore: time.Now(), NotAfter: time.Now().Add(caValidity), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	keyDER, _ := x509.MarshalECPrivateKey(key)
	s := newStack(t)
	os.MkdirAll(s.CADir(), 0o755)
	os.WriteFile(s.CACertFile(), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
	os.WriteFile(filepath.Join(s.CADir(), "ca.key"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600)
	dir := filepath.Join(s.Dir(), "ssl/test")
	if _, err := s.Certificate(dir); err != nil {
		t.Fatal(err)
	}
	ca, _, err := s.loadCA()
	must(t, err)
	if ca.Subject.SerialNumber == "" {
		t.Errorf("old CA kept: %q", ca.Subject)
	}
	if replaced, err := s.Certificate(dir); err != nil || replaced {
		t.Errorf("certificate of the new CA replaced: %v %v", replaced, err)
	}
	if err := verify(t, s, dir, "suplaos.local"); err != nil {
		t.Error(err)
	}
}

// An existing self-signed certificate (older SuplaOS, the Cloud image) is
// replaced by one from the CA.
func TestCertificateReplaced(t *testing.T) {
	s := newStack(t)
	dir := filepath.Join(s.Dir(), "ssl/test")
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: serialNumber(), Subject: pkix.Name{CommonName: "SUPLA"},
		NotBefore: time.Now(), NotAfter: time.Now().Add(leafValidity), DNSNames: []string{"suplaos.local"}}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "server.crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
	replaced, err := s.Certificate(dir)
	if err != nil || !replaced {
		t.Fatalf("replaced %v, %v", replaced, err)
	}
	if err := verify(t, s, dir, "suplaos.local"); err != nil {
		t.Error(err)
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
	must(t, s.Configure("supla", map[string]string{"MAILER_DSN": "smtp://localhost:25"}))
	if len(s.after) != 0 {
		t.Errorf("after without a change of the option: %v", s.after)
	}
	s.QueueAfter("supla", "SUPLA_MQTT_BROKER_ENABLED")
	if len(s.after) != 1 {
		t.Errorf("QueueAfter: %v", s.after)
	}
}

func TestPending(t *testing.T) {
	s := newStack(t)
	must(t, s.Install("supla", nil))
	must(t, s.Install("mosquitto", nil))
	if p, err := s.Pending(); err != nil || len(p) != 0 {
		t.Fatalf("pending after install: %v %v", p, err)
	}
	// A newer catalog changes a compose file of mosquitto.
	a := s.Catalog["mosquitto"]
	dir := t.TempDir()
	for _, f := range []string{"docker-compose.mosquitto.yml", "docker-compose.mosquitto-lan.yml", "docker-compose.mosquitto-tls.yml", "mosquitto-start.sh"} {
		data, _ := os.ReadFile(filepath.Join(a.Dir, f))
		if f == "docker-compose.mosquitto.yml" {
			data = append(data, []byte("# new version\n")...)
		}
		os.WriteFile(filepath.Join(dir, f), data, 0o644)
	}
	a.Dir = dir
	// And a new supla-docker commit.
	s.Catalog["supla"].Fetch.URL += "?new"
	p, err := s.Pending()
	if err != nil || strings.Join(p, ",") != "mosquitto,supla" {
		t.Fatalf("pending = %v %v", p, err)
	}
	s.Catalog["supla"].Fetch.URL = strings.TrimSuffix(s.Catalog["supla"].Fetch.URL, "?new")
	must(t, s.Apply())
	if p, _ := s.Pending(); len(p) != 0 {
		t.Errorf("pending after apply: %v", p)
	}
}

func TestSplitRef(t *testing.T) {
	for ref, want := range map[string][3]string{
		"mariadb:12.2.2":                    {"registry-1.docker.io", "library/mariadb", "12.2.2"},
		"supla/supla-cloud":                 {"registry-1.docker.io", "supla/supla-cloud", "latest"},
		"docker.io/supla/supla-server:v1":   {"registry-1.docker.io", "supla/supla-server", "v1"},
		"ghcr.io/koenkk/zigbee2mqtt:latest": {"ghcr.io", "koenkk/zigbee2mqtt", "latest"},
		"localhost:5000/test":               {"localhost:5000", "test", "latest"},
	} {
		host, repo, tag := splitRef(ref)
		if got := [3]string{host, repo, tag}; got != want {
			t.Errorf("%s: %v, want %v", ref, got, want)
		}
	}
}

func TestParseChallenge(t *testing.T) {
	p := parseChallenge(`realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/mariadb:pull"`)
	if p["realm"] != "https://auth.docker.io/token" || p["service"] != "registry.docker.io" || p["scope"] != "repository:library/mariadb:pull" {
		t.Errorf("%v", p)
	}
}

func TestImageVersion(t *testing.T) {
	c := &imageConfig{
		Labels: map[string]string{"org.opencontainers.image.version": "2.1.2"},
		Env:    []string{"PATH=/bin", "CLOUD_VERSION=26.05.01"},
	}
	if v := c.version(""); v != "2.1.2" {
		t.Errorf("label: %q", v)
	}
	if v := c.version("CLOUD_VERSION"); v != "2.1.2" {
		t.Errorf("label first: %q", v)
	}
	c.Labels = nil
	if v := c.version("CLOUD_VERSION"); v != "26.05.01" {
		t.Errorf("env: %q", v)
	}
}

// TestSuplaDockerFiles checks the supla add-on with real supla-docker files
// from the directory in SUPLAOS_SUPLA_DOCKER (the supla-docker workflow sets
// it): docker compose has to accept the project without unset variables.
func TestSuplaDockerFiles(t *testing.T) {
	dir := os.Getenv("SUPLAOS_SUPLA_DOCKER")
	if dir == "" {
		t.Skip("SUPLAOS_SUPLA_DOCKER not set")
	}
	s := newStack(t)
	for _, f := range s.Catalog["supla"].Fetch.Files {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(s.cacheDir("supla"), f), data, 0o644)
	}
	for _, step := range []struct {
		id  string
		set map[string]string
	}{
		{"mosquitto", nil},
		{"supla", map[string]string{"SUPLA_MQTT_BROKER_ENABLED": "true"}},
		{"zigbee2supla", map[string]string{"Z2S_SUPLA_EMAIL": "test@example.com"}},
	} {
		if err := s.Install(step.id, step.set); err != nil {
			t.Fatalf("%s: %v", step.id, err)
		}
	}
	out, err := s.Command(context.Background(), "config", "--quiet").CombinedOutput()
	if err != nil || strings.Contains(string(out), "not set") {
		t.Fatalf("docker compose config: %v\n%s", err, out)
	}
}
