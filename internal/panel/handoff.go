package panel

import (
	"errors"
	"html"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Goral64/SuplaOS/internal/acme"
)

// Cookies are kept by host name. The panel and the proxied add-ons served
// at names of their own by the router get the session of the panel through
// a code good for one use: the panel gives it to a logged-in browser, which
// takes it to the other name.

const (
	handoffPath    = "/_suplaos/session"
	handoffTimeout = time.Minute
)

type handoffs struct {
	mu    sync.Mutex
	codes map[string]handoff
}

type handoff struct {
	token   string
	expires time.Time
}

func (h *handoffs) create(token string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	for c, ho := range h.codes {
		if now.After(ho.expires) {
			delete(h.codes, c)
		}
	}
	if h.codes == nil {
		h.codes = map[string]handoff{}
	}
	code := newToken()
	h.codes[code] = handoff{token: token, expires: now.Add(handoffTimeout)}
	return code
}

// take returns the token of the code, once.
func (h *handoffs) take(code string) string {
	h.mu.Lock()
	defer h.mu.Unlock()
	ho, ok := h.codes[code]
	delete(h.codes, code)
	if !ok || time.Now().After(ho.expires) {
		return ""
	}
	return ho.token
}

// handoffTargets are the names served by the panel through the router,
// with the port when it is not 443.
func (s *Server) handoffTargets() (map[string]bool, error) {
	st, err := s.openStack()
	if err != nil {
		return nil, err
	}
	set, err := s.acme.Settings()
	if err != nil {
		return nil, err
	}
	port, _, names := routeTable(st, set)
	out := map[string]bool{}
	for name, t := range names {
		if t.local == "" {
			continue
		}
		if port != "443" {
			name = net.JoinHostPort(name, port)
		}
		out[name] = true
	}
	return out, nil
}

// panelLogin is the login page of the panel for a request at host: at the
// name of the panel when the host is one of the names of the router, at
// the same host and the port of the panel otherwise.
func (s *Server) panelLogin(host string) string {
	if targets, err := s.handoffTargets(); err == nil && targets[host] {
		if set, _ := s.acme.Settings(); set != nil {
			name := set.Name(panelInterface)
			if h, port, err := net.SplitHostPort(host); err == nil && h != "" {
				name = net.JoinHostPort(name, port)
			}
			return "https://" + name + "/login"
		}
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return "https://" + net.JoinHostPort(host, Port) + "/login"
}

// postHandoff gives a code for the session of the request, to be taken to
// another name served by the panel.
func (s *Server) postHandoff(w http.ResponseWriter, r *http.Request) {
	var req struct{ Next string }
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	c, err := r.Cookie(cookieName)
	if err != nil {
		jsonError(w, http.StatusUnauthorized, errors.New("not logged in"))
		return
	}
	u, err := url.Parse(req.Next)
	if err != nil || u.Scheme != "https" || u.User != nil {
		jsonError(w, http.StatusBadRequest, errors.New("not an address of SuplaOS"))
		return
	}
	targets, err := s.handoffTargets()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	host := strings.ToLower(u.Host)
	if !targets[host] {
		jsonError(w, http.StatusBadRequest, errors.New("not an address of SuplaOS"))
		return
	}
	q := url.Values{"code": {s.handoffs.create(c.Value)}, "next": {u.RequestURI()}}
	writeJSON(w, http.StatusOK, map[string]string{"url": "https://" + host + handoffPath + "?" + q.Encode()})
}

// takeHandoff sets the cookie of the session given by the code and goes on
// to next. A page does that, not a redirect: a SameSite=Strict cookie is
// not sent on a redirect after a navigation from another site.
func (s *Server) takeHandoff(w http.ResponseWriter, r *http.Request) {
	token := s.handoffs.take(r.URL.Query().Get("code"))
	next := r.URL.Query().Get("next")
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		next = "/"
	}
	if token == "" {
		http.Error(w, "The code is not valid anymore, open the address again.", http.StatusForbidden)
		return
	}
	setCookie(w, token, int(sessionTimeout.Seconds()))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	n := html.EscapeString(next)
	w.Write([]byte(`<!doctype html><meta http-equiv="refresh" content="0;url=` + n + `"><a href="` + n + `">Continue</a>`))
}

// panelInterface is the panel among the web interfaces with a name.
var panelInterface = acme.Interface{ID: "panel", Host: "suplaos"}
