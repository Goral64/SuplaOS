// Package acme gets a Let's Encrypt certificate for a domain of the user
// with the DNS-01 challenge: the API of the DNS provider sets a TXT record,
// so the device does not have to be reachable from the internet.
package acme

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/lego"
	legolog "github.com/go-acme/lego/v4/log"
	"github.com/go-acme/lego/v4/registration"
	"github.com/miekg/dns"
	"golang.org/x/net/publicsuffix"
	"gopkg.in/yaml.v3"

	"github.com/Goral64/SuplaOS/internal/domaincert"
)

const (
	Production = lego.LEDirectoryProduction
	Staging    = lego.LEDirectoryStaging

	// The renewal info (ARI) of the certificate is asked for this often.
	ariInterval = 12 * time.Hour
	// After a failure the next try waits this long, twice as long after
	// each next one, up to maxRetry.
	firstRetry = 15 * time.Minute
	maxRetry   = 24 * time.Hour
)

// Settings are what the user sets in the panel. Fields are the values of
// the fields of the provider, by their Env.
type Settings struct {
	Domain   string            `yaml:"domain" json:"domain"`
	Provider string            `yaml:"provider" json:"provider"`
	Fields   map[string]string `yaml:"fields" json:"fields"`
	Email    string            `yaml:"email,omitempty" json:"email"`
	Staging  bool              `yaml:"staging,omitempty" json:"staging"`
}

// Status is the state of the certificate shown in the panel.
type Status struct {
	Domain      string     `json:"domain"`
	Issuer      string     `json:"issuer,omitempty"`
	NotAfter    *time.Time `json:"notAfter,omitempty"`
	LastAttempt *time.Time `json:"lastAttempt,omitempty"`
	LastError   string     `json:"lastError,omitempty"`
	NextAttempt *time.Time `json:"nextAttempt,omitempty"`
}

// state is kept between the runs in state.json.
type state struct {
	// Directory is the ACME server that issued cert.crt.
	Directory  string    `json:"directory,omitempty"`
	Attempt    time.Time `json:"attempt,omitzero"`
	Error      string    `json:"error,omitempty"`
	Failures   int       `json:"failures,omitempty"`
	ARIChecked time.Time `json:"ariChecked,omitzero"`
	// RenewAt is the time picked in the window suggested by ARI.
	RenewAt time.Time `json:"renewAt,omitzero"`
}

// account is the ACME account, made on first use.
type account struct {
	Directory    string                 `json:"directory"`
	Email        string                 `json:"email,omitempty"`
	Key          string                 `json:"key"`
	Registration *registration.Resource `json:"registration"`
	key          crypto.PrivateKey
}

func (a *account) GetEmail() string                        { return a.Email }
func (a *account) GetRegistration() *registration.Resource { return a.Registration }
func (a *account) GetPrivateKey() crypto.PrivateKey        { return a.key }

// Manager keeps the settings, the account and the certificate in
// <root>/ca/acme.
type Manager struct {
	Root string
	now  func() time.Time
	// lookupCNAME is net.LookupCNAME, another one in the tests.
	lookupCNAME func(string) (string, error)

	// For the tests: another ACME server and DNS.
	directory  string
	client     *http.Client
	provider   challenge.Provider
	dnsOptions []dns01.ChallengeOption
}

func New(root string) *Manager {
	return &Manager{Root: root, now: time.Now, lookupCNAME: net.LookupCNAME}
}

// lego logs through a global logger and providers read their settings from
// the environment: one certificate at a time.
var mu sync.Mutex

func (m *Manager) Dir() string      { return domaincert.Dir(m.Root) }
func (m *Manager) CertFile() string { return filepath.Join(m.Dir(), "cert.crt") }
func (m *Manager) KeyFile() string  { return filepath.Join(m.Dir(), "cert.key") }

func (m *Manager) path(name string) string { return filepath.Join(m.Dir(), name) }

