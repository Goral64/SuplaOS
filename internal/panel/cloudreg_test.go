package panel

import (
	"context"
	"errors"
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
	defer func(f func(context.Context, string) ([]string, error)) { lookupHost = f }(lookupHost)
	answers := map[string][]string{
		"public.example.com":  {"192.168.1.5", "203.0.113.7"},
		"private.example.com": {"192.168.1.5", "fd00::1"},
	}
	lookupHost = func(_ context.Context, host string) ([]string, error) {
		if a, ok := answers[host]; ok {
			return a, nil
		}
		return nil, errors.New("no such host")
	}
	for host, want := range map[string]string{
		"public.example.com":  "ok",
		"private.example.com": "private",
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
