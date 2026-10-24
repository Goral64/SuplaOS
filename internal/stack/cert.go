package stack

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/Goral64/SuplaOS/internal/addon"
	"github.com/Goral64/SuplaOS/internal/domaincert"
)

const (
	caValidity = 20 * 365 * 24 * time.Hour
	// Apple devices reject server certificates valid for more than 825
	// days, also from a CA added by the user.
	leafValidity = 825 * 24 * time.Hour
	// A certificate is renewed when it expires sooner than this.
	renewBefore = 30 * 24 * time.Hour
)

// The CA can only sign certificates for local names and addresses, so its
// key is of no use for other sites even on computers that trust it.
var (
	caDNSDomains = []string{"local", "localhost"}
	caIPRanges   = []string{
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16", "127.0.0.0/8",
		"100.64.0.0/10", "fc00::/7", "fe80::/10", "::1/128",
	}
)

// deviceNames returns the names and addresses the certificates are made
// for. A variable for the tests.
var deviceNames = func() ([]string, []net.IP) {
	host, _ := os.Hostname()
	dns := []string{"localhost"}
	if host != "" && host != "localhost" {
		dns = append([]string{host + ".local"}, dns...)
	}
	var ips []net.IP
	if ip := net.ParseIP(PrimaryIP()); ip != nil && !ip.IsLoopback() {
		ips = append(ips, ip)
	}
	return dns, append(ips, net.ParseIP("127.0.0.1"))
}

// CADir holds the certificate authority of the device: ca.crt, which
// users install to trust the device, and ca.key.
func (s *Stack) CADir() string { return filepath.Join(s.Root, "ca") }

// CACertFile is the CA certificate users download.
func (s *Stack) CACertFile() string { return filepath.Join(s.CADir(), "ca.crt") }

// certificates makes the certificates the installed add-ons ask for and
// renews them when needed. Services of add-ons whose certificate was
// replaced are restarted after Up.
func (s *Stack) certificates() error {
	for _, a := range s.Order() {
		for _, d := range a.Certificates {
			replaced, err := s.Certificate(filepath.Join(s.Dir(), d.Dir))
			if err != nil {
				return err
			}
			if replaced {
				s.restartFor(a, d)
			}
		}
		for _, d := range a.Trust {
			replaced, err := s.trustBundle(filepath.Join(s.Dir(), d.Dir))
			if err != nil {
				return err
			}
			if replaced {
				s.restartFor(a, d)
			}
		}
	}
	return nil
}

// restartFor queues a restart of the services using the certificates of
// d, after Up.
func (s *Stack) restartFor(a *addon.Addon, d addon.CertDir) {
	services := d.Restart
	if len(services) == 0 {
		services = a.Services
	}
	if len(services) > 0 {
		s.after = append(s.after, addon.After{Restart: slices.Clone(services)})
	}
}

// systemCAs is the CA bundle of the system (ca-certificates).
var systemCAs = "/etc/ssl/certs/ca-certificates.crt"

// trustBundle writes ca-certificates.crt in dir: the CAs of the system and
// the CA of the device after them. replaced tells that an existing bundle
// changed, e.g. after an update of ca-certificates.
func (s *Stack) trustBundle(dir string) (replaced bool, err error) {
	ca, _, err := s.loadCA()
	if err != nil {
		return false, err
	}
	system, err := os.ReadFile(systemCAs)
	if err != nil {
		return false, err
	}
	data := append(append(system, '\n'), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})...)
	file := filepath.Join(dir, "ca-certificates.crt")
	old, err := os.ReadFile(file)
	if err == nil && bytes.Equal(old, data) {
		return false, nil
	}
	if err := s.mkdir(dir); err != nil {
		return false, err
	}
	return err == nil, s.writeFile(file, data, 0o644)
}

// Certificate makes server.crt and server.key of an add-on in dir: the
// Let's Encrypt certificate of the domain set in the panel when there is
// one, else one signed by the CA of the device (DeviceCertificate).
// replaced tells that an existing certificate was replaced.
func (s *Stack) Certificate(dir string) (replaced bool, err error) {
	domain, err := domaincert.Load(s.Root, time.Now())
	if err != nil {
		// The certificate of the device does until then.
		fmt.Fprintf(s.stderr(), "certificate of the domain: %v\n", err)
	}
	if domain == nil {
		return s.DeviceCertificate(dir)
	}
	ca, _, err := s.loadCA()
	if err != nil {
		return false, err
	}
	crtFile, keyFile := filepath.Join(dir, "server.crt"), filepath.Join(dir, "server.key")
	old, err := os.ReadFile(crtFile)
	if bytes.Equal(old, domain.Chain) {
		if key, _ := os.ReadFile(keyFile); bytes.Equal(key, domain.Key) {
			return false, nil
		}
	}
	replaced = err == nil
	if err := s.writeFile(keyFile, domain.Key, 0o600); err != nil {
		return false, err
	}
	// ca.crt stays the CA of the device, e.g. for the clients of
	// Mosquitto set up before.
	if err := s.writeFile(filepath.Join(dir, "ca.crt"),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}), 0o644); err != nil {
		return false, err
	}
	return replaced, s.writeFile(crtFile, domain.Chain, 0o644)
}

