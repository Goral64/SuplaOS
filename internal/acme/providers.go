package acme

import (
	"time"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/providers/dns/acmedns"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/providers/dns/cloudns"
	"github.com/go-acme/lego/v4/providers/dns/desec"
	"github.com/go-acme/lego/v4/providers/dns/digitalocean"
	"github.com/go-acme/lego/v4/providers/dns/duckdns"
	"github.com/go-acme/lego/v4/providers/dns/gandiv5"
	"github.com/go-acme/lego/v4/providers/dns/godaddy"
	"github.com/go-acme/lego/v4/providers/dns/hetzner"
	"github.com/go-acme/lego/v4/providers/dns/hostinger"
	"github.com/go-acme/lego/v4/providers/dns/ionos"
	"github.com/go-acme/lego/v4/providers/dns/namecheap"
	"github.com/go-acme/lego/v4/providers/dns/ovh"
	"github.com/go-acme/lego/v4/providers/dns/porkbun"
	"github.com/go-acme/lego/v4/providers/dns/rfc2136"
	"github.com/go-acme/lego/v4/providers/dns/route53"

	"github.com/Goral64/SuplaOS/internal/addon"
)

// Field is a setting of a DNS provider, passed to lego in the environment
// variable Env.
type Field struct {
	Env      string     `json:"env"`
	Label    addon.Text `json:"label"`
	Secret   bool       `json:"secret,omitempty"`
	Optional bool       `json:"optional,omitempty"`
	// Default is used when the field is left empty.
	Default string `json:"default,omitempty"`
}

// Provider is a DNS provider whose API sets the TXT record of the
// challenge. Only these are built in, each one makes the panel bigger.
type Provider struct {
	Name   string  `json:"name"`
	Label  string  `json:"label"`
	Fields []Field `json:"fields"`
	// env are set for lego besides the fields.
	env map[string]string
	// delegation: the provider keeps zones a CNAME of _acme-challenge
	// points to, e.g. for a domain at a registrar without an API.
	delegation bool
	// wait is how long a new record takes to show, when the servers of
	// the zone cannot be asked (2 minutes when not set).
	wait time.Duration
	new  func() (challenge.Provider, error)
}

func text(en, pl string) addon.Text { return addon.Text{"en": en, "pl": pl} }

func token(env string) Field {
	return Field{Env: env, Label: text("API token", "Token API"), Secret: true}
}

