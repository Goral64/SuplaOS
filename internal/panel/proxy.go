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
	servers map[string]*proxyServer // by port
	failed  map[string]string       // last error by port, logged once
}

type proxyServer struct {
	addon, service, port string // port of the service in its container
	srv                  *http.Server

	mu   sync.Mutex
	addr string // of the container, looked up again after an error
}

// WatchProxies keeps the proxies in line with the installed add-ons; it
// does not return.
func (s *Server) WatchProxies(cfg *tls.Config) {
	s.proxies.mu.Lock()
	s.proxies.tls = cfg
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
			want[listen] = &proxyServer{addon: a.ID, service: svc, port: port}
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
		ln, err := net.Listen("tcp", ":"+listen)
		if err != nil {
			if p.failed[listen] != err.Error() {
				log.Printf("proxy %s: %v", ps.addon, err)
			}
			if p.failed == nil {
				p.failed = map[string]string{}
			}
			p.failed[listen] = err.Error()
			continue
		}
		delete(p.failed, listen)
		ps.srv = &http.Server{Handler: s.proxyHandler(ps, listen), ReadHeaderTimeout: 10 * time.Second}
		go ps.srv.Serve(tls.NewListener(ln, p.tls))
		p.servers[listen] = ps
		log.Printf("proxy %s on %s", ps.addon, listen)
	}
}

// proxyAddress returns the address of the container, looked up once.
func (s *Server) proxyAddress(ctx context.Context, ps *proxyServer) (string, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.addr != "" {
		return ps.addr, nil
	}
	st, err := s.openStack()
	if err != nil {
		return "", err
	}
	ip, err := st.ServiceAddress(ctx, ps.service)
	if err != nil {
		return "", err
	}
	ps.addr = net.JoinHostPort(ip, ps.port)
	return ps.addr, nil
}

func (s *Server) proxyHandler(ps *proxyServer, listen string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ses := s.sessions.get(r); ses == nil || ses.MustChange {
			if r.Method != http.MethodGet || r.Header.Get("Upgrade") != "" {
				http.Error(w, "Log in to the SuplaOS panel first.", http.StatusUnauthorized)
				return
			}
			host := r.Host
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			back := "https://" + net.JoinHostPort(host, listen) + r.URL.RequestURI()
			http.Redirect(w, r, "https://"+net.JoinHostPort(host, Port)+"/login?next="+url.QueryEscape(back), http.StatusFound)
			return
		}
		addr, err := s.proxyAddress(r.Context(), ps)
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
				// The container may have been recreated with another
				// address.
				ps.mu.Lock()
				ps.addr = ""
				ps.mu.Unlock()
				if !errors.Is(err, context.Canceled) {
					log.Printf("proxy %s: %v", ps.addon, err)
				}
				http.Error(w, ps.addon+" does not answer, is it running?", http.StatusBadGateway)
			},
		}
		rp.ServeHTTP(w, r)
	})
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