// Settings returns the settings, nil when no domain is set.
func (m *Manager) Settings() (*Settings, error) {
	data, err := os.ReadFile(m.path("settings.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s Settings
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("settings.yaml: %w", err)
	}
	if s.Domain == "" {
		return nil, nil
	}
	return &s, nil
}

// SaveSettings checks and saves s. Empty secret fields keep the values
// saved before: the panel does not show them.
func (m *Manager) SaveSettings(s Settings) error {
	s.Domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s.Domain)), ".")
	s.Email = strings.TrimSpace(s.Email)
	if err := checkDomain(s.Domain); err != nil {
		return err
	}
	if s.Email != "" && !strings.Contains(s.Email, "@") {
		return fmt.Errorf("%q is not an e-mail address", s.Email)
	}
	p := FindProvider(s.Provider)
	if p == nil {
		return fmt.Errorf("unknown DNS provider %q", s.Provider)
	}
	old, err := m.Settings()
	if err != nil {
		return err
	}
	fields := map[string]string{}
	for _, f := range p.Fields {
		v := strings.TrimSpace(s.Fields[f.Env])
		if v == "" && f.Secret && old != nil && old.Provider == s.Provider {
			v = old.Fields[f.Env]
		}
		if v == "" && !f.Optional {
			return fmt.Errorf("%s: %s is required", p.Label, f.Label["en"])
		}
		if v != "" {
			fields[f.Env] = v
		}
	}
	s.Fields = fields
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return m.writeFile("settings.yaml", data, 0o600)
}

// Disable removes the settings with the keys of the DNS provider: the
// add-ons go back to the certificate of the device. The account and the
// last certificate are kept.
func (m *Manager) Disable() error {
	mu.Lock()
	defer mu.Unlock()
	for _, name := range []string{"settings.yaml", "state.json"} {
		if err := os.Remove(m.path(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// checkDomain accepts a name in public DNS: no wildcard, no .local.
func checkDomain(d string) error {
	if len(d) > 253 || !strings.Contains(d, ".") {
		return fmt.Errorf("%q is not a domain name", d)
	}
	if strings.HasSuffix(d, ".local") || strings.HasSuffix(d, ".localhost") {
		return fmt.Errorf("%s: .local names are not in public DNS", d)
	}
	for _, label := range strings.Split(d, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return fmt.Errorf("%q is not a domain name", d)
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return fmt.Errorf("%q is not a domain name", d)
			}
		}
	}
	return nil
}

func (s *Settings) directory(m *Manager) string {
	switch {
	case m.directory != "":
		return m.directory
	case s.Staging:
		return Staging
	}
	return Production
}

// Current returns the certificate when there is one for the domain set and
// it has not expired.
func (m *Manager) Current() (*tls.Certificate, error) {
	c, err := domaincert.Load(m.Root, m.now())
	if err != nil || c == nil {
		return nil, err
	}
	return &c.Pair, nil
}

// Status returns the state of the certificate, nil when no domain is set.
func (m *Manager) Status() (*Status, error) {
	s, err := m.Settings()
	if err != nil || s == nil {
		return nil, err
	}
	st := m.loadState()
	out := &Status{Domain: s.Domain, LastError: st.Error}
	if !st.Attempt.IsZero() {
		out.LastAttempt = &st.Attempt
	}
	if crt := m.leaf(); crt != nil && crt.VerifyHostname(s.Domain) == nil {
		out.Issuer = crt.Issuer.CommonName
		out.NotAfter = &crt.NotAfter
	}
	if next := m.retryAt(st); st.Failures > 0 {
		out.NextAttempt = &next
	}
	return out, nil
}

// Obtain gets a new certificate now, logging to w.
func (m *Manager) Obtain(w io.Writer) error {
	mu.Lock()
	defer mu.Unlock()
	defer logTo(w)()
	s, err := m.Settings()
	if err != nil {
		return err
	}
	if s == nil {
		return errors.New("no domain is set")
	}
	return m.obtain(s, w)
}

// Renew gets a certificate when it is time to: there is none for the
// domain yet, 2/3 of its validity has passed or ARI says so. Not too
// soon after a failed try. renewed tells that there is a new one.
func (m *Manager) Renew(w io.Writer) (renewed bool, err error) {
	mu.Lock()
	defer mu.Unlock()
	defer logTo(w)()
	s, err := m.Settings()
	if err != nil || s == nil {
		return false, err
	}
	st := m.loadState()
	if st.Failures > 0 && m.now().Before(m.retryAt(st)) {
		return false, nil
	}
	if !m.due(s, st, w) {
		return false, nil
	}
	return true, m.obtain(s, w)
}

// logTo sends the log of lego to w until the returned func is called.
func logTo(w io.Writer) func() {
	old := legolog.Logger
	legolog.Logger = log.New(w, "", 0)
	return func() { legolog.Logger = old }
}

func (m *Manager) retryAt(st *state) time.Time {
	wait := firstRetry
	for i := 1; i < st.Failures && wait < maxRetry; i++ {
		wait *= 2
	}
	return st.Attempt.Add(min(wait, maxRetry))
}

func (m *Manager) due(s *Settings, st *state, w io.Writer) bool {
	crt := m.leaf()
	now := m.now()
	if crt == nil || crt.VerifyHostname(s.Domain) != nil || st.Directory != s.directory(m) {
		return true
	}
	if now.After(crt.NotBefore.Add(crt.NotAfter.Sub(crt.NotBefore) * 2 / 3)) {
		return true
	}
	if now.Sub(st.ARIChecked) >= ariInterval {
		m.checkARI(s, st, crt, w)
	}
	return !st.RenewAt.IsZero() && !now.Before(st.RenewAt)
}

// checkARI asks the server when to renew crt. A failure only gets logged:
// the certificate is then renewed by its age.
func (m *Manager) checkARI(s *Settings, st *state, crt *x509.Certificate, w io.Writer) {
	st.ARIChecked = m.now()
	defer m.saveState(st)
	client, err := m.legoClient(s, w, false)
	if err != nil {
		fmt.Fprintf(w, "Renewal info: %v\n", err)
		return
	}
	info, err := client.Certificate.GetRenewalInfo(certificate.RenewalInfoRequest{Cert: crt})
	if err != nil {
		fmt.Fprintf(w, "Renewal info: %v\n", err)
		return
	}
	start, end := info.SuggestedWindow.Start, info.SuggestedWindow.End
	st.RenewAt = start
	if window := end.Sub(start); window > 0 {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(window)))
		st.RenewAt = start.Add(time.Duration(n.Int64()))
	}
}

