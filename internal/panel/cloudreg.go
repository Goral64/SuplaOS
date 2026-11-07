package panel

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Goral64/SuplaOS/internal/domaincert"
	"github.com/Goral64/SuplaOS/internal/stack"
)

// A Supla Cloud registered at cloud.supla.org works with the apps and
// services of SUPLA (push notifications, OAuth apps). cloud.supla.org gives
// a token for the address of the Cloud after connecting to it over HTTPS;
// the Cloud registers itself with the token (supla:register-target-cloud)
// and keeps the token it gets back in supla_settings_string.

const registerCloudURL = "https://cloud.supla.org/register-cloud"

// cloudSettings are the rows of supla_settings_string about the
// registration, as the Cloud names them.
var cloudSettings = []string{"target_token", "allow_tge_reports", "allow_notifications", "ad_notifications_limit"}

// regCheck is one condition of the registration: ok, warn (could not be
// checked from here, or may be fine) or fail.
type regCheck struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Reason string `json:"reason,omitempty"`
	Detail string `json:"detail,omitempty"`
}

type registrationView struct {
	Registered    bool       `json:"registered"`
	Address       string     `json:"address"`
	RegisterURL   string     `json:"registerURL"`
	Notifications string     `json:"notifications"`
	Limit         string     `json:"limit"`
	Checks        []regCheck `json:"checks"`
	Ready         bool       `json:"ready"`
}

// cloudSettingValues reads the registration rows of the Cloud, without the
// token itself.
func cloudSettingValues(ctx context.Context, st *stack.Stack) (map[string]string, error) {
	q := "select name, if(name = 'target_token', '', value) from supla_settings_string where name in ('" +
		strings.Join(cloudSettings, "', '") + "')"
	cmd := st.Command(ctx, "exec", "-T", "supla-db", "sh", "-c",
		`exec mariadb -N -B --user=supla --password="$MYSQL_PASSWORD" supla -e "$0"`, q)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("database not available: %w", err)
	}
	values := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if k, v, ok := strings.Cut(line, "\t"); ok {
			values[k] = v
		} else if line != "" {
			values[line] = ""
		}
	}
	return values, nil
}

// cloudHost splits the address of the Cloud (SUPLA_HOST_ADDRESS) into the
// host and the port.
func cloudHost(address string) (string, string) {
	u, err := url.Parse("https://" + strings.TrimPrefix(address, "https://"))
	if err != nil {
		return address, ""
	}
	return strings.ToLower(u.Hostname()), u.Port()
}

// checkName: cloud.supla.org takes only domain names, without SUPLA in the
// last two parts (supla.io excepted).
func checkName(host string) regCheck {
	c := regCheck{ID: "name", State: "fail"}
	parts := strings.Split(host, ".")
	slices.Reverse(parts)
	switch {
	case net.ParseIP(host) != nil:
		c.Reason = "ip"
	case len(parts) < 2 || parts[0] == "" || parts[1] == "":
		c.Reason = "notDomain"
	case parts[0] == "local":
		c.Reason = "local"
	case (strings.Contains(parts[0], "supla") || strings.Contains(parts[1], "supla")) && parts[1]+"."+parts[0] != "supla.io":
		c.Reason = "supla"
	default:
		c.State = "ok"
	}
	return c
}

// dohServers answer DNS over HTTPS, which a router that catches DNS
// queries (to give its own answers for names of the local network) does
// not see, unlike queries to a public server on port 53.
var dohServers = []string{
	"https://cloudflare-dns.com/dns-query",
	"https://dns.google/resolve",
}

var errNotFound = errors.New("no such host")

// lookupDoH asks a DoH server in the JSON format for the A and AAAA
// records of host.
func lookupDoH(ctx context.Context, server, host string) ([]string, error) {
	var addrs []string
	for _, typ := range []string{"A", "AAAA"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			server+"?name="+url.QueryEscape(host)+"&type="+typ, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/dns-json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		var answer struct {
			Status int
			Answer []struct {
				Type int    `json:"type"`
				Data string `json:"data"`
			}
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&answer)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s: %s", server, resp.Status)
		}
		switch answer.Status {
		case 0:
		case 3:
			return nil, errNotFound
		default:
			return nil, fmt.Errorf("%s: DNS status %d", server, answer.Status)
		}
		for _, a := range answer.Answer {
			if a.Type == 1 || a.Type == 28 {
				addrs = append(addrs, a.Data)
			}
		}
	}
	return addrs, nil
}

