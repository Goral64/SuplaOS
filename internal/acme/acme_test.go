package acme

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/letsencrypt/challtestsrv"
	"github.com/letsencrypt/pebble/v2/ca"
	"github.com/letsencrypt/pebble/v2/db"
	"github.com/letsencrypt/pebble/v2/va"
	"github.com/letsencrypt/pebble/v2/wfe"
)

// dnsProvider sets the TXT records in the test DNS server.
type dnsProvider struct {
	srv   *challtestsrv.ChallSrv
	calls atomic.Int32
	fail  bool
}

func (p *dnsProvider) Present(domain, token, keyAuth string) error {
	p.calls.Add(1)
	if p.fail {
		return errors.New("DNS API refused")
	}
	info := dns01.GetChallengeInfo(domain, keyAuth)
	p.srv.AddDNSTXTRecord(info.EffectiveFQDN, info.Value)
	return nil
}

// Timeout makes lego check the record at once, not after 2 s.
func (p *dnsProvider) Timeout() (timeout, interval time.Duration) {
	return 10 * time.Second, 10 * time.Millisecond
}

func (p *dnsProvider) CleanUp(domain, token, keyAuth string) error {
	p.srv.DeleteDNSTXTRecord(dns01.GetChallengeInfo(domain, keyAuth).EffectiveFQDN)
	return nil
}

func freeUDP(t *testing.T) string {
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return c.LocalAddr().String()
}

// testManager runs Pebble (the test ACME server of Let's Encrypt) and its
// DNS server in this process and returns a Manager using them.
func testManager(t *testing.T) (*Manager, *dnsProvider) {
	t.Setenv("PEBBLE_VA_NOSLEEP", "1")
	t.Setenv("PEBBLE_WFE_NONCEREJECT", "0")
	t.Setenv("PEBBLE_AUTHZREUSE", "0")
	quiet := log.New(io.Discard, "", 0)

	dnsAddr := freeUDP(t)
	dns, err := challtestsrv.New(challtestsrv.Config{DNSAddrs: []string{dnsAddr}, Log: quiet})
	if err != nil {
		t.Fatal(err)
	}
	go dns.Run()
	t.Cleanup(dns.Shutdown)

	store := db.NewMemoryStore()
	authority := ca.New(quiet, store, "", "ecdsa", 0, 1, map[string]ca.Profile{"default": {}})
	validator := va.New(quiet, 0, 0, false, dnsAddr, store)
	front := wfe.New(quiet, store, validator, authority, []string{"pebble.letsencrypt.org"}, false, false, 1, 1)
	srv := httptest.NewTLSServer(front.Handler())
	t.Cleanup(srv.Close)

	p := &dnsProvider{srv: dns}
	m := New(t.TempDir())
	m.lookupCNAME = func(name string) (string, error) { return name, nil }
	m.directory = srv.URL + "/dir"
	m.client = srv.Client()
	m.provider = p
	m.dnsOptions = []dns01.ChallengeOption{
		dns01.AddRecursiveNameservers([]string{dnsAddr}),
		dns01.WrapPreCheck(func(domain, fqdn, value string, check dns01.PreCheckFunc) (bool, error) { return true, nil }),
	}
	return m, p
}

func save(t *testing.T, m *Manager, domain string) {
	t.Helper()
	if err := m.SaveSettings(Settings{Domain: domain, Provider: "duckdns", Fields: map[string]string{"DUCKDNS_TOKEN": "x"}}); err != nil {
		t.Fatal(err)
	}
}

func TestObtainAndRenew(t *testing.T) {
	m, p := testManager(t)
	save(t, m, "supla.example.com")

	var out bytes.Buffer
	renewed, err := m.Renew(&out)
	if err != nil || !renewed {
		t.Fatalf("first Renew: %v %v\n%s", renewed, err, out.String())
	}
	pair, err := m.Current()
	if err != nil || pair == nil {
		t.Fatalf("Current: %v %v", pair, err)
	}
	if pair.Leaf.DNSNames[0] != "supla.example.com" {
		t.Errorf("names %v", pair.Leaf.DNSNames)
	}
	if fi, _ := os.Stat(m.KeyFile()); fi.Mode().Perm() != 0o600 {
		t.Errorf("cert.key mode %v", fi.Mode())
	}

	// A fresh certificate is kept; ARI is asked and says it is not time.
	if renewed, err := m.Renew(&out); err != nil || renewed {
		t.Fatalf("second Renew: %v %v\n%s", renewed, err, out.String())
	}
	if st := m.loadState(); st.ARIChecked.IsZero() || st.RenewAt.IsZero() {
		t.Errorf("ARI not checked: %+v", st)
	}

	// 2/3 of the validity passed.
	leaf := pair.Leaf
	m.now = func() time.Time { return leaf.NotBefore.Add(leaf.NotAfter.Sub(leaf.NotBefore) * 3 / 4) }
	if renewed, err := m.Renew(&out); err != nil || !renewed {
		t.Fatalf("Renew when old: %v %v\n%s", renewed, err, out.String())
	}
	m.now = time.Now
	if again, _ := m.Current(); again == nil || again.Leaf.SerialNumber.Cmp(leaf.SerialNumber) == 0 {
		t.Error("certificate not replaced")
	}

	// Another domain needs another certificate.
	save(t, m, "home.example.org")
	if c, _ := m.Current(); c != nil {
		t.Error("certificate of the old domain is current")
	}
	if renewed, err := m.Renew(&out); err != nil || !renewed {
		t.Fatalf("Renew for new domain: %v %v\n%s", renewed, err, out.String())
	}
	if c, _ := m.Current(); c == nil || c.Leaf.DNSNames[0] != "home.example.org" {
		t.Error("no certificate for the new domain")
	}
	if p.calls.Load() != 3 {
		t.Errorf("%d challenges, want 3", p.calls.Load())
	}
	if st, _ := m.Status(); st.Domain != "home.example.org" || st.NotAfter == nil || st.LastError != "" {
		t.Errorf("status %+v", st)
	}
}