// DeviceCertificate makes server.crt (with the CA certificate after it)
// and server.key in dir, signed by the CA of the device, and puts ca.crt
// next to them for the clients. An existing one is kept while it is signed
// by the CA, covers the current names and addresses of the device and does
// not expire soon. replaced tells that an existing certificate was
// replaced.
func (s *Stack) DeviceCertificate(dir string) (replaced bool, err error) {
	ca, caKey, err := s.loadCA()
	if err != nil {
		return false, err
	}
	dns, ips := deviceNames()
	crtFile := filepath.Join(dir, "server.crt")
	if data, err := os.ReadFile(crtFile); err == nil {
		if certValid(data, ca, dns, ips) {
			return false, nil
		}
		replaced = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return false, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serialNumber(),
		Subject:               pkix.Name{CommonName: dns[0], Organization: []string{"SuplaOS"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(leafValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dns,
		IPAddresses:           permittedIPs(ips),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		return false, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return false, err
	}
	if err := s.writeFile(filepath.Join(dir, "server.key"),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return false, err
	}
	chain := append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})...)
	if err := s.writeFile(filepath.Join(dir, "ca.crt"),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw}), 0o644); err != nil {
		return false, err
	}
	return replaced, s.writeFile(crtFile, chain, 0o644)
}

// certValid tells whether the first certificate in data is signed by ca,
// is valid for all the names and addresses and does not expire soon.
func certValid(data []byte, ca *x509.Certificate, dns []string, ips []net.IP) bool {
	block, _ := pem.Decode(data)
	if block == nil {
		return false
	}
	crt, err := x509.ParseCertificate(block.Bytes)
	if err != nil || crt.CheckSignatureFrom(ca) != nil {
		return false
	}
	if time.Until(crt.NotAfter) < renewBefore {
		return false
	}
	for _, name := range dns {
		if !slices.Contains(crt.DNSNames, name) {
			return false
		}
	}
	for _, ip := range permittedIPs(ips) {
		if !slices.ContainsFunc(crt.IPAddresses, ip.Equal) {
			return false
		}
	}
	return true
}

// loadCA reads the CA of the device and makes it on first use.
func (s *Stack) loadCA() (*x509.Certificate, *ecdsa.PrivateKey, error) {
	crtFile, keyFile := s.CACertFile(), filepath.Join(s.CADir(), "ca.key")
	pair, err := tls.LoadX509KeyPair(crtFile, keyFile)
	if err == nil {
		key, ok := pair.PrivateKey.(*ecdsa.PrivateKey)
		if !ok {
			return nil, nil, fmt.Errorf("%s: not an ECDSA key", keyFile)
		}
		crt, err := x509.ParseCertificate(pair.Certificate[0])
		// CAs made before 0.4.2 had the same name on every device, and
		// browsers mix up such CAs: they are made anew, with an ID.
		if err != nil || crt.Subject.SerialNumber != "" {
			return crt, key, err
		}
	} else if _, statErr := os.Stat(crtFile); !errors.Is(statErr, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("CA of the device: %w", err)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	host, _ := os.Hostname()
	// The ID tells apart the CAs of devices with the same name, also of the
	// same device installed again.
	pub, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, nil, err
	}
	sum := sha256.Sum256(pub)
	id := hex.EncodeToString(sum[:4])
	var ranges []*net.IPNet
	for _, r := range caIPRanges {
		_, n, _ := net.ParseCIDR(r)
		ranges = append(ranges, n)
	}
	tmpl := &x509.Certificate{
		SerialNumber: serialNumber(),
		Subject: pkix.Name{
			CommonName:   "SuplaOS " + host + " " + id,
			Organization: []string{"SuplaOS"},
			SerialNumber: id,
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(caValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
		PermittedDNSDomains:   caDNSDomains,
		PermittedIPRanges:     ranges,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	if err := s.writeFile(keyFile,
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return nil, nil, err
	}
	if err := s.writeFile(crtFile,
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return nil, nil, err
	}
	crt, err := x509.ParseCertificate(der)
	return crt, key, err
}

// permittedIPs leaves out addresses the CA cannot sign for, e.g. a public
// address: a certificate with one would be rejected as a whole.
func permittedIPs(ips []net.IP) []net.IP {
	var out []net.IP
	for _, ip := range ips {
		for _, r := range caIPRanges {
			if _, n, _ := net.ParseCIDR(r); n.Contains(ip) {
				out = append(out, ip)
				break
			}
		}
	}
	return out
}

func serialNumber() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		panic(err)
	}
	return n
}

// RenewCertificates renews the certificates of the add-ons when needed,
// e.g. after the address of the device changed, and restarts the add-ons
// that got a new one. A new address or name of the device also changes
// .env (SUPLAOS_IP, SUPLAOS_HOSTNAME): then the stack is applied and the
// containers using them are recreated; not while new add-on files wait,
// those are applied after backups.
func (s *Stack) RenewCertificates() error {
	if pending, err := s.Pending(); err == nil && len(pending) == 0 && len(s.State.Addons) > 0 && s.envChanged() {
		if err := s.Apply(); err != nil {
			return err
		}
		return s.Up()
	}
	if err := s.certificates(); err != nil {
		return err
	}
	return s.RunAfter()
}

// envChanged tells whether .env would be written differently now.
func (s *Stack) envChanged() bool {
	p, err := s.Plan()
	if err != nil {
		return false
	}
	disk, err := os.ReadFile(filepath.Join(s.Dir(), ".env"))
	return err == nil && !bytes.Equal(disk, p.Files[".env"])
}
