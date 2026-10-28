package panel

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Web interfaces of add-ons with web.proxy are served by the panel on
// their own port, over HTTPS and to logged-in users only. The session
// cookie is the one of the panel: cookies do not depend on the port.

const proxyInterval = 10 * time.Second

type proxies struct {
	mu      sync.Mutex
	tls     *tls.Config
	panel   http.Handler // served by the router at the name of the panel
	servers map[string]*proxyServer // by port
	router  *router
	failed  map[string]string // last error by port, logged once
}

type proxyServer struct {
	addon string
	upstream
	srv *http.Server
}

// upstream is a port of a service in its container.
type upstream struct {
	service, port string

	mu   sync.Mutex
	addr string // of the container, looked up again after an error
}

// WatchProxies keeps the proxies and the router in line with the installed
// add-ons; it does not return. panel is served by the router at the name
// of the panel.
func (s *Server) WatchProxies(cfg *tls.Config, panel http.Handler) {
	s.proxies.mu.Lock()
	s.proxies.tls, s.proxies.panel = cfg, panel
	s.proxies.mu.Unlock()
	for {
		s.updateProxies()
		time.Sleep(proxyInterval)
	}
}

// updateProxies starts and stops the proxies after add-ons were
// installed, removed or got another port.
func (s *Server) updateProxies() {
	p := &s.proxies
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tls == nil {
		return
	}
	st, err := s.openStack()
	if err != nil {
		log.Printf("proxies: %v", err)
		return
	}
	want := map[string]*proxyServer{}
	for _, a := range st.Order() {
		if a.Web == nil || a.Web.Proxy == "" {
			continue
		}
		svc, port, _ := strings.Cut(a.Web.Proxy, ":")
		if listen := st.Expand(a.Web.Port); listen != "" {
			want[listen] = &proxyServer{addon: a.ID, upstream: upstream{service: svc, port: port}}
		}
	}
	for listen, ps := range p.servers {
		if w := want[listen]; w == nil || w.addon != ps.addon || w.service != ps.service || w.port != ps.port {
			ps.srv.Close()
			delete(p.servers, listen)
		}
	}
	if p.servers == nil {
		p.servers = map[string]*proxyServer{}
	}
	for listen, ps := range want {
		if p.servers[listen] != nil {
			continue
		}
		// The port may still be taken, e.g. by the container of an older
		// version publishing it; tried again later.
		ln, err := p.listen(listen, "proxy "+ps.addon)
		if err != nil {
			continue
		}
		ps.srv = &http.Server{Handler: s.proxyHandler(ps, listen), ReadHeaderTimeout: 10 * time.Second}
		go ps.srv.Serve(tls.NewListener(ln, p.tls))
		p.servers[listen] = ps
		log.Printf("proxy %s on %s", ps.addon, listen)
	}
	s.updateRouter(st)
}

// listen listens on port; an error is logged once until it changes.
func (p *proxies) listen(port, what string) (net.Listener, error) {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		if p.failed[port] != err.Error() {
			log.Printf("%s: %v", what, err)
		}
		if p.failed == nil {
			p.failed = map[string]string{}
		}
		p.failed[port] = err.Error()
		return nil, err
	}
	delete(p.failed, port)
	return ln, nil
}

// address returns the address of the container, looked up once.
func (s *Server) address(ctx context.Context, u *upstream) (string, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.addr != "" {
		return u.addr, nil
	}
	st, err := s.openStack()
	if err != nil {
		return "", err
	}
	ip, err := st.ServiceAddress(ctx, u.service)
	if err != nil {
		return "", err
	}
	u.addr = net.JoinHostPort(ip, u.port)
	return u.addr, nil
}

// forget makes the address looked up again: the container may have been
// recreated with another one.
func (u *upstream) forget() {
	u.mu.Lock()
	u.addr = ""
	u.mu.Unlock()
}

// proxyHandler serves ps on listen, or through the router at a name of its
// own when listen is empty.
func (s *Server) proxyHandler(ps *proxyServer, listen string) http.Handler {
	return s.guard(ps.addon, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == handoffPath {
			s.takeHandoff(w, r)
			return
		}
		if ses := s.sessions.get(r); ses == nil || ses.MustChange {
			if r.Method != http.MethodGet || r.Header.Get("Upgrade") != "" {
				http.Error(w, "Log in to the SuplaOS panel first.", http.StatusUnauthorized)
				return
			}
			back := "https://" + r.Host + r.URL.RequestURI()
			if listen != "" {
				host := r.Host
				if h, _, err := net.SplitHostPort(host); err == nil {
					host = h
				}
				back = "https://" + net.JoinHostPort(host, listen) + r.URL.RequestURI()
			}
			http.Redirect(w, r, s.panelLogin(r.Host)+"?next="+url.QueryEscape(back), http.StatusFound)
			return
		}
		addr, err := s.address(r.Context(), &ps.upstream)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		rp := &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.SetURL(&url.URL{Scheme: "http", Host: addr})
				pr.SetXForwarded()
				dropCookie(pr.Out.Header, cookieName)
			},
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				ps.forget()
				if !errors.Is(err, context.Canceled) {
					log.Printf("proxy %s: %v", ps.addon, err)
				}
				http.Error(w, ps.addon+" does not answer, is it running?", http.StatusBadGateway)
			},
		}
		rp.ServeHTTP(w, r)
	}))
}

// dropCookie removes the cookie from the request headers: the session of
// the panel is not for the add-on.
func dropCookie(h http.Header, name string) {
	var keep []string
	for _, line := range h.Values("Cookie") {
		for _, part := range strings.Split(line, ";") {
			if k, _, _ := strings.Cut(strings.TrimSpace(part), "="); k != name && strings.TrimSpace(part) != "" {
				keep = append(keep, strings.TrimSpace(part))
			}
		}
	}
	h.Del("Cookie")
	if len(keep) > 0 {
		h.Set("Cookie", strings.Join(keep, "; "))
	}
}
