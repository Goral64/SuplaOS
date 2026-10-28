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
nazwą, a także pod nazwami panelu i dodatków (niżej). Do certyfikatu rekordy
DNS nie są potrzebne, więc wybierz, co ci pasuje:

- Lokalny wpis DNS dla tej nazwy w routerze, Pi-hole albo AdGuard Home,
  wskazujący na urządzenie. Nic o twojej sieci nie trafia do publicznego DNS.
  Jeśli na routerze działa reverse proxy (np. Nginx Proxy Manager), nazwa
  musi wskazywać na sam SuplaOS, nie na proxy, inaczej odpowie ono swoim
  certyfikatem.
- Plik hosts na komputerze (np.
  `192.168.1.20 supla.example.pl suplaos.example.pl`), dobry na próbę, ale
  na telefonach zwykle się tego nie da zrobić.
- Rekord A w publicznym DNS z adresem urządzenia, np.
  `supla.example.pl A 192.168.1.20`, albo jeden rekord wildcard
  `*.example.pl` dla wszystkich nazw. Działa w całym domu bez żadnych innych
  ustawień. Niektóre routery (np. Fritz!Box, OpenWrt) blokują nazwy
  z publicznego DNS wskazujące na adresy prywatne (ochrona przed DNS
  rebinding), wtedy dodaj tam domenę jako wyjątek.

SUPLA Scripts dostaje zmiany kanałów z Supla Cloud na swój adres, więc jego
nazwa musi działać także na samym urządzeniu, w DNS, z którego korzysta
urządzenie (zwykle router).

## Ustawienie

1. W panelu wejdź w System, Dostęp, Własna domena (Let's Encrypt), Ustaw domenę.
2. Wpisz domenę, wybierz dostawcę DNS i uzupełnij jego klucze (niżej).
   E-mail jest opcjonalny.
3. Kliknij Zapisz i pobierz certyfikat. Trwa to od kilku sekund do kilku
   minut, w logu widać, co się dzieje. Gdy router sam odpowiada na
   zapytania DNS (wiele tak robi), SuplaOS nie widzi rekordu i po prostu
   czeka: ok. 2 minuty, z OVH 5.

Gdy certyfikat jest gotowy, karta proponuje ustawić adresy Supla Cloud
i SUPLA Scripts na domenę i nazwę SUPLA Scripts (Użyj domeny). Potem Cloud
pokazuje też brokera MQTT pod domeną. Jeśli SUPLA Scripts loguje się przez
Supla Cloud, połącz je ponownie na stronie SUPLA Scripts (Połącz z Supla
Cloud).

Serwer testowy Let's Encrypt służy do wypróbowania ustawień: przeglądarki
nie ufają jego certyfikatom, ale nie liczy się on do limitów Let's Encrypt.

## Nazwy panelu i dodatków

Supla Cloud odpowiada pod domeną. Panel i dodatki z interfejsem WWW dostają
własne nazwy obok niej: dla `supla.example.pl` panel to
`suplaos.example.pl`, Zigbee2MQTT `supla-z2m.example.pl`, a SUPLA Scripts
`supla-scripts.example.pl`. Gdy domena jest tuż pod sufiksem publicznym
(`example.pl`, `mojanazwa.duckdns.org`), nazwy są pod nią:
`suplaos.example.pl`, `z2m.mojanazwa.duckdns.org`. Nazwy można zmienić na
karcie, puste pole to nazwa domyślna. Certyfikat ma je wszystkie, a dodatek
zainstalowany później sam dostaje swoją nazwę w nowym certyfikacie.

W DuckDNS i deSEC nazwa obok domeny to kolejna nazwa do założenia tam
(DuckDNS daje pięć za darmo), nazwa pod domeną nie.

Wszystkie działają na porcie 443: urządzenie patrzy, o jaką nazwę pyta
przeglądarka, i przekazuje połączenie dalej. Pod adresem IP, `<nazwa>.local`
i każdą inną nazwą port 443 to jak dotąd Supla Cloud. W sieci lokalnej panel
i dodatki są też pod swoimi portami (8443, 8080, 8444). Panel otwarty pod
swoją nazwą otwiera dodatki pod ich nazwami.

Logujesz się raz, w panelu: Zigbee2MQTT pod swoją nazwą bierze sesję
stamtąd.

## Dostęp z internetu

Skoro wszystko jest na porcie 443, z internetu wystarczy przekierować na
urządzenie ten jeden port i dać nazwom rekordy A z twoim publicznym adresem.
Aplikacja Supla i urządzenia łączą się po porcie 2016, przekieruj go też,
jeśli aplikacja ma działać poza domem. 8883 tylko dla klientów MQTT
z zewnątrz. Nie przekierowuj 2015, nie jest szyfrowany.

Panel i Zigbee2MQTT odpowiadają tylko w sieci lokalnej, dopóki nie włączysz
ich w System, Dostęp, Dostęp z internetu. Supla Cloud i SUPLA Scripts mają
własne logowanie i działają zawsze. Panelem da się zmienić wszystko na
urządzeniu, więc wystawiając go do internetu, użyj długiego hasła. Po pięciu
złych hasłach z jednego adresu logowanie stamtąd czeka, po każdym kolejnym
dłużej.

Bez stałego publicznego adresu da się też przez tunel Cloudflare: dodaj
Public Hostname dla każdej nazwy, wszystkie z usługą HTTPS i adresem
urządzenia w sieci lokalnej z portem 443, a w ustawieniach TLS wpisz tę
nazwę jako Origin Server Name. Żądania przez tunel liczą się jako
z internetu. Tunel przenosi tylko interfejsy WWW. Urządzenia i aplikacja
Supla łączą się po porcie 2016, którego tunel nie przeniesie; w domu i tak
łączą się z urządzeniem bezpośrednio.

## Klucze dostawcy DNS

Klucz pozwala SuplaOS zmieniać DNS domeny, więc daj mu tak mało uprawnień,
jak dostawca pozwala.

- Cloudflare: My Profile, API Tokens, Create Token, szablon Edit zone DNS,
  tylko dla strefy domeny.
- OVH: klucze utworzysz na https://eu.api.ovh.com/createToken/ z prawami
  `POST /domain/zone/example.pl/*` i `DELETE /domain/zone/example.pl/*`
  (zamiast `example.pl` twoja domena) i ważnością Unlimited, inaczej po
  jakimś czasie przestaną działać. Dostaniesz Application key, Application
  secret i Consumer key. Serwery OVH pokazują nowy rekord
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

Nazwy panelu i dodatków potrzebują każda takiego rekordu, np.
`_acme-challenge.suplaos.example.pl CNAME supla-example.duckdns.org`.
W DuckDNS wszystkie mogą wskazywać na tę samą nazwę.

Rekord wildcard w strefie (`*.example.pl`) nie przeszkadza.

## Co się zmienia

- Supla Cloud otwiera się pod `https://supla.example.pl`, panel pod
  `https://suplaos.example.pl` (i `https://supla.example.pl:8443`).
- Panel pod domeną i nazwami odpowiada certyfikatem domeny, a pod
  `<nazwa>.local` i adresem IP jak dotąd certyfikatem urządzenia.
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
