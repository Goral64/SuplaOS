package panel

import (
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Goral64/SuplaOS/internal/addon"
	"github.com/Goral64/SuplaOS/internal/stack"
)

// The panel and the add-ons it serves answer requests from the internet
// only when that is turned on for them. Supla Cloud and SUPLA Scripts have
// logins of their own and are always reachable.

// privateNets are addresses that are not on the internet.
var privateNets = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("100.64.0.0/10"), // CGNAT, e.g. Tailscale
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
}

// localNets are the networks of the interfaces of the device: at home also
// public IPv6 addresses. Looked up again now and then.
var localNets struct {
	sync.Mutex
	nets    []netip.Prefix
	updated time.Time
}

// interfaceNets is net.InterfaceAddrs, another one in the tests.
var interfaceNets = func() ([]netip.Prefix, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	var out []netip.Prefix
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			if p, err := netip.ParsePrefix(n.String()); err == nil {
				out = append(out, p.Masked())
			}
		}
	}
	return out, nil
}

// isLocal tells whether ip is in the local network or a private one.
func isLocal(ip netip.Addr) bool {
	ip = ip.Unmap()
	for _, n := range privateNets {
		if n.Contains(ip) {
			return true
		}
	}
	localNets.Lock()
	defer localNets.Unlock()
	if time.Since(localNets.updated) > 30*time.Second {
		if nets, err := interfaceNets(); err == nil {
			localNets.nets, localNets.updated = nets, time.Now()
		}
	}
	for _, n := range localNets.nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func remoteAddr(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip, _ := netip.ParseAddr(host)
	return ip.Unmap()
}

// forwardedFor are the addresses a proxy in the local network (e.g. the
// Cloudflare tunnel or a reverse proxy on the router) says the request came
// from.
func forwardedFor(r *http.Request) []netip.Addr {
	var out []netip.Addr
	add := func(s string) {
		s = strings.Trim(strings.TrimSpace(s), `"`)
		if h, _, err := net.SplitHostPort(s); err == nil {
			s = h
		}
		if ip, err := netip.ParseAddr(strings.Trim(s, "[]")); err == nil {
			out = append(out, ip.Unmap())
		}
	}
	add(r.Header.Get("Cf-Connecting-Ip"))
	add(r.Header.Get("X-Real-Ip"))
	for _, v := range r.Header.Values("X-Forwarded-For") {
		for _, part := range strings.Split(v, ",") {
			add(part)
		}
	}
	for _, v := range r.Header.Values("Forwarded") {
		for _, part := range strings.FieldsFunc(v, func(c rune) bool { return c == ',' || c == ';' }) {
			if k, val, ok := strings.Cut(strings.TrimSpace(part), "="); ok && strings.EqualFold(k, "for") {
				add(val)
			}
		}
	}
	return out
}

// fromInternet tells whether the request came from the internet: from an
// address outside the local network, or through a proxy in it from one.
// Headers are believed only from the local network; there a forged one
// can only take access away.
func fromInternet(r *http.Request) bool {
	if !isLocal(remoteAddr(r)) {
		return true
	}
	return slices.ContainsFunc(forwardedFor(r), func(ip netip.Addr) bool { return !isLocal(ip) })
}

// clientIP is the address logins are counted by: the one a proxy in the
// local network gives, the address of the connection otherwise.
func clientIP(r *http.Request) string {
	ip := remoteAddr(r)
	if isLocal(ip) {
		if fwd := forwardedFor(r); len(fwd) > 0 {
			return fwd[0].String()
		}
	}
	return ip.String()
}

// internetAllowed tells whether id ("panel" or an add-on) answers requests
// from the internet. An add-on needs the panel too: it is logged in to
// there.
func (s *Server) internetAllowed(id string) bool {
	allowed := s.getSettings().Internet
	return slices.Contains(allowed, "panel") && (id == "panel" || slices.Contains(allowed, id))
}

// guard refuses requests from the internet unless id allows them.
func (s *Server) guard(id string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fromInternet(r) && !s.internetAllowed(id) {
			http.Error(w, "Access from the internet is turned off in the SuplaOS panel (System, Access).\n"+
				"Dostęp z internetu jest wyłączony w panelu SuplaOS (System, Dostęp).", http.StatusForbidden)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// internetEntry is a web interface served by the panel on the Access page.
type internetEntry struct {
	ID      string     `json:"id"`
	Label   addon.Text `json:"label"`
	Allowed bool       `json:"allowed"`
}

func guarded(st *stack.Stack) []internetEntry {
	out := []internetEntry{{ID: "panel", Label: panelLabel}}
	for _, a := range st.Order() {
		if a.Web != nil && a.Web.Proxy != "" {
			out = append(out, internetEntry{ID: a.ID, Label: a.Name})
		}
	}
	return out
}

func (s *Server) getInternet(w http.ResponseWriter, r *http.Request) {
	st, err := s.openStack()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	list := guarded(st)
	allowed := s.getSettings().Internet
	for i := range list {
		list[i].Allowed = slices.Contains(allowed, list[i].ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"interfaces": list, "fromInternet": fromInternet(r)})
}

func (s *Server) putInternet(w http.ResponseWriter, r *http.Request) {
	var req struct{ Allowed []string }
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	st, err := s.openStack()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	var allowed []string
	for _, e := range guarded(st) {
		if slices.Contains(req.Allowed, e.ID) {
			allowed = append(allowed, e.ID)
		}
	}
	if err := s.saveSettings(func(set *Settings) { set.Internet = allowed }); err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}