// publicResolver asks a public DNS server, so that a name the local network
// resolves to the device does not hide what cloud.supla.org gets.
var publicResolver = &net.Resolver{
	PreferGo: true,
	Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, "1.1.1.1:53")
	},
}

// lookupHost resolves host as the internet sees it, if it can: sure is
// false when the answer may come from the local network.
var lookupHost = func(ctx context.Context, host string) (addrs []string, sure bool, err error) {
	for _, server := range dohServers {
		dctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		addrs, err := lookupDoH(dctx, server, host)
		cancel()
		if err == nil || errors.Is(err, errNotFound) {
			return addrs, true, err
		}
	}
	addrs, err = publicResolver.LookupHost(ctx, host)
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return nil, false, err
		}
		// No public DNS from here: the one of the system.
		addrs, err = net.DefaultResolver.LookupHost(ctx, host)
	}
	return addrs, false, err
}

func isPrivate(ip netip.Addr) bool {
	ip = ip.Unmap()
	return slices.ContainsFunc(privateNets, func(n netip.Prefix) bool { return n.Contains(ip) })
}

// checkDNS: the name has to point to a public address.
func checkDNS(ctx context.Context, host string) regCheck {
	c := regCheck{ID: "dns", State: "fail"}
	addrs, sure, err := lookupHost(ctx, host)
	if err != nil || len(addrs) == 0 {
		c.Reason = "notFound"
		return c
	}
	c.Detail = strings.Join(addrs, ", ")
	for _, a := range addrs {
		if ip, err := netip.ParseAddr(a); err == nil && !isPrivate(ip) {
			c.State = "ok"
			return c
		}
	}
	c.Reason = "private"
	if !sure {
		// The router may give its own answer for the name; cloud.supla.org
		// checks it anyway.
		c.State, c.Reason = "warn", "local"
	}
	return c
}

// checkCertificate: a certificate of the domain from the panel, trusted
// (not from the staging server of Let's Encrypt). Without one the Cloud
// may still be served by a proxy with its own, e.g. a Cloudflare tunnel.
func (s *Server) checkCertificate(host string, https regCheck) regCheck {
	c := regCheck{ID: "certificate", State: "ok"}
	cert, _ := domaincert.Load(s.cfg.Root, time.Now())
	if set, _ := s.acme.Settings(); set != nil && set.Staging && cert != nil {
		c.State, c.Reason = "fail", "staging"
		return c
	}
	if cert == nil || cert.Pair.Leaf.VerifyHostname(host) != nil {
		if https.State != "ok" {
			c.State, c.Reason = "warn", "none"
		}
		return c
	}
	c.Detail = cert.Domain
	return c
}

// checkHTTPS asks the Cloud at its address, as cloud.supla.org does, with
// the certificate checked. It goes through the router, which not every
// router lets the device do (or the router answers itself): so only a
// warning when it fails.
func checkHTTPS(ctx context.Context, address string) regCheck {
	c := regCheck{ID: "https", State: "warn"}
	client := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{}}}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://"+address+"/api/v2.3.0/server-info", nil)
	if err != nil {
		c.Reason, c.Detail = "unreachable", err.Error()
		return c
	}
	req.Header.Set("Accept", "application/json")
	res, err := client.Do(req)
	if err != nil {
		var cert *tls.CertificateVerificationError
		if errors.As(err, &cert) {
			c.Reason, c.Detail = "certificate", cert.Err.Error()
		} else {
			c.Reason, c.Detail = "unreachable", err.Error()
			if u, ok := err.(*url.Error); ok {
				c.Detail = u.Err.Error()
			}
		}
		return c
	}
	defer res.Body.Close()
	var info struct {
		CloudVersion string `json:"cloudVersion"`
	}
	data, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK || json.Unmarshal(data, &info) != nil || info.CloudVersion == "" {
		c.Reason, c.Detail = "notCloud", res.Status
		return c
	}
	c.State, c.Detail = "ok", info.CloudVersion
	return c
}

