package stack

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// certificates creates the self-signed certificates the installed add-ons
// ask for, unless they exist already.
func (s *Stack) certificates() error {
	for _, a := range s.Order() {
		for _, d := range a.Certificates {
			dir := filepath.Join(s.Dir(), d)
			if _, err := os.Stat(filepath.Join(dir, "server.crt")); err == nil {
				continue
			}
			if err := s.selfSigned(dir); err != nil {
				return err
			}
		}
	}
	return nil
}

// selfSigned writes server.crt and server.key for the device: its name,
// name.local and IP address, valid for 10 years.
func (s *Stack) selfSigned(dir string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	host, _ := os.Hostname()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: host, Organization: []string{"SuplaOS"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{host, host + ".local", "localhost"},
	}
	if ip := net.ParseIP(PrimaryIP()); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return err
	}
	if err := s.writeFile(filepath.Join(dir, "server.key"),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return err
	}
	return s.writeFile(filepath.Join(dir, "server.crt"),
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644)
}