func (m *Manager) obtain(s *Settings, w io.Writer) error {
	st := m.loadState()
	st.Attempt = m.now()
	err := m.get(s, st, w)
	if err != nil {
		st.Error = err.Error()
		st.Failures++
	} else {
		st.Error, st.Failures = "", 0
		st.ARIChecked, st.RenewAt = time.Time{}, time.Time{}
		st.Directory = s.directory(m)
	}
	if err := m.saveState(st); err != nil {
		return err
	}
	return err
}

func (m *Manager) get(s *Settings, st *state, w io.Writer) error {
	client, err := m.legoClient(s, w, true)
	if err != nil {
		return err
	}
	provider := m.provider
	if provider == nil {
		if provider, err = m.newProvider(s); err != nil {
			return err
		}
	}
	var options []dns01.ChallengeOption
	if p := FindProvider(s.Provider); !p.delegation || !m.followCNAME(s.Domain, w) {
		defer setenv("LEGO_DISABLE_CNAME_SUPPORT", "true")()
		wait := p.wait
		if wait == 0 {
			wait = 2 * time.Minute
		}
		options = append(options, dns01.WrapPreCheck(exactCheck(s.Domain, wait, w)))
	}
	if err := client.Challenge.SetDNS01Provider(provider, append(options, m.dnsOptions...)...); err != nil {
		return err
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	req := certificate.ObtainRequest{Domains: []string{s.Domain}, Bundle: true, PrivateKey: key}
	// A renewal says which certificate it replaces: Let's Encrypt does
	// not count those in its rate limits.
	if crt := m.leaf(); crt != nil && st.Directory == s.directory(m) && crt.VerifyHostname(s.Domain) == nil {
		req.ReplacesCertID, _ = certificate.MakeARICertID(crt)
	}
	fmt.Fprintf(w, "Getting a certificate for %s from %s\n", s.Domain, s.directory(m))
	res, err := client.Certificate.Obtain(req)
	if err != nil && req.ReplacesCertID != "" {
		fmt.Fprintf(w, "%v\nTrying again as a new certificate\n", err)
		req.ReplacesCertID = ""
		res, err = client.Certificate.Obtain(req)
	}
	if err != nil {
		return err
	}
	if err := m.writeFile("cert.key", res.PrivateKey, 0o600); err != nil {
		return err
	}
	if err := m.writeFile("cert.crt", res.Certificate, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(w, "Certificate for %s saved\n", s.Domain)
	return nil
}

// legoClient returns a lego client with the account of the server of s,
// registered when register is set and there is none yet.
func (m *Manager) legoClient(s *Settings, w io.Writer, register bool) (*lego.Client, error) {
	dir := s.directory(m)
	a, err := m.loadAccount()
	if err != nil {
		return nil, err
	}
	if a == nil || a.Directory != dir || a.Email != s.Email {
		if !register {
			return nil, errors.New("no account yet")
		}
		key, err := certcrypto.GeneratePrivateKey(certcrypto.EC256)
		if err != nil {
			return nil, err
		}
		a = &account{Directory: dir, Email: s.Email, key: key}
	}
	cfg := lego.NewConfig(a)
	cfg.CADirURL = dir
	cfg.UserAgent = "SuplaOS"
	cfg.Certificate.KeyType = certcrypto.EC256
	if m.client != nil {
		cfg.HTTPClient = m.client
	}
	client, err := lego.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	if a.Registration == nil {
		fmt.Fprintf(w, "Making an account at %s\n", dir)
		if a.Registration, err = client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true}); err != nil {
			return nil, err
		}
		if err := m.saveAccount(a); err != nil {
			return nil, err
		}
	}
	return client, nil
}

