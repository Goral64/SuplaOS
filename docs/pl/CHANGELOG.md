# Zmiany

[In English](../../CHANGELOG.md)

## Niewydane

- Wi-Fi w panelu (System, Wi-Fi, i w kreatorze pierwszego uruchomienia):
  wyszukiwanie sieci, łączenie, usuwanie zapamiętanej. Po restarcie
  urządzenie łączy się z nią samo, więc kabel można odłączyć.
- Obrazy mają wpa_supplicant, bez którego Wi-Fi nie działało, a obraz x86-64
  także firmware kart Intel i Atheros. Aktualizacja dokłada to samo do
  istniejącej instalacji.

## 0.6.0

- Własna domena z certyfikatem Let's Encrypt (System, Własna domena):
  certyfikat pobierany przez API dostawcy DNS (DNS-01), więc urządzenie nie
  musi być widoczne z internetu. 16 dostawców DNS, domena u rejestratora bez
  API działa przez CNAME do DuckDNS albo deSEC. Certyfikat odnawia się sam.
  Zobacz [własna domena](domain.md).
- Z ustawioną domeną Supla Cloud, SUPLA Scripts i szyfrowany port MQTT
  używają jej certyfikatu, a panel odpowiada nim pod domeną, a certyfikatem
  urządzenia pod nazwą .local i adresem IP. Adresy Supla Cloud i SUPLA
  Scripts można jednym przyciskiem ustawić na domenę.

## 0.5.2

- Nowy certyfikat (np. po zmianie adresu urządzenia) uruchamia ponownie
  tylko usługi, które z niego korzystają: Supla Cloud, a nie bazę danych
  i supla-server.
- Kontenery znajdują urządzenie po nazwie (<nazwa>.local), np. SUPLA Scripts
  dociera do Cloud, gdy jego adres to suplaos.local.
- Po zmianie adresu albo nazwy urządzenia kontenery, które z nich korzystają,
  same dostają nowe wartości (np. adres brokera MQTT w Supla Cloud).

## 0.5.1

- SUPLA Scripts pozwala logować się przez Supla Cloud (OAuth) i dostaje od
  niego zmiany kanałów. Gdy Supla działa na tym samym urządzeniu, aplikację
  OAuth zakłada sam panel (Połącz z Supla Cloud na stronie dodatku).
- supla-server ufa CA urządzenia, więc jego webhooki docierają po HTTPS do
  usług na urządzeniu.
- Przeglądarki nie wpisują już zapamiętanych loginów w pola haseł ustawień
  dodatków (np. Mosquitto), przez co zapis formularza mógł zapisać złe
  wartości.

## 0.5.0

- Nowy dodatek: SUPLA Scripts (sceny, termostaty, polecenia głosowe,
  powiadomienia), na porcie 8444, z własną bazą i kopiami.
- Po aktualizacji dodatki, których skrypty startowe się zmieniły, są
  uruchamiane ponownie, więc nowe skrypty działają od razu.
- Dwie kopie zrobione w tej samej sekundzie już się nie gryzą (np.
  przywrócenie zaraz po zrobieniu kopii).

## 0.4.2

- CA każdego urządzenia ma własną nazwę. CA z 0.4.0 i 0.4.1 miały wszystkie
  tę samą i przeglądarki je myliły, np. Firefox nie otwierał panelu drugiego
  urządzenia. Po aktualizacji wymieniają się same: zainstaluj ponownie
  certyfikat CA (strona System) i usuń stary `SuplaOS suplaos`. Klienci MQTT
  na porcie 8883 też potrzebują nowego CA.
- Zmiana ustawienia Supli nie uruchamia już ponownie jej bazy danych.
- Pierwsze konto Supli można podać od razu przy instalacji Supli, także
  w kreatorze pierwszego uruchomienia.

## 0.4.1

- Kopie można przywracać: Przywróć na stronie dodatku Supla albo
  `suplaos restore supla <plik>`. Najpierw powstaje kopia bieżących danych.
- Wgraj kopię wgrywa pobraną kopię na urządzenie, np. nowe.
- Kopie bazy Supli zawierają teraz też jej procedury.
- Zmiany w SuplaOS można przeczytać w panelu (Zmiany na stronie
  Aktualizacje), także dla wersji czekającej na instalację.
- Uruchom ponownie i Wyłącz otwierają stronę, która czeka na urządzenie
  i sama wraca do panelu, gdy ono wstanie.

