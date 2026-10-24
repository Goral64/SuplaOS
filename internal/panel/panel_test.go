package panel

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Goral64/SuplaOS/internal/acme"
)

func newServer(t *testing.T) *Server {
	t.Helper()
	return New(Config{
		Root:     t.TempDir(),
		Addons:   "../../addons",
		Settings: t.TempDir() + "/panel.yaml",
		Web: fstest.MapFS{
			"index.html":    {Data: []byte("<html>panel</html>")},
			"assets/app.js": {Data: []byte("app")},
		},
	})
}

func do(t *testing.T, h http.Handler, method, path, token string, header bool) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader("{}"))
	if token != "" {
		r.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	}
	if header {
		r.Header.Set("X-SuplaOS", "1")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAuth(t *testing.T) {
	s := newServer(t)
	h := s.Handler()
	if w := do(t, h, "GET", "/api/addons", "", false); w.Code != http.StatusUnauthorized {
		t.Errorf("no session: %d", w.Code)
	}
	token := s.sessions.create(false)
	if w := do(t, h, "GET", "/api/addons", token, false); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"id":"supla"`) {
		t.Errorf("addons: %d %s", w.Code, w.Body)
	}
	if w := do(t, h, "PUT", "/api/settings", token, false); w.Code != http.StatusForbidden {
		t.Errorf("change without X-SuplaOS: %d", w.Code)
	}
	must := s.sessions.create(true)
	if w := do(t, h, "GET", "/api/addons", must, false); w.Code != http.StatusForbidden {
		t.Errorf("default password session: %d", w.Code)
	}
	if w := do(t, h, "GET", "/api/session", must, false); !strings.Contains(w.Body.String(), `"mustChange":true`) {
		t.Errorf("session: %s", w.Body)
	}
}

func TestSessionsSurviveRestart(t *testing.T) {
	file := t.TempDir() + "/sessions.json"
	var a sessions
	a.load(file)
	token := a.create(false)
	var b sessions
	b.load(file)
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	if b.get(r) == nil {
		t.Fatal("session lost after restart")
	}
	if data, _ := os.ReadFile(file); strings.Contains(string(data), token) {
		t.Error("token stored in plain text")
	}
	b.delete(r)
	var c sessions
	c.load(file)
	if c.get(r) != nil {
		t.Error("deleted session came back")
	}
}

func TestStatic(t *testing.T) {
	h := newServer(t).Handler()
	for _, p := range []string{"/", "/addons/supla", "/system"} {
		if w := do(t, h, "GET", p, "", false); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "panel") {
			t.Errorf("%s: %d %q", p, w.Code, w.Body)
		}
	}
	w := do(t, h, "GET", "/assets/app.js", "", false)
	if w.Body.String() != "app" || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("asset: %q %v", w.Body, w.Header())
	}
}

func TestJobs(t *testing.T) {
	var js jobs
	release := make(chan struct{})
	j, err := js.start("first", func(j *Job) error {
		j.Write([]byte("one\rtwo\nthree"))
		<-release
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := js.start("second", func(*Job) error { return nil }); err == nil {
		t.Fatal("second job started while the first runs")
	}
	var got []string
	done := make(chan bool)
	go func() {
		done <- j.follow(context.Background(), func(l []string) { got = append(got, l...) })
	}()
	close(release)
	select {
	case ok := <-done:
		if !ok || strings.Join(got, "|") != "one|two|three" || j.State != "done" {
			t.Errorf("follow = %v %q %s", ok, got, j.State)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("follow did not end")
	}
}

func TestBackupUpload(t *testing.T) {
	s := newServer(t)
	h := s.Handler()
	token := s.sessions.create(false)
	var dump bytes.Buffer
	gz := gzip.NewWriter(&dump)
	gz.Write([]byte("-- MariaDB dump\n"))
	gz.Close()
	upload := func(addon, name string, body []byte) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/addons/"+addon+"/backups?name="+url.QueryEscape(name), bytes.NewReader(body))
		r.AddCookie(&http.Cookie{Name: cookieName, Value: token})
		r.Header.Set("X-SuplaOS", "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := upload("supla", "old.sql.gz", dump.Bytes()); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"supla-old.sql.gz"`) {
		t.Errorf("upload: %d %s", w.Code, w.Body)
	}
	if w := upload("supla", "supla-old.sql.gz", dump.Bytes()); w.Code != http.StatusConflict {
		t.Errorf("same name: %d", w.Code)
	}
	if w := upload("supla", "cut.sql.gz", dump.Bytes()[:dump.Len()-4]); w.Code != http.StatusBadRequest {
		t.Errorf("truncated: %d", w.Code)
	}
	if w := upload("supla", "plain.sql", []byte("select 1;")); w.Code != http.StatusBadRequest {
		t.Errorf("not gzip: %d", w.Code)
	}
	if w := upload("mosquitto", "x.gz", dump.Bytes()); w.Code != http.StatusNotFound {
		t.Errorf("add-on without backups: %d", w.Code)
	}
	w := do(t, h, "GET", "/api/backups", token, false)
	if got := w.Body.String(); !strings.Contains(got, "supla-old.sql.gz") || strings.Contains(got, "upload") || strings.Contains(got, "cut") {
		t.Errorf("backups: %s", got)
	}
	r := httptest.NewRequest("POST", "/api/addons/supla/restore", strings.NewReader(`{"name":"zigbee2mqtt-x.gz"}`))
	r.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	r.Header.Set("X-SuplaOS", "1")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("restore of another add-on's backup: %d", w.Code)
	}
}

