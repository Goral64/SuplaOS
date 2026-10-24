// Package domaincert reads the certificate of the domain of the user got by
// package acme. It does not need lego, so the suplaos command uses it too.
package domaincert

import (
	"crypto/tls"
	"errors"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Dir holds the settings, the ACME account and the certificate.
func Dir(root string) string { return filepath.Join(root, "ca", "acme") }

// Cert is the certificate of the domain: the chain and the key as PEM.
type Cert struct {
	Domain string
	Chain  []byte
	Key    []byte
	Pair   tls.Certificate
}

// Domain returns the domain set in the panel, "" when there is none.
func Domain(root string) string {
	d, _ := domain(root)
	return d
}

func domain(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(Dir(root), "settings.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var s struct {
		Domain string `yaml:"domain"`
	}
	err = yaml.Unmarshal(data, &s)
	return s.Domain, err
}

// Load returns the certificate when a domain is set and the certificate
// covers it and has not expired; nil otherwise.
func Load(root string, now time.Time) (*Cert, error) {
	dir := Dir(root)
	name, err := domain(root)
	if err != nil || name == "" {
		return nil, err
	}
	chain, err := os.ReadFile(filepath.Join(dir, "cert.crt"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	key, err := os.ReadFile(filepath.Join(dir, "cert.key"))
	if err != nil {
		return nil, err
	}
	// Also fails for a pair read while a new one was being written.
	pair, err := tls.X509KeyPair(chain, key)
	if err != nil {
		return nil, err
	}
	if pair.Leaf.VerifyHostname(name) != nil || now.After(pair.Leaf.NotAfter) {
		return nil, nil
	}
	return &Cert{Domain: name, Chain: chain, Key: key, Pair: pair}, nil
}
