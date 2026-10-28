package panel

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Goral64/SuplaOS/internal/acme"
	"github.com/Goral64/SuplaOS/internal/stack"
)

func routerStack(t *testing.T, s *Server) *stack.Stack {
	t.Helper()
	st, err := s.openStack()
	if err != nil {
		t.Fatal(err)
	}
	st.State.Addons = map[string]*stack.Installed{"supla": {}, "zigbee2mqtt": {}, "supla-scripts": {}}
	return st
}

func TestRouteTable(t *testing.T) {
	s := newServer(t)
	st := routerStack(t, s)
	port, def, names := routeTable(st, nil)
	if port != "443" || def != (target{tls: "supla-cloud:443"}) || len(names) != 0 {
		t.Errorf("without a domain: %s %v %v", port, def, names)
	}
	set := &acme.Settings{Domain: "supla.example.com", Names: map[string]string{"zigbee2mqtt": "zigbee.example.com"}}
	_, _, names = routeTable(st, set)
	want := map[string]target{
		"suplaos.example.com":       {local: "panel"},
		"zigbee.example.com":        {local: "zigbee2mqtt"},
		"supla-scripts.example.com": {tls: "supla-scripts:443"},
	}
	if len(names) != len(want) {
		t.Errorf("names %v", names)
	}
	for n, w := range want {
		if names[n] != w {
			t.Errorf("%s: %v, want %v", n, names[n], w)
		}
	}
	// No Supla Cloud, no router.
	delete(st.State.Addons, "supla")
	if port, _, _ := routeTable(st, set); port != "" {
		t.Errorf("router without Supla Cloud on %s", port)
	}
}

func TestRouter(t *testing.T) {
	s := newServer(t)
	cfg, err := s.TLSConfig()
	if err != nil {
		t.Fatal(err)
	}
	s.proxies.tls = cfg
	s.proxies.panel = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "panel") })
	backend := func(text string) *httptest.Server {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, text) }))
		t.Cleanup(srv.Close)
		return srv
	}
	cloud, scripts := backend("cloud"), backend("scripts")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	rt := &router{s: s, port: "443", ln: ln, ups: map[string]*upstream{
		"supla-cloud:443":   {addr: cloud.Listener.Addr().String()},
		"supla-scripts:443": {addr: scripts.Listener.Addr().String()},
	}, locals: map[string]*localServer{}}
	defer rt.close()
	go rt.serve()
	rt.update(target{tls: "supla-cloud:443"}, map[string]target{
		"suplaos.example.com":       {local: "panel"},
		"supla-z2m.example.com":     {local: "zigbee2mqtt"},
		"supla-scripts.example.com": {tls: "supla-scripts:443"},
	})

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "tcp", ln.Addr().String())
			},
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	get := func(u string) (*http.Response, string) {
		t.Helper()
		resp, err := client.Get(u)
		if err != nil {
			t.Fatalf("%s: %v", u, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp, string(body)
	}
	for u, want := range map[string]string{
		"https://suplaos.example.com/":       "panel",
		"https://SupLaos.Example.com/":       "panel",
		"https://supla-scripts.example.com/": "scripts",
		"https://supla.example.com/":         "cloud",
		"https://other.example.com/":         "cloud",
		"https://127.0.0.1/":                 "cloud", // no name
	} {
		resp, body := get(u)
		if body != want {
			t.Errorf("%s: %q, want %q", u, body, want)
		}
		// Passed on as it is: the certificate is the one of the service.
		if want == "cloud" && !bytes.Equal(resp.TLS.PeerCertificates[0].Raw, cloud.Certificate().Raw) {
			t.Errorf("%s: not the certificate of the Cloud", u)
		}
	}
	// A proxied add-on without a session: to the login of the panel and
	// back to its name without a port.
	resp, _ := get("https://supla-z2m.example.com/devices")
	want := "https://supla-z2m.example.com:8443/login?next=" + url.QueryEscape("https://supla-z2m.example.com/devices")
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != want {
		t.Errorf("z2m: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}

	// No HTTP/2, and a request for another name on the connection of the
	// panel is refused.
	conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{ServerName: "suplaos.example.com", NextProtos: []string{"h2", "http/1.1"}, InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	if proto := conn.ConnectionState().NegotiatedProtocol; proto != "http/1.1" {
		t.Errorf("protocol %q", proto)
	}
	io.WriteString(conn, "GET / HTTP/1.1\r\nHost: supla.example.com\r\nConnection: close\r\n\r\n")
	if reply, _ := io.ReadAll(conn); !bytes.HasPrefix(reply, []byte("HTTP/1.1 421")) {
		t.Errorf("other name: %.40q", reply)
	}
	conn.Close()

	// A name gone: its server is closed, the name goes to the Cloud.
	rt.update(target{tls: "supla-cloud:443"}, map[string]target{"suplaos.example.com": {local: "panel"}})
	if len(rt.locals) != 1 {
		t.Errorf("servers %v", rt.locals)
	}
	if _, body := get("https://supla-z2m.example.com/"); body != "cloud" {
		t.Errorf("removed name: %q", body)
	}

	// Not TLS: closed.
	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(c, "GET / HTTP/1.0\r\n\r\n")
	if n, _ := c.Read(make([]byte, 10)); n != 0 {
		t.Errorf("plain HTTP answered")
	}
	c.Close()
}