func TestRetryAfterFailure(t *testing.T) {
	m, p := testManager(t)
	p.fail = true
	save(t, m, "supla.example.com")

	if _, err := m.Renew(io.Discard); err == nil || !strings.Contains(err.Error(), "DNS API refused") {
		t.Fatalf("Renew: %v", err)
	}
	st, _ := m.Status()
	if st.LastError == "" || st.NextAttempt == nil {
		t.Fatalf("status %+v", st)
	}
	calls := p.calls.Load()
	if renewed, _ := m.Renew(io.Discard); renewed || p.calls.Load() != calls {
		t.Error("tried again right after a failure")
	}

	// Twice as long after the second failure.
	start := time.Now()
	m.now = func() time.Time { return start.Add(16 * time.Minute) }
	if renewed, _ := m.Renew(io.Discard); !renewed {
		t.Fatal("no try after 15 minutes")
	}
	m.now = func() time.Time { return start.Add(40 * time.Minute) }
	if renewed, _ := m.Renew(io.Discard); renewed {
		t.Error("tried again 24 minutes after the second failure")
	}

	// Obtain (the button in the panel) does not wait.
	p.fail = false
	m.now = time.Now
	if err := m.Obtain(io.Discard); err != nil {
		t.Fatal(err)
	}
	if st, _ := m.Status(); st.LastError != "" || st.NextAttempt != nil {
		t.Errorf("status after success %+v", st)
	}
}

func TestFollowCNAME(t *testing.T) {
	m := New(t.TempDir())
	for target, want := range map[string]bool{
		"_acme-challenge.supla.example.pl.": true,  // no CNAME
		"supla-example.duckdns.org.":        true,  // set aside for the challenge
		"example.pl.":                       false, // *.example.pl CNAME example.pl
		"www.example.pl.":                   false,
	} {
		m.lookupCNAME = func(string) (string, error) { return target, nil }
		if got := m.followCNAME("supla.example.pl", io.Discard); got != want {
			t.Errorf("CNAME to %s: follow %v, want %v", target, got, want)
		}
	}
	m.lookupCNAME = func(string) (string, error) { return "", errors.New("no such host") }
	if !m.followCNAME("supla.example.pl", io.Discard) {
		t.Error("not followed after a failed lookup")
	}
}

func TestExactCheckIntercepted(t *testing.T) {
	// Something on the way answers the queries: wait in one go, longer
	// than the time limit of lego, and do not ask again.
	var slept time.Duration
	askAuthoritative = func(string, string, string) (bool, error) { return false, errNotAuthoritative }
	sleep = func(d time.Duration) { slept += d }
	defer func() { askAuthoritative, sleep = authoritative, time.Sleep }()
	check := exactCheck("supla.example.pl", 2*time.Minute, io.Discard)
	done, err := check("", "_acme-challenge.supla.example.pl.", "v", nil)
	if !done || err != nil {
		t.Errorf("check: %v, %v", done, err)
	}
	if slept < time.Minute+50*time.Second || slept > 2*time.Minute {
		t.Errorf("waited %s", slept)
	}
}

// slowProvider gives lego a time limit shorter than the wait.
type slowProvider struct{ *dnsProvider }

func (p slowProvider) Timeout() (timeout, interval time.Duration) {
	return 200 * time.Millisecond, 10 * time.Millisecond
}

