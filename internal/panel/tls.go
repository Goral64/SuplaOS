package panel

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goral64/SuplaOS/internal/domaincert"
)

// Port is the HTTPS port of the panel; 443 belongs to the Supla Cloud.
const Port = "8443"

// The certificates are checked this often: the address of the device may
// change at any time (DHCP).
const certInterval = time.Minute

// certDir holds the certificate of the panel, signed by the CA of the
// device like those of the add-ons.
func (s *Server) certDir() string { return filepath.Join(s.cfg.Root, "ca", "panel") }

func (s *Server) caFile() string { return filepath.Join(s.cfg.Root, "ca", "ca.crt") }

// renewCertificates renews the certificate of the panel and, with addons,
// those of the add-ons when needed. Fails while another command holds the
// lock.
func (s *Server) renewCertificates(addons bool) error {
	st, err := s.openStack()
	if err != nil {
		return err
	}
	if err := st.Lock(); err != nil {
		return err
	}
	defer st.Unlock()
	replaced, err := st.DeviceCertificate(s.certDir())
	if err != nil {
		return err
	}
	if replaced || s.cert.Load() == nil {
		if err := s.loadCertificate(); err != nil {
			return err
		}
	}
	s.loadDomainCertificate()
	if !addons {
		return nil
	}
	return st.RenewCertificates()
}

// loadDomainCertificate serves the certificate of the domain, when there
// is one, besides the certificate of the device.
func (s *Server) loadDomainCertificate() {
	c, err := domaincert.Load(s.cfg.Root, time.Now())
	if err != nil {
		log.Printf("certificate of the domain: %v", err)
	}
	if c == nil {
		s.domainCert.Store(nil)
		return
	}
	if old := s.domainCert.Load(); old == nil || !bytes.Equal(old.Certificate[0], c.Pair.Certificate[0]) {
		s.domainCert.Store(&c.Pair)
	}
}

// renewDomain gets or renews the certificate of the domain when it is time
// to. That may take minutes: it does not hold the lock of the stack, the
// add-ons get the new certificate on the next renewCertificates.
func (s *Server) renewDomain() {
	if _, err := s.acme.Renew(log.Writer()); err != nil {
		log.Printf("certificate of the domain: %v", err)
	}
}

func (s *Server) loadCertificate() error {
	dir := s.certDir()
	pair, err := tls.LoadX509KeyPair(filepath.Join(dir, "server.crt"), filepath.Join(dir, "server.key"))
	if err != nil {
		return err
	}
	s.cert.Store(&pair)
	return nil
}

// TLSConfig makes the certificate of the panel if needed and returns the
// configuration serving it, replaced when it is renewed.
func (s *Server) TLSConfig() (*tls.Config, error) {
	// The lock may be held for a while by the suplaos command; an existing
	// certificate does until then.
	for try := 0; ; try++ {
		err := s.renewCertificates(false)
		if err == nil || s.loadCertificate() == nil {
			break
		}
		if try == 30 {
			return nil, fmt.Errorf("certificate: %w", err)
		}
		time.Sleep(time.Second)
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			if d := s.domainCert.Load(); d != nil && hello.ServerName != "" && d.Leaf.VerifyHostname(hello.ServerName) == nil {
				return d, nil
			}
			return s.cert.Load(), nil
		},
	}, nil
}

// WatchCertificates renews the certificates when needed; it does not
// return.
func (s *Server) WatchCertificates() {
	for {
		time.Sleep(certInterval)
		s.renewDomain()
		if err := s.renewCertificates(true); err != nil && !strings.Contains(err.Error(), "another suplaos command") {
			log.Printf("certificates: %v", err)
		}
	}
}

// Redirect sends plain HTTP requests to the panel over HTTPS, without any
// content.
func Redirect() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
		if host == "" || strings.ContainsAny(host, "/@\\") {
			host, _ = os.Hostname()
			host += ".local"
		}
		w.Header().Set("Location", "https://"+net.JoinHostPort(host, Port)+r.URL.RequestURI())
		w.Header().Set("Connection", "close")
		w.WriteHeader(http.StatusMovedPermanently)
	})
}

// getCA serves the CA certificate of the device, to be installed by
// users; it is public.
func (s *Server) getCA(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(s.caFile())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	host, _ := os.Hostname()
	w.Header().Set("Content-Type", "application/x-x509-ca-cert")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="suplaos-%s-ca.crt"`, host))
	w.Write(data)
}

// getCAInfo tells the fingerprint of the CA certificate, to compare with
// the one shown by the system when installing it.
func (s *Server) getCAInfo(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(s.caFile())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	block, _ := pem.Decode(data)
	if block == nil {
		jsonError(w, http.StatusInternalServerError, errors.New("ca.crt: no certificate"))
		return
	}
	crt, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	sum := sha256.Sum256(crt.Raw)
	hex := make([]string, len(sum))
	for i, b := range sum {
		hex[i] = fmt.Sprintf("%02X", b)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name": crt.Subject.CommonName, "sha256": strings.Join(hex, ":"), "notAfter": crt.NotAfter,
	})
}
