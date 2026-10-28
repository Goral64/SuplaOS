package panel

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Goral64/SuplaOS/internal/acme"
	"github.com/Goral64/SuplaOS/internal/stack"
)

// The port of Supla Cloud is served by the panel, so that one port
// forwarded from the internet reaches all the web interfaces, each at a
// name of its own. The router reads the name the browser asks for (SNI)
// and passes the connection on: to services with TLS of their own (Supla
// Cloud, SUPLA Scripts) as it is, the panel and the proxied add-ons are
// served here. No name, an IP address or any other name goes to Supla
// Cloud, as before.

const helloTimeout = 10 * time.Second

type router struct {
	s      *Server
	port   string
	ln     net.Listener
	routes atomic.Pointer[routes]

	// Kept between updates: the addresses of the containers and the
	// servers of the panel.
	ups    map[string]*upstream     // by service:port
	locals map[string]*localServer // by "panel" or the add-on

	mu     sync.Mutex
	failed string // last error, logged once
}

// routes are by name; def takes the rest.
type routes struct {
	names map[string]*route
	def   *route
}

// route passes connections to a service, or to a server of the panel.
type route struct {
	up    *upstream
	local *localServer
}

// target is where a name goes: a server of the panel ("panel" or an
// add-on with a proxy) or the service:port of a service with TLS.
type target struct{ local, tls string }

// routeTable returns the port of the router, the target of the rest and
// the targets by name; no port when there is no Supla Cloud.
func routeTable(st *stack.Stack, set *acme.Settings) (port string, def target, names map[string]target) {
	for _, a := range st.Order() {
		if a.Web != nil && a.Web.TLS != "" && a.Web.Host == "" {
			// The port is an option of the add-on.
			values := st.Values(a)
			port, def = os.Expand(a.Web.Port, func(k string) string { return values[k] }), target{tls: a.Web.TLS}
			break
		}
	}
	names = map[string]target{}
	if port == "" || set == nil {
		return port, def, names
	}
	for _, i := range interfacesOf(st) {
		a := st.Catalog[i.ID]
		switch {
		case i.ID == "panel" || a.Web.Proxy != "":
			names[set.Name(i)] = target{local: i.ID}
		case a.Web.TLS != "":
			names[set.Name(i)] = target{tls: a.Web.TLS}
		}
	}
	return port, def, names
}

// updateRouter starts, changes or stops the router; called with
// s.proxies.mu held.
func (s *Server) updateRouter(st *stack.Stack) {
	p := &s.proxies
	set, err := s.acme.Settings()
	if err != nil {
		log.Printf("router: %v", err)
		return
	}
	port, def, names := routeTable(st, set)
	if p.router != nil && p.router.port != port {
		p.router.close()
		p.router = nil
	}
	if port == "" {
		return
	}
	if p.router == nil {
		// Until the Cloud has been recreated without its port, after an
		// update of SuplaOS; tried again later.
		ln, err := p.listen(port, "router")
		if err != nil {
			return
		}
		p.router = &router{s: s, port: port, ln: ln, ups: map[string]*upstream{}, locals: map[string]*localServer{}}
		go p.router.serve()
		log.Printf("router on %s", port)
	}
	p.router.update(def, names)
}

func (rt *router) update(def target, names map[string]target) {
	p := &rt.s.proxies
	routeTo := func(t target) *route {
		if t.local == "" {
			if rt.ups[t.tls] == nil {
				svc, port, _ := strings.Cut(t.tls, ":")
				rt.ups[t.tls] = &upstream{service: svc, port: port}
			}
			return &route{up: rt.ups[t.tls]}
		}
		if rt.locals[t.local] == nil {
			h := p.panel
			if t.local != "panel" {
				h = rt.s.addonProxy(t.local)
			}
			rt.locals[t.local] = newLocalServer(rt.ln.Addr(), h, p.tls)
		}
		return &route{local: rt.locals[t.local]}
	}
	rs := &routes{names: map[string]*route{}, def: routeTo(def)}
	used := map[string]bool{}
	for name, t := range names {
		rs.names[name] = routeTo(t)
		used[t.local] = true
	}
	rt.routes.Store(rs)
	for id, l := range rt.locals {
		if !used[id] {
			l.close()
			delete(rt.locals, id)
		}
	}
}