func TestInstallAccount(t *testing.T) {
	s := newServer(t)
	h := s.Handler()
	token := s.sessions.create(false)
	install := func(addon, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/addons/"+addon+"/install", strings.NewReader(body))
		r.AddCookie(&http.Cookie{Name: cookieName, Value: token})
		r.Header.Set("X-SuplaOS", "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := install("supla", `{"account":{"email":"me@example.com","password":"short"}}`); w.Code != http.StatusBadRequest {
		t.Errorf("short password: %d", w.Code)
	}
	if w := install("mosquitto", `{"account":{"email":"me@example.com","password":"long enough"}}`); w.Code != http.StatusBadRequest {
		t.Errorf("account for another add-on: %d", w.Code)
	}
}

func TestTLS(t *testing.T) {
	s := newServer(t)
	cfg, err := s.TLSConfig()
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(s.Handler())
	srv.TLS = cfg
	srv.StartTLS()
	defer srv.Close()

	ca, err := os.ReadFile(s.caFile())
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(ca)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "localhost"}}}
	resp, err := client.Get(srv.URL + "/api/ca.crt")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != string(ca) {
		t.Errorf("ca.crt: %d", resp.StatusCode)
	}
	token := s.sessions.create(false)
	if w := do(t, s.Handler(), "GET", "/api/ca", token, false); !strings.Contains(w.Body.String(), `"sha256"`) {
		t.Errorf("ca info: %s", w.Body)
	}

	// The certificate of a domain set in the panel is served for that
	// name only.
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour),
		DNSNames: []string{"supla.example.com"}}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	keyDER, _ := x509.MarshalECPrivateKey(key)
	dir := filepath.Join(s.cfg.Root, "ca", "acme")
	os.MkdirAll(dir, 0o700)
	os.WriteFile(filepath.Join(dir, "settings.yaml"), []byte("domain: supla.example.com\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "cert.crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
	os.WriteFile(filepath.Join(dir, "cert.key"), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600)
	if err := s.renewCertificates(false); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string][]byte{"supla.example.com": der, "localhost": nil} {
		conn, err := tls.Dial("tcp", srv.Listener.Addr().String(), &tls.Config{ServerName: name, InsecureSkipVerify: true})
		if err != nil {
			t.Fatal(err)
		}
		got := conn.ConnectionState().PeerCertificates[0].Raw
		conn.Close()
		if (want != nil) != bytes.Equal(got, der) {
			t.Errorf("%s: certificate of the domain served %v", name, bytes.Equal(got, der))
		}
	}
}

func TestRedirect(t *testing.T) {
	for host, want := range map[string]string{
		"suplaos.local":   "https://suplaos.local:8443/setup?a=1",
		"192.168.1.20:80": "https://192.168.1.20:8443/setup?a=1",
		"[fe80::1]":       "https://[fe80::1]:8443/setup?a=1",
	} {
		r := httptest.NewRequest("GET", "/setup?a=1", nil)
		r.Host = host
		w := httptest.NewRecorder()
		Redirect().ServeHTTP(w, r)
		if w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != want || w.Body.Len() != 0 {
			t.Errorf("%s: %d %s %q", host, w.Code, w.Header().Get("Location"), w.Body)
		}
	}
}

