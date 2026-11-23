# Pierwsze kroki

[English](../start.md)

Od pustej karty do pierwszego urządzenia w Supli. Bardziej szczegółowy opis
każdej części jest w [README](README.md).

## 1. Sprzęt

- Raspberry Pi 4 (najlepiej z dyskiem SSD na USB zamiast karty SD) albo
  Raspberry Pi 5. Pi 3B+ też działa, ale przy 1 GB RAM wystarczy na samą
  Suplę.
- Komputer x86-64, np. mini PC albo terminal jak HP T630.
- Maszyna wirtualna: Proxmox, VirtualBox, VMware, Hyper-V, zobacz
  [maszyny wirtualne](virtual-machines.md).

Na start najprościej podłączyć urządzenie kablem do routera.

## 2. Obraz

Pobierz z [Releases](https://github.com/Goral64/SuplaOS/releases) plik:

- `suplaos-<wersja>-rpi-arm64.img.xz` na Raspberry Pi,
- `suplaos-<wersja>-amd64.img.xz` na komputer x86-64 i maszynę wirtualną.

## 3. Wgranie

Wgraj obraz na kartę SD, dysk USB albo SSD przez Raspberry Pi Imager
(„Wybierz OS”, potem „Użyj własnego”) albo balenaEtcher. Oba biorą plik
`.xz` bez rozpakowywania. W Raspberry Pi Imager pomiń „ustawienia OS”,
SuplaOS z nich nie korzysta. Wgranie kasuje wszystko, co było na tym
nośniku.

## 4. Pierwsze włączenie i panel

Włóż nośnik, podłącz kabel sieciowy, włącz urządzenie i poczekaj 2-3
minuty: przy pierwszym starcie system powiększa się na cały dysk.

Otwórz w przeglądarce https://suplaos.local:8443. Nie każdy telefon
i komputer zna nazwy `.local`. Wtedy weź adres IP z listy urządzeń
w routerze albo z ekranu podłączonego do urządzenia i otwórz
`https://<adres IP>:8443`.

Przeglądarka ostrzeże przed certyfikatem, bo urządzenie tworzy własny.
Na początek go zaakceptuj, potem możesz zainstalować
[certyfikat CA urządzenia](certificate.md) i ostrzeżenia znikną.

Zaloguj się hasłem `supla`. Panel od razu poprosi o nowe hasło, to samo
działa później przy SSH (`ssh supla@suplaos.local`).

Bez kabla: urządzenie z Wi-Fi, które przez 2 minuty nie znajdzie sieci,
tworzy własną, `SuplaOS-XXXX` z hasłem `suplaos-setup`. Połącz z nią
telefon, panel otworzy się sam (albo otwórz https://192.168.4.1:8443),
i wybierz tam swoje Wi-Fi.

## 5. Kreator

Wybierasz język, strefę czasową i nazwę urządzenia. Od nazwy zależy adres
panelu: z nazwą `dom` będzie to https://dom.local:8443.

Jeśli przenosisz się z innego urządzenia z SuplaOS, tu przywracasz
[pełną kopię](backup.md).

## 6. Supla i konto

W ostatnim kroku kreatora zainstaluj Suplę (albo później w Dodatkach,
Supla). Pobieranie obrazów trwa kilka minut, zależnie od łącza. Od razu
możesz podać pierwsze konto, e-mail i hasło. Konto nie wymaga potwierdzenia
e-mailem. Można je też założyć później na stronie dodatku Supla.

Supla Cloud jest potem pod `https://<adres urządzenia>`, bez portu.
Zaloguj się tam tym kontem.

Masz już konto w publicznym Supla Cloud? Możesz je przenieść razem
z urządzeniami i historią, zobacz
[przeniesienie z Supla Cloud](migration.md). Zrób to od razu, zanim
podłączysz tu urządzenia.

## 7. Aplikacja i pierwsze urządzenie

Nowe konto ma przez pierwszy tydzień włączoną rejestrację klientów
(aplikacji na telefonach) i urządzeń. Później nowy telefon dopisze się do
konta dopiero po włączeniu rejestracji klientów w Supla Cloud, na stronie
Smartfony przyciskiem „Rejestracja klientów”. Działa wtedy przez 24
godziny.

W aplikacji SUPLA na telefonie podaj e-mail konta w Twoim Supla Cloud,
a w ustawieniach zaawansowanych konta adres serwera: adres IP SuplaOS.
Aplikacja i urządzenia łączą się z serwerem po porcie 2016. Poza domem
aplikacja zadziała dopiero z [własną domeną](domain.md) i przekierowanym
portem 2016.

Urządzenie najprościej dodać kreatorem w aplikacji („Dodaj urządzenie”),
gdy aplikacja jest już połączona z Twoim serwerem. Kreator poradzi sobie
tylko z urządzeniem, które:

- jest w trybie konfiguracji i ma wtedy własną sieć Wi-Fi o nazwie
  zaczynającej się od `SUPLA-` (albo prefiksu producenta, np. `ZAMEL-`)
  i kończącej się częścią adresu MAC, jak w GUI-Generic i innych firmware
  na SuplaDevice oraz w modułach producentów współpracujących z Suplą,
- ma pod 192.168.4.1 standardową stronę konfiguracji Supli (Wi-Fi, serwer,
  e-mail),
- łączy się z Twoim Wi-Fi w paśmie, które obsługuje: ESP8266 i większość
  ESP32 tylko 2,4 GHz.

Rejestracji urządzeń nie trzeba włączać w Cloud: gdy jest wyłączona,
kreator poprosi o hasło konta i sam ją włączy. Kreator ustawia w
urządzeniu Wi-Fi, adres serwera (ten, z którym połączona jest aplikacja) i
e-mail. Nie zmienia weryfikacji certyfikatu: moduły, które mają domyślnie
Supla CA (np. Zamela), nie połączą się wtedy z żadnym własnym serwerem. Po
kreatorze wprowadź taki moduł jeszcze raz w tryb konfiguracji i ustaw
weryfikację jak niżej.

Bez aplikacji rejestracja urządzeń musi być włączona: po pierwszym
tygodniu konta włącz ją w Supla Cloud na stronie Moja SUPLA przyciskiem
„Rejestracja urządzeń” (działa 24 godziny). Potem w ustawieniach
urządzenia (strona konfiguracji GUI-Generic i innych firmware na
SuplaDevice, zwykle pod 192.168.4.1 w trybie konfiguracji) ustaw:

- serwer: adres IP SuplaOS w Twojej sieci,
- e-mail: e-mail konta w Twoim Supla Cloud,
- weryfikację certyfikatu: Custom CA z certyfikatem z karty
  „Certyfikat CA dla urządzeń” na stronie dodatku Supla w panelu, albo Skip
  certificate verification, jeśli firmware nie ma Custom CA.

Po zapisaniu urządzenie powinno pojawić się w Supla Cloud po kilku
sekundach.

## 8. Kopie

Na stronie Kopie włącz automatyczną pełną kopię, najlepiej na udział
sieciowy albo pobieraj ją co jakiś czas na komputer. Kopia na tej samej
karcie nie pomoże, gdy karta padnie. Zobacz [pełna kopia](backup.md).

## Gdy coś nie działa

- Panel się nie otwiera: sprawdź adres IP w routerze i pamiętaj o `https://`
  i porcie `:8443`. Po pierwszym włączeniu daj urządzeniu kilka minut.
- Zadanie w panelu się nie udało: jego okno pokazuje log, a strona dodatku
  ma „Pokaż logi” jego kontenerów. Koniec logu zwykle mówi, co poszło nie
  tak.
- Urządzenie nie łączy się z Suplą: sprawdź e-mail, rejestrację urządzeń
  w Cloud i weryfikację certyfikatu w urządzeniu.
- Aplikacja nie łączy się: sprawdź adres serwera, e-mail i rejestrację
  klientów w Cloud.

Pytając na forum albo w
[Issues](https://github.com/Goral64/SuplaOS/issues), napisz, na czym
instalujesz, jaka wersja SuplaOS (strona Aktualizacje), w którym kroku
utknąłeś i co dokładnie widzisz, najlepiej z końcem logu.