func TestObtainIntercepted(t *testing.T) {
	m, p := testManager(t)
	m.provider = slowProvider{p}
	m.dnsOptions = m.dnsOptions[:1]
	askAuthoritative = func(string, string, string) (bool, error) { return false, errNotAuthoritative }
	sleep = func(time.Duration) { time.Sleep(400 * time.Millisecond) }
	defer func() { askAuthoritative, sleep = authoritative, time.Sleep }()
	if err := m.SaveSettings(Settings{Domain: "supla.example.com", Provider: "cloudflare", Fields: map[string]string{"CF_DNS_API_TOKEN": "x"}}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := m.Obtain(&out); err != nil {
		t.Fatalf("Obtain: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "waiting 2m0s for the record instead") {
		t.Errorf("output:\n%s", out.String())
	}
}

func TestSaveSettings(t *testing.T) {
	m := New(t.TempDir())
	if s, err := m.Settings(); s != nil || err != nil {
		t.Fatalf("Settings before saving: %v %v", s, err)
	}
	bad := []Settings{
		{Domain: "localhost", Provider: "duckdns", Fields: map[string]string{"DUCKDNS_TOKEN": "x"}},
		{Domain: "suplaos.local", Provider: "duckdns", Fields: map[string]string{"DUCKDNS_TOKEN": "x"}},
		{Domain: "*.example.com", Provider: "duckdns", Fields: map[string]string{"DUCKDNS_TOKEN": "x"}},
		{Domain: "-a.example.com", Provider: "duckdns", Fields: map[string]string{"DUCKDNS_TOKEN": "x"}},
		{Domain: "supla.example.com", Provider: "nope"},
		{Domain: "supla.example.com", Provider: "duckdns"},
		{Domain: "supla.example.com", Provider: "duckdns", Email: "x", Fields: map[string]string{"DUCKDNS_TOKEN": "x"}},
	}
	for _, s := range bad {
		if err := m.SaveSettings(s); err == nil {
			t.Errorf("%+v accepted", s)
		}
	}

	err := m.SaveSettings(Settings{Domain: " Supla.Example.COM. ", Provider: "ovh", Fields: map[string]string{
		"OVH_APPLICATION_KEY": "ak", "OVH_APPLICATION_SECRET": "as", "OVH_CONSUMER_KEY": "ck", "OTHER": "x",
	}})
	if err != nil {
		t.Fatal(err)
	}
	// Secrets left empty are kept, other fields are not.
	if err := m.SaveSettings(Settings{Domain: "supla.example.com", Provider: "ovh", Fields: map[string]string{"OVH_APPLICATION_KEY": "ak2"}}); err != nil {
		t.Fatal(err)
	}
	s, _ := m.Settings()
	want := map[string]string{"OVH_APPLICATION_KEY": "ak2", "OVH_APPLICATION_SECRET": "as", "OVH_CONSUMER_KEY": "ck"}
	if s.Domain != "supla.example.com" || len(s.Fields) != len(want) {
		t.Fatalf("settings %+v", s)
	}
	for k, v := range want {
		if s.Fields[k] != v {
			t.Errorf("%s = %q, want %q", k, s.Fields[k], v)
		}
	}
	if fi, _ := os.Stat(m.path("settings.yaml")); fi.Mode().Perm() != 0o600 {
		t.Errorf("settings.yaml mode %v", fi.Mode())
	}
	// Secrets of another provider are not taken over.
	if err := m.SaveSettings(Settings{Domain: "supla.example.com", Provider: "duckdns"}); err == nil {
		t.Error("duckdns without a token accepted")
	}
}

// Every provider is made from its fields, so the names of the variables
// are the ones lego reads.
func TestProviders(t *testing.T) {
	m := New(t.TempDir())
	names := map[string]bool{}
	for _, p := range Providers {
		if names[p.Name] {
			t.Errorf("%s twice", p.Name)
		}
		names[p.Name] = true
		fields := map[string]string{}
		for _, f := range p.Fields {
			if f.Label["en"] == "" || f.Label["pl"] == "" {
				t.Errorf("%s: %s without a label", p.Name, f.Env)
			}
			fields[f.Env] = "value"
			if f.Default != "" {
				fields[f.Env] = f.Default
			}
		}
		switch p.Name {
		case "acmedns":
			fields["ACME_DNS_API_BASE"] = "https://auth.example.com"
		case "rfc2136":
			fields["DNSUPDATE_NAMESERVER"] = "127.0.0.1:53"
			fields["DNSUPDATE_TSIG_SECRET"] = "c2VjcmV0"
		case "namecheap":
			// Asks a web service for the public address when made.
			continue
		}
		if _, err := m.newProvider(&Settings{Provider: p.Name, Fields: fields}); err != nil {
			t.Errorf("%s: %v", p.Name, err)
		}
		for env := range fields {
			if _, set := os.LookupEnv(env); set {
				t.Errorf("%s left in the environment", env)
			}
		}
	}
	if len(Providers) != 16 {
		t.Errorf("%d providers", len(Providers))
	}
}
