# Own domain (Let's Encrypt)

[Po polsku](pl/domain.md)

With your own domain the device gets a certificate from Let's Encrypt,
which browsers, phones and apps trust out of the box. You don't have to
install the [CA of the device](certificate.md) anywhere then.

SuplaOS gets the certificate with the DNS-01 challenge: it puts a TXT
record in the DNS of the domain through the API of your DNS provider. The
device doesn't have to be reachable from the internet and no port has to be
opened on the router. The certificate is renewed on its own.

## What you need

- A domain or a subdomain, e.g. `supla.example.com`. A free name from
  [DuckDNS](https://www.duckdns.org) (`myname.duckdns.org`) or
  [deSEC](https://desec.io) (`myname.dedyn.io`) works too.
- A DNS provider with an API supported by SuplaOS: Cloudflare, DuckDNS,
  deSEC, OVH, Hetzner, Hostinger, Amazon Route 53, DigitalOcean, Gandi,
  GoDaddy, Namecheap, Porkbun, IONOS, ClouDNS, acme-dns or any DNS server
  with RFC 2136 updates (BIND, PowerDNS, Technitium). If yours is missing,
  ask in Issues.
- If your registrar has no API (many Polish ones don't), it still works
  with a free name at DuckDNS or deSEC, see below.

## The name has to point to the device

The certificate alone isn't enough, browsers have to find the device by
that name. The DNS record isn't needed for the certificate, so pick what
suits you:

- A local DNS entry for the name in the router, Pi-hole or AdGuard Home,
  pointing to the device. Nothing about your network ends up in public DNS.
- The hosts file on a computer (e.g. `192.168.1.20 supla.example.com`), good
  for a try, but phones usually can't do that.
- An A record in public DNS with the address of the device, e.g.
  `supla.example.com A 192.168.1.20`. Works everywhere at home without
  setting anything else. Some routers (e.g. Fritz!Box, OpenWrt) block names
  in public DNS that point to private addresses (DNS rebinding protection),
  then add the domain as an exception there.

Access from outside needs an A record with your public address and ports
forwarded on the router. That's up to you, SuplaOS doesn't need it for the
certificate.

## Setting it up

1. In the panel go to System, Access, Own domain (Let's Encrypt), Set a domain.
2. Enter the domain, pick the DNS provider and fill in its keys (see
   below). The e-mail is optional.
3. Click Save and get the certificate. It takes from a few seconds to a few
   minutes, the log shows what's going on.

When the certificate is there, the card offers to set the addresses of
Supla Cloud and SUPLA Scripts to the domain (Use the domain). After that
the Cloud also shows the MQTT broker at the domain. If SUPLA Scripts logs in
with Supla Cloud, connect them again on the SUPLA Scripts page (Connect with
Supla Cloud).

The test server of Let's Encrypt is for trying the setup out: browsers don't
trust its certificate, but it doesn't count towards the limits of Let's
Encrypt.

## Keys of the DNS provider

The key lets SuplaOS change the DNS of the domain, so give it as few rights
as the provider allows.

- Cloudflare: My Profile, API Tokens, Create Token, template Edit zone DNS,
  only for the zone of the domain.
- OVH: create the keys at https://eu.api.ovh.com/createToken/ with the
  rights `POST /domain/zone/example.com/*` and
  `DELETE /domain/zone/example.com/*` (your domain instead of
  `example.com`). You get the application key, the application secret and
  the consumer key. The servers of OVH show a new record only after a few
  minutes, so getting the certificate takes a while.
- DuckDNS: the token from the top of the page after logging in.
- deSEC: Token Management, a token for the domain.
- The others: the [lego documentation](https://go-acme.github.io/lego/dns/)
  (SuplaOS uses that library) describes where to find the keys.

Saved keys are not shown in the panel anymore. Leave the field empty to keep
the saved value.

## Domain without an API

If the DNS of the domain can't be changed through an API, leave only the
challenge to another service. Make a free name at DuckDNS (e.g.
`supla-example.duckdns.org`) and add one record at your registrar, once:

    _acme-challenge.supla.example.com CNAME supla-example.duckdns.org

In the panel enter `supla.example.com` as the domain and DuckDNS with its
token as the provider. SuplaOS puts the TXT record at DuckDNS and Let's
Encrypt finds it through the CNAME. deSEC and acme-dns work the same way.

A wildcard record in the zone (`*.example.com`) doesn't get in the way.

## What changes

- The panel answers with the certificate of the domain at the domain name,
  and at `<name>.local` and the IP address with the certificate of the
  device, as before.
- Supla Cloud, SUPLA Scripts and the encrypted MQTT port have one
  certificate each: the one of the domain. Opened by the IP address or
  `<name>.local` they show a warning, use the domain for them.
- MQTT clients on port 8883 connect to the domain and check it with the CAs
  of the system, without the CA file of the device.
- The certificate is renewed on its own, long before it expires. If that
  fails, the card shows the error and when the next try is.

## Turning it off

The Turn off button on the card brings back the certificate of the device
everywhere and removes the keys of the DNS provider. Set the addresses of Supla Cloud
and SUPLA Scripts back on their add-on pages if you changed them.