// addonProxy serves the web interface of the add-on through the router.
func (s *Server) addonProxy(id string) http.Handler {
	st, err := s.openStack()
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, err.Error(), http.StatusBadGateway)
		})
	}
	svc, port, _ := strings.Cut(st.Catalog[id].Web.Proxy, ":")
	return s.proxyHandler(&proxyServer{addon: id, upstream: upstream{service: svc, port: port}}, "")
}

func (rt *router) close() {
	rt.ln.Close()
	for _, l := range rt.locals {
		l.close()
	}
}

func (rt *router) serve() {
	for {
		c, err := rt.ln.Accept()
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}
		go rt.handle(c)
	}
}

func (rt *router) handle(c net.Conn) {
	c.SetReadDeadline(time.Now().Add(helloTimeout))
	name, hello, err := serverName(c)
	if err != nil {
		c.Close()
		return
	}
	c.SetReadDeadline(time.Time{})
	pc := &peekedConn{Conn: c, r: io.MultiReader(bytes.NewReader(hello), c)}
	rs := rt.routes.Load()
	r := rs.names[strings.TrimSuffix(strings.ToLower(name), ".")]
	if r == nil {
		r = rs.def
	}
	if r.local != nil {
		r.local.ln.hand(pc)
		return
	}
	rt.pass(pc, r.up)
}

// pass sends the connection on to the service as it is.
func (rt *router) pass(c net.Conn, up *upstream) {
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), helloTimeout)
	defer cancel()
	b, err := rt.dial(ctx, up)
	if err != nil {
		// The container may have been recreated with another address.
		up.forget()
		b, err = rt.dial(ctx, up)
	}
	if err != nil {
		up.forget()
		rt.logError(err)
		return
	}
	defer b.Close()
	rt.logError(nil)
	done := make(chan struct{})
	go func() {
		io.Copy(b, c)
		closeWrite(b)
		close(done)
	}()
	io.Copy(c, b)
	closeWrite(c)
	<-done
}

func (rt *router) dial(ctx context.Context, up *upstream) (net.Conn, error) {
	addr, err := rt.s.address(ctx, up)
	if err != nil {
		return nil, err
	}
	return (&net.Dialer{}).DialContext(ctx, "tcp", addr)
}

func (rt *router) logError(err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if msg != "" && msg != rt.failed {
		log.Printf("router: %v", err)
	}
	rt.failed = msg
}

func closeWrite(c net.Conn) {
	if pc, ok := c.(*peekedConn); ok {
		c = pc.Conn
	}
	if tc, ok := c.(interface{ CloseWrite() error }); ok {
		tc.CloseWrite()
	}
}

var errHello = errors.New("hello read")

// serverName reads the ClientHello from c and returns the name asked for,
// "" without one, and all the bytes read, to be sent on.
func serverName(c net.Conn) (string, []byte, error) {
	var buf bytes.Buffer
	var name string
	got := false
	err := tls.Server(helloConn{Conn: c, r: io.TeeReader(c, &buf)}, &tls.Config{
		GetConfigForClient: func(h *tls.ClientHelloInfo) (*tls.Config, error) {
			name, got = h.ServerName, true
			return nil, errHello
		},
	}).Handshake()
	if !got {
		return "", nil, err
	}
	return name, buf.Bytes(), nil
}

// helloConn only reads: the handshake ends before anything is sent.
type helloConn struct {
	net.Conn
	r io.Reader
}

func (c helloConn) Read(p []byte) (int, error)  { return c.r.Read(p) }
func (c helloConn) Write(p []byte) (int, error) { return 0, io.ErrClosedPipe }

// peekedConn gives the bytes read for the name first.
type peekedConn struct {
	net.Conn
	r io.Reader
}

func (c *peekedConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// localServer serves connections of the router in the panel.
type localServer struct {
	ln  *connListener
	srv *http.Server
}

func newLocalServer(addr net.Addr, h http.Handler, cfg *tls.Config) *localServer {
	l := &localServer{
		ln:  &connListener{addr: addr, conns: make(chan net.Conn), done: make(chan struct{})},
		srv: &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second},
	}
	go l.srv.Serve(tls.NewListener(l.ln, cfg))
	return l
}

func (l *localServer) close() { l.srv.Close() }

// connListener hands the connections of the router to an http.Server.
type connListener struct {
	addr  net.Addr
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

func (l *connListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *connListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *connListener) Addr() net.Addr { return l.addr }

func (l *connListener) hand(c net.Conn) {
	select {
	case l.conns <- c:
	case <-l.done:
		c.Close()
	}
}