// registrationChecks are the conditions cloud.supla.org checks, as far as
// they can be checked from here.
func (s *Server) registrationChecks(ctx context.Context, address string) []regCheck {
	host, _ := cloudHost(address)
	name := checkName(host)
	if name.State != "ok" {
		return []regCheck{name}
	}
	dns := checkDNS(ctx, host)
	if dns.State == "fail" {
		return []regCheck{name, dns}
	}
	https := checkHTTPS(ctx, address)
	return []regCheck{name, dns, s.checkCertificate(host, https), https}
}

func checksPass(checks []regCheck) bool {
	return !slices.ContainsFunc(checks, func(c regCheck) bool { return c.State == "fail" })
}

func (s *Server) getRegistration(w http.ResponseWriter, r *http.Request) {
	st, err := s.openStack()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	if !st.IsInstalled("supla") {
		jsonError(w, http.StatusNotFound, errors.New("supla is not installed"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	values, err := cloudSettingValues(ctx, st)
	if err != nil {
		jsonError(w, http.StatusServiceUnavailable, err)
		return
	}
	address := st.Values(st.Catalog["supla"])["SUPLA_HOST_ADDRESS"]
	_, registered := values["target_token"]
	v := registrationView{
		Registered:    registered,
		Address:       address,
		RegisterURL:   registerCloudURL + "?domain=" + url.QueryEscape(address),
		Notifications: values["allow_notifications"],
		Limit:         values["ad_notifications_limit"],
		Checks:        []regCheck{},
	}
	if !registered {
		v.Checks = s.registrationChecks(ctx, address)
		v.Ready = checksPass(v.Checks)
	}
	writeJSON(w, http.StatusOK, v)
}

// registrationToken takes the token from what was pasted: the token, or
// the whole command cloud.supla.org shows.
var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9._~+/=-]{8,512}$`)

func registrationToken(s string) (string, error) {
	f := strings.Fields(s)
	if len(f) == 0 || !tokenPattern.MatchString(f[len(f)-1]) {
		return "", errors.New("paste the token or the command from cloud.supla.org")
	}
	return f[len(f)-1], nil
}

func (s *Server) registerCloud(w http.ResponseWriter, r *http.Request) {
	var req struct{ Token string }
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	token, err := registrationToken(req.Token)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	s.stackJob(w, "register Supla Cloud", func(st *stack.Stack, j *Job) error {
		if !st.IsInstalled("supla") {
			return errors.New("supla is not installed")
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		values, err := cloudSettingValues(ctx, st)
		if err != nil {
			return err
		}
		if _, ok := values["target_token"]; ok {
			return errors.New("this Supla Cloud is registered already")
		}
		address := st.Values(st.Catalog["supla"])["SUPLA_HOST_ADDRESS"]
		j.Logf("Checking https://%s", address)
		for _, c := range s.registrationChecks(ctx, address) {
			j.Logf("  %s: %s %s %s", c.ID, c.State, c.Reason, c.Detail)
			if c.State == "fail" {
				return errors.New("the Cloud does not meet the conditions of the registration")
			}
		}
		j.Logf("Registering https://%s at cloud.supla.org", address)
		if err := st.Run("supla", "console", []string{"supla:register-target-cloud", token}); err != nil {
			return fmt.Errorf("registration failed: %w", errors.Unwrap(err))
		}
		// Whether the Cloud may send push notifications, and how many.
		j.Logf("Fetching the settings of the registration")
		if err := st.Run("supla", "console", []string{"supla:ad:fetch-settings", "-v"}); err != nil {
			j.Logf("Could not fetch them now; the Cloud does it again later.")
		}
		return nil
	})
}

// forgetRegistration removes the registration from this Cloud only, e.g.
// after it was undone at cloud.supla.org or to register another address.
func (s *Server) forgetRegistration(w http.ResponseWriter, r *http.Request) {
	st, err := s.openStack()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	if !st.IsInstalled("supla") {
		jsonError(w, http.StatusNotFound, errors.New("supla is not installed"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	q := "delete from supla_settings_string where name in ('" + strings.Join(cloudSettings, "', '") + "')"
	cmd := st.Command(ctx, "exec", "-T", "supla-db", "sh", "-c",
		`exec mariadb --user=supla --password="$MYSQL_PASSWORD" supla -e "$0"`, q)
	if out, err := cmd.CombinedOutput(); err != nil {
		jsonError(w, http.StatusServiceUnavailable, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out))))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}