## 0.4.0

- Tylko HTTPS. Panel jest teraz pod https://<nazwa>.local:8443, port 80
  tylko tam przekierowuje. Supla Cloud zostaje na 443 i nie nasłuchuje już
  na 8081. Interfejs Zigbee2MQTT jest dalej na porcie 8080, ale po HTTPS
  i dopiero po zalogowaniu do panelu.
- Urządzenie ma własny urząd certyfikacji. Wystarczy raz zainstalować jego
  certyfikat (strona System, [jak](certificate.md)), a panel, Supla Cloud,
  Zigbee2MQTT i MQTT na porcie 8883 działają bez ostrzeżeń. Certyfikaty
  odnawiają się same po zmianie nazwy albo adresu urządzenia. Certyfikat
  Cloud nie wygasa już po roku.
- Klienci MQTT ze starym `server.crt` z Mosquitto potrzebują zamiast niego
  certyfikatu CA (strona dodatku Mosquitto albo strona System).
- Nowe pliki dodatków z aktualizacji stosują się same zaraz po niej
  (najpierw z kopią), bez klikania Zastosuj.

Po aktualizacji ze starszej wersji odśwież stronę panelu, jest już pod nowym
adresem, a przeglądarka ostrzega przed certyfikatem, dopóki nie zainstalujesz
CA.

## 0.3.1

- Log aktualizacji systemu otwiera się w tym samym oknie co inne operacje,
  ze stanem i przyciskiem Zamknij; Pokaż log otwiera go później ponownie.
- Potwierdzenia wyglądają jak reszta panelu, a nie jak okienka przeglądarki.
- Anuluj na stronie zmiany hasła.
- Nowa kompilacja tej samej wersji obrazu pokazuje datę zbudowania zamiast
  „26.05.01 → 26.05.01”.
- Nowe wersje supla-docker są sprawdzane codziennie i po przejściu testów
  trafiają tu jako pull request.

## 0.3.0

- Panel co kilka godzin sprawdza, czy dodatki mają nowsze obrazy. Na
  stronie dodatku widać zainstalowaną wersję, nową wersję i link do listy
  zmian, a Aktualizuj jest aktywny tylko wtedy, gdy jest co aktualizować.
  Dodatki z aktualizacją są oznaczone na pulpicie, liście dodatków i
  stronie Aktualizacje.
- Uruchom, Zatrzymaj i Uruchom ponownie zależą od stanu dodatku.
- Aktualizacja dodatku nie tworzy już od nowa kontenerów innych dodatków
  (aktualizacja Supli restartowała zigbee2supla).
- Pulpit: czas pracy w dniach, godzinach i minutach, liczba CPU w osobnej
  linii.

## 0.2.5

- `suplaos run supla sql` wykonuje SQL na bazie Supli: skrypt
  (`< plik.sql`), zapytanie (`-e "..."`) albo klient MariaDB.
- README opisuje, jak uruchamiać polecenia konsoli Supla Cloud, np. zmianę
  limitów konta.

## 0.2.4

- Ustawienia pierwszego startu (nowe klucze SSH, powiększenie partycji)
  działają tylko na obrazach SuplaOS, a nie po instalacji pakietu gdzie
  indziej.

## 0.2.3

- Polska dokumentacja w `docs/pl`.

## 0.2.2

- Przypomnienie o restarcie pojawia się po każdej aktualizacji jądra, także
  automatycznej, i również na pulpicie.
- Automatyczne aktualizacje Debiana można wyłączyć na stronie Aktualizacje.

## 0.2.1

- Przy polskim języku systemu panel nie widział aktualizacji.
- Aktualizacja czeka, gdy apt jest zajęty, zamiast kończyć się błędem.

## 0.2.0

- Aktualizacje Debiana, jądra Raspberry Pi i samego SuplaOS z panelu
  (Aktualizacje) albo przez `sudo suplaos upgrade`. SuplaOS ma własne
  repozytorium apt.
- Nowe wersje plików dodatków stosuje się na żądanie, po zrobieniu kopii.
- qemu-guest-agent w obrazie x86-64, opis maszyn wirtualnych.

## 0.1.0

- Pierwsze obrazy na Raspberry Pi 4/5 i x86-64, z panelem WWW i dodatkami
  Supla, Mosquitto, Zigbee2MQTT i zigbee2supla. Bez aktualizacji, lepiej
  zainstalować 0.2.0 lub nowszy.