var Providers = []Provider{
	{Name: "cloudflare", Label: "Cloudflare",
		Fields: []Field{{Env: "CF_DNS_API_TOKEN", Label: text("API token (Zone, DNS, Edit)", "Token API (Zone, DNS, Edit)"), Secret: true}},
		new:    func() (challenge.Provider, error) { return cloudflare.NewDNSProvider() }},
	{Name: "duckdns", Label: "Duck DNS",
		Fields:     []Field{{Env: "DUCKDNS_TOKEN", Label: text("Token", "Token"), Secret: true}},
		delegation: true,
		new:        func() (challenge.Provider, error) { return duckdns.NewDNSProvider() }},
	{Name: "desec", Label: "deSEC",
		Fields:     []Field{token("DESEC_TOKEN")},
		delegation: true,
		new:        func() (challenge.Provider, error) { return desec.NewDNSProvider() }},
	{Name: "ovh", Label: "OVH",
		Fields: []Field{
			{Env: "OVH_ENDPOINT", Label: text("Endpoint", "Endpoint"), Optional: true, Default: "ovh-eu"},
			{Env: "OVH_APPLICATION_KEY", Label: text("Application key", "Application key")},
			{Env: "OVH_APPLICATION_SECRET", Label: text("Application secret", "Application secret"), Secret: true},
			{Env: "OVH_CONSUMER_KEY", Label: text("Consumer key", "Consumer key"), Secret: true},
		},
		// The servers of OVH take a few minutes to show a new record.
		env:  map[string]string{"OVH_PROPAGATION_TIMEOUT": "900", "OVH_POLLING_INTERVAL": "10"},
		wait: 5 * time.Minute,
		new:  func() (challenge.Provider, error) { return ovh.NewDNSProvider() }},
	{Name: "hetzner", Label: "Hetzner",
		Fields: []Field{token("HETZNER_API_TOKEN")},
		new:    func() (challenge.Provider, error) { return hetzner.NewDNSProvider() }},
	{Name: "hostinger", Label: "Hostinger",
		Fields: []Field{token("HOSTINGER_API_TOKEN")},
		new:    func() (challenge.Provider, error) { return hostinger.NewDNSProvider() }},
	{Name: "route53", Label: "Amazon Route 53",
		Fields: []Field{
			{Env: "AWS_ACCESS_KEY_ID", Label: text("Access key ID", "Access key ID")},
			{Env: "AWS_SECRET_ACCESS_KEY", Label: text("Secret access key", "Secret access key"), Secret: true},
			{Env: "AWS_REGION", Label: text("Region", "Region"), Optional: true, Default: "us-east-1"},
			{Env: "AWS_HOSTED_ZONE_ID", Label: text("Hosted zone ID", "Hosted zone ID"), Optional: true},
		},
		new: func() (challenge.Provider, error) { return route53.NewDNSProvider() }},
	{Name: "digitalocean", Label: "DigitalOcean",
		Fields: []Field{token("DO_AUTH_TOKEN")},
		new:    func() (challenge.Provider, error) { return digitalocean.NewDNSProvider() }},
	{Name: "gandiv5", Label: "Gandi",
		Fields: []Field{{Env: "GANDIV5_PERSONAL_ACCESS_TOKEN", Label: text("Personal access token", "Personal access token"), Secret: true}},
		new:    func() (challenge.Provider, error) { return gandiv5.NewDNSProvider() }},
	{Name: "godaddy", Label: "GoDaddy",
		Fields: []Field{
			{Env: "GODADDY_API_KEY", Label: text("API key", "Klucz API")},
			{Env: "GODADDY_API_SECRET", Label: text("API secret", "Sekret API"), Secret: true},
		},
		new: func() (challenge.Provider, error) { return godaddy.NewDNSProvider() }},
	// Namecheap only lets in addresses added in its settings: the public
	// address of the device.
	{Name: "namecheap", Label: "Namecheap",
		Fields: []Field{
			{Env: "NAMECHEAP_API_USER", Label: text("API user", "Użytkownik API")},
			{Env: "NAMECHEAP_API_KEY", Label: text("API key", "Klucz API"), Secret: true},
		},
		new: func() (challenge.Provider, error) { return namecheap.NewDNSProvider() }},
	{Name: "porkbun", Label: "Porkbun",
		Fields: []Field{
			{Env: "PORKBUN_API_KEY", Label: text("API key", "Klucz API")},
			{Env: "PORKBUN_SECRET_API_KEY", Label: text("Secret API key", "Sekretny klucz API"), Secret: true},
		},
		new: func() (challenge.Provider, error) { return porkbun.NewDNSProvider() }},
	{Name: "ionos", Label: "IONOS",
		Fields: []Field{{Env: "IONOS_API_KEY", Label: text("API key (prefix.secret)", "Klucz API (prefiks.sekret)"), Secret: true}},
		new:    func() (challenge.Provider, error) { return ionos.NewDNSProvider() }},
	{Name: "cloudns", Label: "ClouDNS",
		Fields: []Field{
			{Env: "CLOUDNS_AUTH_ID", Label: text("API user ID", "ID użytkownika API")},
			{Env: "CLOUDNS_AUTH_PASSWORD", Label: text("API user password", "Hasło użytkownika API"), Secret: true},
		},
		new: func() (challenge.Provider, error) { return cloudns.NewDNSProvider() }},
	// The account of acme-dns is registered on first use and kept in
	// ACME_DNS_STORAGE_PATH, set by the Manager. Then it asks for a CNAME.
	{Name: "acmedns", Label: "acme-dns",
		Fields:     []Field{{Env: "ACME_DNS_API_BASE", Label: text("Address of acme-dns", "Adres acme-dns")}},
		delegation: true,
		new:        func() (challenge.Provider, error) { return acmedns.NewDNSProvider() }},
	{Name: "rfc2136", Label: "RFC 2136 (BIND, PowerDNS, Technitium…)",
		Fields: []Field{
			{Env: "DNSUPDATE_NAMESERVER", Label: text("DNS server (host:port)", "Serwer DNS (host:port)")},
			{Env: "DNSUPDATE_TSIG_KEY", Label: text("TSIG key name", "Nazwa klucza TSIG"), Optional: true},
			{Env: "DNSUPDATE_TSIG_SECRET", Label: text("TSIG secret", "Sekret TSIG"), Secret: true, Optional: true},
			{Env: "DNSUPDATE_TSIG_ALGORITHM", Label: text("TSIG algorithm", "Algorytm TSIG"), Optional: true, Default: "hmac-sha256."},
		},
		delegation: true,
		new:        func() (challenge.Provider, error) { return rfc2136.NewDNSProvider() }},
}

// FindProvider returns the provider called name, nil when there is none.
func FindProvider(name string) *Provider {
	for i := range Providers {
		if Providers[i].Name == name {
			return &Providers[i]
		}
	}
	return nil
}