// followCNAME tells whether the TXT record goes where a CNAME of
// _acme-challenge.<domain> points: only when it points out of the domain,
// to a zone set aside for the challenge (e.g. at DuckDNS). A wildcard
// CNAME of the zone itself also answers for _acme-challenge, then the
// record goes under the name itself, which comes before the wildcard.
// Only asked for providers used for such zones: the answer stays in the
// cache of the resolver.
func (m *Manager) followCNAME(domain string, w io.Writer) bool {
	name := "_acme-challenge." + domain + "."
	target, err := m.lookupCNAME(name)
	if err != nil || strings.EqualFold(target, name) {
		return true
	}
	base, err1 := publicsuffix.EffectiveTLDPlusOne(domain)
	to, err2 := publicsuffix.EffectiveTLDPlusOne(strings.TrimSuffix(target, "."))
	if err1 != nil || err2 != nil || !strings.EqualFold(base, to) {
		fmt.Fprintf(w, "%s points to %s, the record goes there\n", name, target)
		return true
	}
	return false
}

var errNotAuthoritative = errors.New("not authoritative")

// exactCheck checks that all the authoritative servers of the zone of
// domain have the record under fqdn itself. The check of lego asks the
// resolver first and follows a CNAME, also one of a wildcard the resolver
// got before the record was there. When something on the way answers DNS
// queries itself (some routers, WSL), the servers cannot be asked: then it
// waits wait and leaves the check to Let's Encrypt.
func exactCheck(domain string, wait time.Duration, w io.Writer) dns01.WrapPreCheckFunc {
	start := time.Now()
	told := false
	return func(_, fqdn, value string, _ dns01.PreCheckFunc) (bool, error) {
		found, err := authoritative(domain, fqdn, value)
		if !errors.Is(err, errNotAuthoritative) {
			return found, err
		}
		if !told {
			fmt.Fprintf(w, "%v: waiting %s for the record instead\n", err, wait)
			told = true
		}
		return time.Since(start) >= wait, nil
	}
}

