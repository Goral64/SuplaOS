package panel

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"
)

// Requests of httptest come from 192.0.2.1: the tests take it for the
// local network, besides a public IPv6 one.
func TestMain(m *testing.M) {
	interfaceNets = func() ([]netip.Prefix, error) {
		return []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24"), netip.MustParsePrefix("2a01:4f8:1:2::/64")}, nil
	}
	os.Exit(m.Run())
}

func request(remote string, headers ...string) *http.Request {
	r := httptest.NewRequest("GET", "/api/session", nil)
	r.RemoteAddr = remote
	for i := 0; i+1 < len(headers); i += 2 {
		r.Header.Add(headers[i], headers[i+1])
	}
	return r
}

func TestFromInternet(t *testing.T) {
	for _, c := range []struct {
		r        *http.Request
		internet bool
		ip       string
	}{
		{request("192.168.1.20:5000"), false, "192.168.1.20"},
		{request("[::ffff:10.0.0.3]:5000"), false, "10.0.0.3"},
		{request("[2a01:4f8:1:2::77]:5000"), false, "2a01:4f8:1:2::77"}, // public IPv6 at home
		{request("100.101.102.103:5000"), false, "100.101.102.103"},
		{request("8.8.8.8:5000"), true, "8.8.8.8"},
		{request("[2a00:1450::1]:5000"), true, "2a00:1450::1"},
		// Through a proxy in the local network.
		{request("192.168.1.1:5000", "Cf-Connecting-Ip", "8.8.4.4"), true, "8.8.4.4"},
		{request("192.168.1.1:5000", "X-Forwarded-For", "192.168.1.9, 8.8.4.4"), true, "192.168.1.9"},
		{request("192.168.1.1:5000", "Forwarded", `for="[2a00:1450::2]:4711";proto=https`), true, "2a00:1450::2"},
		{request("192.168.1.1:5000", "X-Real-Ip", "192.168.1.30"), false, "192.168.1.30"},
		// Headers from the internet are not believed for the address.
		{request("8.8.8.8:5000", "Cf-Connecting-Ip", "192.168.1.30"), true, "8.8.8.8"},
	} {
		if got := fromInternet(c.r); got != c.internet {
			t.Errorf("%s %v: internet %v", c.r.RemoteAddr, c.r.Header, got)
		}
		if got := clientIP(c.r); got != c.ip {
			t.Errorf("%s %v: client %s, want %s", c.r.RemoteAddr, c.r.Header, got, c.ip)
		}
	}
}

func TestInternetAccess(t *testing.T) {
	s := newServer(t)
	h := s.Handler()
	token := s.sessions.create(false)
	send := func(method, path, body, remote string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.RemoteAddr = remote
		r.AddCookie(&http.Cookie{Name: cookieName, Value: token})
		r.Header.Set("X-SuplaOS", "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	ps := &proxyServer{addon: "zigbee2mqtt", upstream: upstream{service: "zigbee2mqtt", port: "8080"}}
	z2m := func(remote string) int {
		r := httptest.NewRequest("GET", "https://supla-z2m.example.com/", nil)
		r.RemoteAddr = remote
		w := httptest.NewRecorder()
		s.proxyHandler(ps, "").ServeHTTP(w, r)
		return w.Code
	}
	if w := send("GET", "/api/session", "", "8.8.8.8:1"); w.Code != http.StatusForbidden {
		t.Errorf("panel from the internet: %d", w.Code)
	}
	if code := z2m("8.8.8.8:1"); code != http.StatusForbidden {
		t.Errorf("z2m from the internet: %d", code)
	}
	if code := z2m("192.168.1.5:1"); code != http.StatusFound {
		t.Errorf("z2m at home: %d", code)
	}

	st := routerStack(t, s)
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	// Only the panel and the proxied add-ons can be turned on.
	if w := send("PUT", "/api/internet", `{"allowed":["zigbee2mqtt","supla-scripts"]}`, "192.168.1.5:1"); w.Code != http.StatusOK {
		t.Fatalf("put: %d %s", w.Code, w.Body)
	}
	if got := s.getSettings().Internet; len(got) != 1 || got[0] != "zigbee2mqtt" {
		t.Errorf("allowed %v", got)
	}
	// An add-on needs the panel, to log in.
	if code := z2m("8.8.8.8:1"); code != http.StatusForbidden {
		t.Errorf("z2m without the panel: %d", code)
	}
	send("PUT", "/api/internet", `{"allowed":["panel","zigbee2mqtt"]}`, "192.168.1.5:1")
	if w := send("GET", "/api/internet", "", "8.8.8.8:1"); w.Code != http.StatusOK ||
		!strings.Contains(w.Body.String(), `"id":"panel","label":{"en":"SuplaOS panel","pl":"Panel SuplaOS"},"allowed":true`) ||
		!strings.Contains(w.Body.String(), `"fromInternet":true`) {
		t.Errorf("get: %d %s", w.Code, w.Body)
	}
	if code := z2m("8.8.8.8:1"); code != http.StatusFound {
		t.Errorf("z2m allowed: %d", code)
	}
}

func TestLoginThrottle(t *testing.T) {
	var s sessions
	for range maxFailures {
		s.result("8.8.8.8", false)
	}
	if !s.throttled("8.8.8.8") || s.throttled("192.168.1.5") {
		t.Error("not counted by address")
	}
	// Each next wrong password doubles the wait, up to an hour.
	f := s.failed["8.8.8.8"]
	f.last = time.Now().Add(-failDelay - time.Second)
	if s.throttled("8.8.8.8") {
		t.Error("still waiting after failDelay")
	}
	f.count = maxFailures + 2
	if !s.throttled("8.8.8.8") {
		t.Error("not waiting longer")
	}
	f.count, f.last = 100, time.Now().Add(-maxFailDelay-time.Second)
	if s.throttled("8.8.8.8") {
		t.Error("waiting more than maxFailDelay")
	}
	s.result("8.8.8.8", true)
	if s.failed["8.8.8.8"] != nil {
		t.Error("not cleared after a good password")
	}
}
