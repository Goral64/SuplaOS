package panel

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"
)

func TestServerAddress(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"suplaos-rpi.local", "localhost", "supla-server.local"},
		IPAddresses:  []net.IP{net.ParseIP("172.16.144.2"), net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	for host, want := range map[string]string{
		"172.16.144.2":         "172.16.144.2",
		"suplaos-rpi.local":    "suplaos-rpi.local",
		"supla-rpi.goral64.pl": "172.16.144.2",
		"192.168.1.9":          "172.16.144.2",
	} {
		got, covered := serverAddress(data, host)
		if got != want || covered != (host == want) {
			t.Errorf("%s: %s %v, want %s", host, got, covered, want)
		}
	}
}