func TestProxy(t *testing.T) {
	s := newServer(t)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("z2m " + r.URL.Path + " cookie=" + r.Header.Get("Cookie")))
	}))
	defer backend.Close()
	ps := &proxyServer{addon: "zigbee2mqtt", service: "zigbee2mqtt", port: "8080", addr: backend.Listener.Addr().String()}
	h := s.proxyHandler(ps, "8080")

	r := httptest.NewRequest("GET", "https://suplaos.local:8080/devices?x=1", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	want := "https://suplaos.local:8443/login?next=" + url.QueryEscape("https://suplaos.local:8080/devices?x=1")
	if w.Code != http.StatusFound || w.Header().Get("Location") != want {
		t.Errorf("no session: %d %s", w.Code, w.Header().Get("Location"))
	}
	r = httptest.NewRequest("GET", "https://suplaos.local:8080/api", nil)
	r.Header.Set("Upgrade", "websocket")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("websocket without session: %d", w.Code)
	}
	if w := do(t, h, "GET", "/", s.sessions.create(true), false); w.Code != http.StatusFound {
		t.Errorf("default password session: %d", w.Code)
	}

	r = httptest.NewRequest("GET", "https://suplaos.local:8080/devices", nil)
	r.AddCookie(&http.Cookie{Name: "other", Value: "1"})
	r.AddCookie(&http.Cookie{Name: cookieName, Value: s.sessions.create(false)})
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK || w.Body.String() != "z2m /devices cookie=other=1" {
		t.Errorf("proxied: %d %q", w.Code, w.Body)
	}

	// A gone container is looked up again.
	backend.Close()
	r = httptest.NewRequest("GET", "https://suplaos.local:8080/", nil)
	r.AddCookie(&http.Cookie{Name: cookieName, Value: s.sessions.create(false)})
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadGateway || ps.addr != "" {
		t.Errorf("gone: %d %q", w.Code, ps.addr)
	}
}

func TestDomainAPI(t *testing.T) {
	s := newServer(t)
	h := s.Handler()
	token := s.sessions.create(false)
	send := func(method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.AddCookie(&http.Cookie{Name: cookieName, Value: token})
		r.Header.Set("X-SuplaOS", "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := send("GET", "/api/domain", ""); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"status":null`) ||
		!strings.Contains(w.Body.String(), `"name":"ovh"`) {
		t.Fatalf("no domain: %d %s", w.Code, w.Body)
	}
	if w := send("PUT", "/api/domain", `{"domain":"suplaos.local","provider":"duckdns","fields":{"DUCKDNS_TOKEN":"x"}}`); w.Code != http.StatusBadRequest {
		t.Errorf(".local accepted: %d", w.Code)
	}
	if w := send("POST", "/api/domain/renew", ""); w.Code != http.StatusBadRequest {
		t.Errorf("renew without a domain: %d", w.Code)
	}

	// Saved settings come back without the secrets.
	err := s.acme.SaveSettings(acme.Settings{Domain: "supla.example.com", Provider: "ovh", Fields: map[string]string{
		"OVH_APPLICATION_KEY": "ak", "OVH_APPLICATION_SECRET": "very secret", "OVH_CONSUMER_KEY": "ck secret",
	}})
	if err != nil {
		t.Fatal(err)
	}
	w := send("GET", "/api/domain", "")
	body := w.Body.String()
	if strings.Contains(body, "very secret") || strings.Contains(body, "ck secret") || !strings.Contains(body, `"OVH_APPLICATION_KEY":"ak"`) ||
		!strings.Contains(body, `"secrets":["OVH_APPLICATION_SECRET","OVH_CONSUMER_KEY"]`) || !strings.Contains(body, `"domain":"supla.example.com"`) {
		t.Errorf("settings: %s", body)
	}

	if w := send("DELETE", "/api/domain", ""); w.Code != http.StatusAccepted {
		t.Fatalf("disable: %d %s", w.Code, w.Body)
	}
	if set, _ := s.acme.Settings(); set != nil {
		t.Errorf("settings kept: %+v", set)
	}
}