func authoritative(domain, fqdn, value string) (bool, error) {
	// The zone of the domain, not of fqdn: a lookup of fqdn would leave
	// the CNAME of a wildcard in the resolver.
	zone, err := dns01.FindZoneByFqdn(domain + ".")
	if err != nil {
		return false, err
	}
	servers, err := net.LookupNS(zone)
	if err != nil {
		return false, err
	}
	client := &dns.Client{Timeout: 10 * time.Second}
	for _, ns := range servers {
		msg := new(dns.Msg)
		msg.SetQuestion(fqdn, dns.TypeTXT)
		msg.RecursionDesired = false
		in, _, err := client.Exchange(msg, net.JoinHostPort(ns.Host, "53"))
		if err != nil {
			return false, err
		}
		if !in.Authoritative {
			return false, fmt.Errorf("the answer of %s is %w, something on the way answers DNS queries itself", ns.Host, errNotAuthoritative)
		}
		found := false
		for _, rr := range in.Answer {
			if txt, ok := rr.(*dns.TXT); ok && strings.Join(txt.Txt, "") == value {
				found = true
			}
		}
		if !found {
			return false, fmt.Errorf("%s does not have the TXT record of %s yet", ns.Host, fqdn)
		}
	}
	return true, nil
}

// setenv sets an environment variable until the returned func is called.
func setenv(k, v string) func() {
	old, had := os.LookupEnv(k)
	os.Setenv(k, v)
	if had {
		return func() { os.Setenv(k, old) }
	}
	return func() { os.Unsetenv(k) }
}

// newProvider makes the DNS provider of s, with its fields in the
// environment while lego reads them.
func (m *Manager) newProvider(s *Settings) (challenge.Provider, error) {
	p := FindProvider(s.Provider)
	if p == nil {
		return nil, fmt.Errorf("unknown DNS provider %q", s.Provider)
	}
	env := maps.Clone(p.env)
	if env == nil {
		env = map[string]string{}
	}
	for _, f := range p.Fields {
		if v := s.Fields[f.Env]; v != "" {
			env[f.Env] = v
		} else if f.Default != "" {
			env[f.Env] = f.Default
		}
	}
	if p.Name == "acmedns" {
		env["ACME_DNS_STORAGE_PATH"] = m.path("acme-dns.json")
	}
	for k, v := range env {
		defer setenv(k, v)()
	}
	provider, err := p.new()
	if err != nil {
		return nil, err
	}
	return provider, nil
}

func (m *Manager) leaf() *x509.Certificate {
	data, err := os.ReadFile(m.CertFile())
	if err != nil {
		return nil
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil
	}
	crt, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil
	}
	return crt
}

func (m *Manager) loadAccount() (*account, error) {
	data, err := os.ReadFile(m.path("account.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var a account
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("account.json: %w", err)
	}
	if a.key, err = certcrypto.ParsePEMPrivateKey([]byte(a.Key)); err != nil {
		return nil, fmt.Errorf("account.json: %w", err)
	}
	return &a, nil
}

func (m *Manager) saveAccount(a *account) error {
	a.Key = string(certcrypto.PEMEncode(a.key))
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	return m.writeFile("account.json", data, 0o600)
}

func (m *Manager) loadState() *state {
	var st state
	if data, err := os.ReadFile(m.path("state.json")); err == nil {
		json.Unmarshal(data, &st)
	}
	return &st
}

func (m *Manager) saveState(st *state) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return m.writeFile("state.json", data, 0o600)
}

// writeFile writes a file in Dir through a temporary one. Files made by
// root (the panel) are given to the owner of the data directory, like
// those of the stack.
func (m *Manager) writeFile(name string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(m.Dir(), 0o700); err != nil {
		return err
	}
	m.chown(m.Dir())
	path := m.path(name)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	m.chown(tmp)
	return os.Rename(tmp, path)
}

func (m *Manager) chown(path string) {
	if os.Geteuid() != 0 {
		return
	}
	st, err := os.Stat(m.Root)
	if err != nil {
		return
	}
	if sys, ok := st.Sys().(*syscall.Stat_t); ok {
		os.Lchown(path, int(sys.Uid), int(sys.Gid))
	}
}
