# Własna domena (Let's Encrypt)

[In English](../domain.md)

Z własną domeną urządzenie dostaje certyfikat Let's Encrypt, któremu od razu
ufają przeglądarki, telefony i aplikacje. Nie trzeba wtedy nigdzie instalować
[CA urządzenia](certificate.md).

SuplaOS pobiera certyfikat metodą DNS-01: przez API twojego dostawcy DNS
wpisuje rekord TXT w DNS domeny. Urządzenie nie musi być widoczne
z internetu i nie trzeba otwierać żadnego portu na routerze. Certyfikat
odnawia się sam.

## Co jest potrzebne

- Domena albo subdomena, np. `supla.example.pl`. Wystarczy też darmowa nazwa
  z [DuckDNS](https://www.duckdns.org) (`mojanazwa.duckdns.org`) albo
  [deSEC](https://desec.io) (`mojanazwa.dedyn.io`).
- Dostawca DNS z API obsługiwanym przez SuplaOS: Cloudflare, DuckDNS, deSEC,
  OVH, Hetzner, Hostinger, Amazon Route 53, DigitalOcean, Gandi, GoDaddy,
  Namecheap, Porkbun, IONOS, ClouDNS, acme-dns albo dowolny serwer DNS
  z aktualizacjami RFC 2136 (BIND, PowerDNS, Technitium). Jeśli twojego
  brakuje, napisz w Issues.
- Jeśli rejestrator nie ma API (wiele polskich nie ma), też się da, przez
  darmową nazwę w DuckDNS albo deSEC, opis niżej.

## Nazwa musi wskazywać na urządzenie

Sam certyfikat nie wystarczy, przeglądarka musi znaleźć urządzenie pod tą
nazwą. Do certyfikatu rekord DNS nie jest potrzebny, więc wybierz, co ci
pasuje:

- Lokalny wpis DNS dla tej nazwy w routerze, Pi-hole albo AdGuard Home,
  wskazujący na urządzenie. Nic o twojej sieci nie trafia do publicznego DNS.
- Plik hosts na komputerze (np. `192.168.1.20 supla.example.pl`), dobry na
  próbę, ale na telefonach zwykle się tego nie da zrobić.
- Rekord A w publicznym DNS z adresem urządzenia, np.
  `supla.example.pl A 192.168.1.20`. Działa w całym domu bez żadnych innych
  ustawień. Niektóre routery (np. Fritz!Box, OpenWrt) blokują nazwy
  z publicznego DNS wskazujące na adresy prywatne (ochrona przed DNS
  rebinding), wtedy dodaj tam domenę jako wyjątek.

Dostęp z zewnątrz wymaga rekordu A z twoim publicznym adresem
i przekierowania portów na routerze. To twoja decyzja, do certyfikatu
SuplaOS tego nie potrzebuje.

## Ustawienie

1. W panelu wejdź w System, Własna domena (Let's Encrypt), Ustaw domenę.
2. Wpisz domenę, wybierz dostawcę DNS i uzupełnij jego klucze (niżej).
   E-mail jest opcjonalny.
3. Kliknij Zapisz i pobierz certyfikat. Trwa to od kilku sekund do kilku
   minut, w logu widać, co się dzieje.

Gdy certyfikat jest gotowy, karta proponuje ustawić adresy Supla Cloud
i SUPLA Scripts na domenę (Użyj domeny). Potem Cloud pokazuje też brokera
MQTT pod domeną. Jeśli SUPLA Scripts loguje się przez Supla Cloud, połącz je
ponownie na stronie SUPLA Scripts (Połącz z Supla Cloud).

Serwer testowy Let's Encrypt służy do wypróbowania ustawień: przeglądarki
nie ufają jego certyfikatom, ale nie liczy się on do limitów Let's Encrypt.

## Klucze dostawcy DNS

Klucz pozwala SuplaOS zmieniać DNS domeny, więc daj mu tak mało uprawnień,
jak dostawca pozwala.

- Cloudflare: My Profile, API Tokens, Create Token, szablon Edit zone DNS,
  tylko dla strefy domeny.
- OVH: klucze utworzysz na https://eu.api.ovh.com/createToken/ z prawami
  `POST /domain/zone/example.pl/*` i `DELETE /domain/zone/example.pl/*`
  (zamiast `example.pl` twoja domena). Dostaniesz Application key,
  Application secret i Consumer key. Serwery OVH pokazują nowy rekord
  dopiero po kilku minutach, więc pobranie certyfikatu chwilę trwa.
- DuckDNS: token z góry strony po zalogowaniu.
- deSEC: Token Management, token dla domeny.
- Pozostali: [dokumentacja lego](https://go-acme.github.io/lego/dns/)
  (z tej biblioteki korzysta SuplaOS) opisuje, gdzie znaleźć klucze.

Zapisanych kluczy panel już nie pokazuje. Zostaw pole puste, żeby zachować
zapisaną wartość.

## Domena bez API

Jeśli DNS domeny nie da się zmieniać przez API, oddaj innej usłudze tylko
samo wyzwanie. Załóż darmową nazwę w DuckDNS (np.
`supla-example.duckdns.org`) i raz dodaj u rejestratora jeden rekord:

    _acme-challenge.supla.example.pl CNAME supla-example.duckdns.org

W panelu wpisz domenę `supla.example.pl`, a jako dostawcę DuckDNS z jego
tokenem. SuplaOS wpisuje rekord TXT w DuckDNS, a Let's Encrypt znajduje go
przez CNAME. Tak samo działa deSEC i acme-dns.

Rekord wildcard w strefie (`*.example.pl`) nie przeszkadza.

## Co się zmienia

- Panel pod nazwą domeny odpowiada certyfikatem domeny, a pod `<nazwa>.local`
  i adresem IP jak dotąd certyfikatem urządzenia.
- Supla Cloud, SUPLA Scripts i szyfrowany port MQTT mają po jednym
  certyfikacie: certyfikat domeny. Otwarte przez adres IP albo
  `<nazwa>.local` pokażą ostrzeżenie, używaj dla nich domeny.
- Klienci MQTT na porcie 8883 łączą się z domeną i sprawdzają ją systemowymi
  CA, bez pliku CA urządzenia.
- Certyfikat odnawia się sam, dużo przed wygaśnięciem. Jeśli się nie uda,
  karta pokazuje błąd i termin kolejnej próby.

## Wyłączenie

Przycisk Wyłącz na karcie przywraca wszędzie certyfikat urządzenia i usuwa
klucze dostawcy DNS. Adresy Supla Cloud i SUPLA Scripts, jeśli były zmienione na
domenę, ustaw z powrotem na stronach tych dodatków.
