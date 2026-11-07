package panel

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckName(t *testing.T) {
	for host, want := range map[string]string{
		"myhome.duckdns.org":    "",
		"mojasupla.duckdns.org": "",
		"cloud.example.com":     "",
		"home.supla.io":         "",
		"192.168.1.10":          "ip",
		"suplaos.local":         "local",
		"localhost":             "notDomain",
		"cloud.mysupla.com":     "supla",
		"cloud.example.supla":   "supla",
	} {
		c := checkName(host)
		if c.Reason != want || (want == "") != (c.State == "ok") {
			t.Errorf("%s: %s %q, want %q", host, c.State, c.Reason, want)
		}
	}
}

func TestCloudHost(t *testing.T) {
	for address, want := range map[string][2]string{
		"Home.Example.com":      {"home.example.com", ""},
		"home.example.com:8443": {"home.example.com", "8443"},
		"192.168.1.10":          {"192.168.1.10", ""},
	} {
		if h, p := cloudHost(address); h != want[0] || p != want[1] {
			t.Errorf("%s: %s %s", address, h, p)
		}
	}
}

func TestCheckDNS(t *testing.T) {
	defer func(f func(context.Context, string) ([]string, bool, error)) { lookupHost = f }(lookupHost)
	answers := map[string][]string{
		"public.example.com":  {"192.168.1.5", "203.0.113.7"},
		"private.example.com": {"192.168.1.5", "fd00::1"},
		"local.example.com":   {"192.168.1.5"},
	}
	lookupHost = func(_ context.Context, host string) ([]string, bool, error) {
		if a, ok := answers[host]; ok {
			return a, host != "local.example.com", nil
		}
		return nil, true, errors.New("no such host")
	}
	for host, want := range map[string]string{
		"public.example.com":  "ok",
		"private.example.com": "private",
		"local.example.com":   "local",
		"none.example.com":    "notFound",
	} {
		c := checkDNS(context.Background(), host)
		if got := c.Reason; (want == "ok" && c.State != "ok") || (want != "ok" && got != want) {
			t.Errorf("%s: %s %q, want %s", host, c.State, c.Reason, want)
		}
	}
}

func TestRegistrationToken(t *testing.T) {
	for in, want := range map[string]string{
		"abcDEF123456":    "abcDEF123456",
		" abcDEF123456\n": "abcDEF123456",
		"docker exec -it -u www-data supla-cloud php bin/console supla:register-target-cloud abc_DEF-123.456": "abc_DEF-123.456",
		"":           "",
		"short":      "",
		"bad;token$": "",
	} {
		got, err := registrationToken(in)
		if got != want || (want == "") != (err != nil) {
			t.Errorf("%q: %q %v", in, got, err)
		}
	}
}

func TestLookupDoH(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("name") + " " + r.URL.Query().Get("type") {
		case "a.example.com A":
			w.Write([]byte(`{"Status":0,"Answer":[{"type":5,"data":"b.example.com."},{"type":1,"data":"203.0.113.7"}]}`))
		case "a.example.com AAAA":
			w.Write([]byte(`{"Status":0}`))
		default:
			w.Write([]byte(`{"Status":3}`))
		}
	}))
	defer srv.Close()
	addrs, err := lookupDoH(context.Background(), srv.URL, "a.example.com")
	if err != nil || len(addrs) != 1 || addrs[0] != "203.0.113.7" {
		t.Errorf("a.example.com: %v %v", addrs, err)
	}
	if _, err := lookupDoH(context.Background(), srv.URL, "none.example.com"); !errors.Is(err, errNotFound) {
		t.Errorf("none.example.com: %v", err)
	}
}
