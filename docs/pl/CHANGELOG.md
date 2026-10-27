# Zmiany

[In English](../../CHANGELOG.md)

## 0.9.8

- Własna domena: pobieranie certyfikatu za routerem, który sam odpowiada na
  zapytania DNS (także wysłane prosto do serwerów dostawcy DNS), kończyło
  się błędem „time limit exceeded”, nawet z dobrym tokenem. Teraz SuplaOS
  czeka na rekord w jednym kroku i sprawdzenie zostawia Let's Encrypt.
  Trwa to ok. 2 minuty (z OVH 5).

## 0.9.7

- Okna zakończenia przenoszenia i zapomnienia konta źródłowego miały
  Anuluj na obu przyciskach. Przycisk potwierdzenia mówi teraz Zakończ
  przenoszenie albo Zapomnij.

## 0.9.6

- Odczyt konta źródłowego do przeniesienia najpierw sprawdza zakresy tokena
  i wymienia sekcje bez Odczytu. Gdy źródłowy Cloud mimo to czegoś odmówi,
  komunikat pokazuje jego powód i zakresy, które widzi w tokenie.

## 0.9.5

- Zigbee2MQTT startuje od razu z ustawieniami dodatku, bez własnej strony
  pierwszego uruchomienia (która do tego przekierowywała na localhost). Klucz
  sieci i identyfikatory losują się przy pierwszym starcie.
- Typ koordynatora USB (zstack, ember, deconz...) ustawia się w dodatku:
  Zigbee2MQTT nie rozpozna go ze swojego kontenera. Wcześniejsze instalacje
  zachowują typ wybrany na stronie pierwszego uruchomienia.

## 0.9.4

- Bez sieci (bez kabla i bez znanego Wi-Fi w zasięgu przez 2 minuty)
  urządzenie z Wi-Fi tworzy własną sieć, `SuplaOS-XXXX` z hasłem
  `suplaos-setup`. Telefon z nią połączony otwiera panel, w którym wybiera
  się Wi-Fi. Pierwsze uruchomienie nie wymaga już kabla.
- Dociąganie nowszej historii pomiarów pomija kanały usunięte w międzyczasie
  w źródle, zamiast zatrzymywać się na pierwszym z nich.
- Przycisk Otwórz zatrzymanego albo niedziałającego dodatku jest wyszarzony.

## 0.9.2

- Przeniesienie z Supla Cloud przenosi też SUPLA Scripts: sceny, grupy scen,
  termostaty i powiadomienia trafiają do SUPLA Scripts zainstalowanego
  tutaj, razem z odwołaniami do kanałów i do siebie nawzajem. SUPLA Scripts
  wymaga tokena z zakresem Kanały: wykonywanie akcji; jeśli token
  przeniesienia ma tylko zakresy odczytu, w karcie można podać drugi.
  Przeniesienie zaczęte w 0.9.0 albo 0.9.1 też może przenieść SUPLA Scripts.

## 0.9.1

- Przeniesienie z Supla Cloud podnosi limity konta tutaj (liczbę urządzeń,
  lokalizacji, identyfikatorów dostępu, scen, harmonogramów i inne), żeby
  zmieściło się źródło z zapasem. W 0.9.0 konto z ponad 100 urządzeniami
  przenosiło się, ale potem nie dało się dodać żadnego urządzenia.
- Strefa czasowa konta jest przenoszona ze źródła.
- Tytuły okien zadań są przetłumaczone.

## 0.9.0

- Przeniesienie z innego Supla Cloud (strona dodatku Supla): konto
  w publicznym Supla Cloud albo na innym własnym serwerze przechodzi tu
  razem z urządzeniami, kanałami, lokalizacjami, identyfikatorami dostępu,
  grupami kanałów, scenami, harmonogramami, reakcjami, linkami
  bezpośrednimi, ikonami i historią pomiarów. Urządzenia i kanały zachowują
  swoje ID, każde urządzenie dostaje swoje ustawienia, gdy przestawi się je
  na ten serwer. Zobacz [przeniesienie z Supla Cloud](migration.md).
- supla-server ma certyfikat podpisany przez CA urządzenia, z jego adresem.
  Urządzenia mogą go sprawdzać przez Custom CA, certyfikat do wklejenia jest
  na stronie dodatku Supla. Zigbee2Supla sprawdza go tak samo.
- Karta Wi-Fi pokazuje nowe połączenie bez odświeżania strony.

## 0.8.2

- Wentylator obudów Raspberry Pi (System, Ogólne): Argon ONE dostaje obroty
  według temperatury, a jego przycisk zasilania uruchamia system ponownie
  (dwa naciśnięcia) albo go wyłącza (przytrzymanie); wentylatorem na pinie
  GPIO (oficjalny wentylator obudowy Pi 4, Fan SHIM) steruje jądro. Active
  Cooler w Pi 5 nie potrzebuje niczego.
- Klucze SSH w panelu (System, Dostęp): dodawanie klucza publicznego
  i usuwanie.
- Kopie mają własną stronę, System jest podzielony na Ogólne, Sieć i Dostęp.

## 0.8.1

- Wi-Fi działa według przepisów kraju ze strefy czasowej, więc działają
  sieci 5 GHz. Wcześniej Raspberry Pi nie widziało sieci 5 GHz, a sterownik
  Wi-Fi zapełniał ekran komunikatami „set chanspec ... fail”.
- Na ekranie urządzenia są tylko krytyczne komunikaty jądra, reszta jest
  w dzienniku.
- Przywracanie pełnej kopii czeka na nową bazę do 10 minut zamiast 2: na
  Raspberry Pi tyle trwa jej pierwsze uruchomienie.

## 0.8.0

- Pełna kopia urządzenia (System, Pełna kopia): wszystkie dodatki
  z ustawieniami i danymi, CA i własna domena w jednym pliku, zaszyfrowane
  hasłem (age). Przywraca się ją na tym samym urządzeniu albo, w kreatorze,
  na innym, żeby się tam przenieść. Zobacz [pełna kopia](backup.md).
- Kopie automatyczne codziennie albo co tydzień, na dysk urządzenia albo
  udział SMB lub NFS, z kilkoma najnowszymi.
- `suplaos export` i `suplaos import` robią to samo przez SSH.
- Obrazy i aktualizacja dokładają cifs-utils i nfs-common; dołączony do nich
  rpcbind jest wyłączony.

## 0.7.0

- Wi-Fi w panelu (System, Wi-Fi, i w kreatorze pierwszego uruchomienia):
  wyszukiwanie sieci, łączenie, rozłączanie, zapamiętane sieci z włączanym
  albo wyłączanym łączeniem automatycznym. Po restarcie urządzenie łączy się
  z siecią samo, więc kabel można odłączyć.
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
