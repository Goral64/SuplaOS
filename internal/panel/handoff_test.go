package panel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Goral64/SuplaOS/internal/acme"
)

func TestHandoff(t *testing.T) {
	s := newServer(t)
	h := s.Handler()
	st := routerStack(t, s)
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	if err := s.acme.SaveSettings(acme.Settings{Domain: "supla.example.com", Provider: "duckdns", Fields: map[string]string{"DUCKDNS_TOKEN": "x"}}); err != nil {
		t.Fatal(err)
	}
	token := s.sessions.create(false)
	post := func(next string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"next": next})
		r := httptest.NewRequest("POST", "/api/session/handoff", strings.NewReader(string(body)))
		r.AddCookie(&http.Cookie{Name: cookieName, Value: token})
		r.Header.Set("X-SuplaOS", "1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	// Names served by the panel only: not the Cloud, Scripts or others.
	for _, next := range []string{
		"https://supla.example.com/", "https://supla-scripts.example.com/", "https://evil.example.org/",
		"http://supla-z2m.example.com/", "https://supla-z2m.example.com:8444/", "https://u@supla-z2m.example.com/",
	} {
		if w := post(next); w.Code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", next, w.Code, w.Body)
		}
	}
	w := post("https://Supla-Z2M.example.com/devices?x=1")
	var res struct{ URL string }
	json.Unmarshal(w.Body.Bytes(), &res)
	u, err := url.Parse(res.URL)
	if w.Code != http.StatusOK || err != nil || u.Host != "supla-z2m.example.com" || u.Path != handoffPath || u.Query().Get("next") != "/devices?x=1" {
		t.Fatalf("handoff: %d %s", w.Code, w.Body)
	}

	// The code gives the cookie once, then goes on with a page.
	take := func(query string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		s.takeHandoff(w, httptest.NewRequest("GET", handoffPath+"?"+query, nil))
		return w
	}
	w = take(u.RawQuery)
	cookie := w.Result().Cookies()
	if w.Code != http.StatusOK || len(cookie) != 1 || cookie[0].Value != token || !cookie[0].Secure ||
		!strings.Contains(w.Body.String(), `url=/devices?x=1`) {
		t.Errorf("take: %d %v %s", w.Code, cookie, w.Body)
	}
	if w := take(u.RawQuery); w.Code != http.StatusForbidden || len(w.Result().Cookies()) != 0 {
		t.Errorf("taken twice: %d", w.Code)
	}
	// Only a path of the same host.
	for _, next := range []string{"//evil.example.org/", "/\\evil.example.org", "https://evil.example.org/"} {
		q := url.Values{"code": {s.handoffs.create(token)}, "next": {next}}
		if w := take(q.Encode()); !strings.Contains(w.Body.String(), `url=/"`) {
			t.Errorf("%s: %s", next, w.Body)
		}
	}
	// Too late.
	code := s.handoffs.create(token)
	s.handoffs.codes[code] = handoff{token: token, expires: time.Now().Add(-time.Second)}
	if w := take("code=" + code); w.Code != http.StatusForbidden {
		t.Errorf("expired: %d", w.Code)
	}

	// The proxied add-on at its name sends to the login at the name of the
	// panel, on the LAN port to the same host.
	ps := &proxyServer{addon: "zigbee2mqtt", upstream: upstream{service: "zigbee2mqtt", port: "8080"}}
	for listen, c := range map[string][2]string{
		"":     {"https://supla-z2m.example.com/devices", "https://suplaos.example.com/login?next=" + url.QueryEscape("https://supla-z2m.example.com/devices")},
		"8080": {"https://supla-z2m.example.com:8080/devices", "https://supla-z2m.example.com:8443/login?next=" + url.QueryEscape("https://supla-z2m.example.com:8080/devices")},
	} {
		w := httptest.NewRecorder()
		s.proxyHandler(ps, listen).ServeHTTP(w, httptest.NewRequest("GET", c[0], nil))
		if w.Header().Get("Location") != c[1] {
			t.Errorf("%q: %s", listen, w.Header().Get("Location"))
		}
	}
}
