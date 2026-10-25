# SuplaOS

[English](../../README.md)

System na Raspberry Pi 4/5, komputery x86-64 i maszyny wirtualne
([Proxmox, VirtualBox, VMware, Hyper-V](virtual-machines.md)), przygotowany pod
[supla-docker](https://github.com/SUPLA/supla-docker), czyli własny serwer
Supli w domu. Wgrywasz obraz na kartę albo dysk, włączasz urządzenie
i resztę robisz z przeglądarki. Oprócz Supli można dołożyć dodatki:
brokera MQTT, Zigbee2MQTT, Zigbee2Supla, SUPLA Scripts.

Projekt jest na wczesnym etapie. Obraz dla Raspberry Pi działa na Pi 4
z dyskiem SSD i na Pi 3B+ (Supla działa tam dobrze, ale przy 1 GB RAM nie
dokładałbym wiele więcej), wciąż czeka na test na Pi 5. Obraz x86-64 działa
na terminalu HP T630. Jeśli możesz pomóc, zobacz [testowanie](testing.md).
Obrazy są w [Releases](https://github.com/Goral64/SuplaOS/releases).

## Pierwsze uruchomienie

Podłącz urządzenie kablem do routera, adres dostanie przez DHCP. Otwórz
w przeglądarce https://suplaos.local:8443 (http://suplaos.local przekieruje
tam, adres IP widać też na ekranie podłączonym do urządzenia) i zaloguj się
hasłem `supla`. Panel najpierw poprosi o jego zmianę, nowe hasło działa też przy
SSH (`ssh supla@suplaos.local`, które też poprosi o nowe hasło, jeśli
zalogujesz się tam najpierw). Potem wybierasz język, strefę czasową, nazwę
urządzenia (i Wi-Fi, jeśli urządzenie je ma) i od razu możesz zainstalować
Suplę. Wi-Fi można też ustawić później w System, Sieć, potem kabel nie
jest już potrzebny.

Bez kabla też się da, jeśli urządzenie ma Wi-Fi: gdy przez 2 minuty nie
znajdzie sieci, tworzy własną, `SuplaOS-XXXX` z hasłem `suplaos-setup`.
Połącz z nią telefon, panel otworzy się sam (albo otwórz
https://192.168.4.1:8443), i wybierz swoje Wi-Fi. Urządzenie się z nim
połączy, a telefon wraca do Twojej sieci. Tak samo dzieje się później, gdy
urządzenie straci sieć, np. po zmianie hasła Wi-Fi w routerze.

Przeglądarka na początku ostrzeże przed certyfikatem, bo urządzenie tworzy
własny. Zaakceptuj go albo lepiej raz zainstaluj certyfikat CA urządzenia
(System, Dostęp w panelu, [jak](certificate.md)). Zwykłe HTTP nie jest
używane, także w sieci lokalnej.

Z własną domeną (albo darmową z DuckDNS lub deSEC) urządzenie może zamiast
tego dostać certyfikat Let's Encrypt, zaufany bez instalowania czegokolwiek,
zobacz [własna domena](domain.md).

## Dodatki

Dodatki instaluje się, konfiguruje i aktualizuje w panelu, w zakładce Dodatki.
Po instalacji Supli Supla Cloud jest pod https://<adres urządzenia>.
Pierwsze konto można podać już przy instalacji Supli (także w kreatorze
pierwszego uruchomienia) albo założyć później na stronie dodatku Supla.
Interfejs Zigbee2MQTT jest pod https://<adres urządzenia>:8080, udostępnia
go panel po zalogowaniu.

Konto z publicznego Supla Cloud można tu przenieść razem z urządzeniami,
scenami i historią pomiarów, a urządzenia i kanały zachowują swoje ID:
zobacz [przeniesienie z Supla Cloud](migration.md).

[SUPLA Scripts](https://github.com/fracz/supla-scripts) jest pod
https://<adres urządzenia>:8444. Loguje się tam tokenem osobistym z Supla
Cloud (Konto, Bezpieczeństwo, Osobiste tokeny dostępowe) z dostępem do
konta, urządzeń i kanałów, a potem zakłada login i hasło do samego SUPLA
Scripts. Działa z Cloud na tym urządzeniu i z cloud.supla.org. Obraz buduje
SuplaOS z wydań supla-scripts.

Gdy Supla działa na tym samym urządzeniu, na stronie dodatku SUPLA Scripts
jest Połącz z Supla Cloud: panel zakłada w Cloud aplikację OAuth (na podanym
koncie, hasło nie jest zapisywane) i od tej pory SUPLA Scripts pozwala
logować się przez Supla Cloud i od razu dostaje zmiany kanałów. Bez Supli na
urządzeniu ustawienia dodatku przyjmują aplikację OAuth założoną ręcznie
(Integracje, Moje aplikacje OAuth, adres przekierowania https://<adres
urządzenia>:8444/authorize).

To samo można zrobić przez SSH poleceniem `suplaos`:

    suplaos list
    suplaos info supla
    suplaos install supla
    suplaos run supla console supla:user:create

ZigBee z koordynatorem USB:

    suplaos install mosquitto
    suplaos install zigbee2mqtt ZIGBEE_ADAPTER=/dev/serial/by-id/usb-...
    suplaos install zigbee2supla Z2S_SUPLA_EMAIL=ty@example.com

Ustawienia zmienia się później przez `suplaos config <dodatek> KLUCZ=WARTOŚĆ`.
Wszystko jest w `/srv/suplaos`: wygenerowany projekt compose w `stack/`,
kopie bazy w `backups/`.

Instrukcje do Supla Cloud często podają polecenia w stylu
`docker compose exec -u www-data supla-cloud php bin/console ...`, np. do
zmiany limitów konta. W SuplaOS to samo robi się przez
`suplaos run supla console`:

    suplaos run supla console supla:user:change-limits ty@example.com

`suplaos run supla console list` pokazuje wszystkie polecenia konsoli. SQL
wysyła się do kontenera bazy przez `suplaos run supla sql`, jako skrypt albo
pojedyncze zapytanie, a bez argumentów otwiera się klient MariaDB:

    suplaos run supla sql < skrypt.sql
    suplaos run supla sql -e "select email from supla_user"

Inne polecenia `docker compose` działają jako `suplaos compose ...` (np.
`suplaos compose ps`), w projekcie compose SuplaOS.

## Kopie zapasowe

Pełna kopia na stronie Kopie zawiera wszystkie dodatki z danymi i CA
urządzenia, zaszyfrowane hasłem. Może się robić sama codziennie albo co
tydzień, także na udział sieciowy (SMB albo NFS). Żeby przenieść się na inne
urządzenie albo nową kartę SD, przywróć ją tam w kreatorze pierwszego
uruchomienia. Zobacz [pełna kopia](backup.md).

Same kopie bazy Supli są na stronie dodatku Supla, a panel robi je też przed
każdą aktualizacją Supli. Przywróć wczytuje kopię z powrotem: najpierw
powstaje kopia bieżących danych, a jeśli wczytanie się nie uda, wracają one
na miejsce.

Przez SSH:

    suplaos backup supla
    suplaos restore supla supla-20261024141039.sql.gz

## Aktualizacje

Panel pokazuje dostępne aktualizacje w zakładce Aktualizacje i instaluje je
jednym przyciskiem: pakiety Debiana, jądro Raspberry Pi i sam SuplaOS, który
ma własne repozytorium apt. Przez SSH to `sudo suplaos upgrade`. Poprawki
bezpieczeństwa i aktualizacje stabilnego Debiana instalują się też same
codziennie (można to wyłączyć na tej samej stronie), a panel przypomina
o restarcie, gdy czeka nowe jądro.

Gdy aktualizacja przynosi nowe wersje plików dodatków (np. nowszy
supla-docker), panel sam je stosuje po aktualizacji, najpierw robiąc kopię,
a kontenery tych dodatków powstają od nowa. Do wersji 1.0 dzieje się to bez
pytania. Nowsze obrazy kontenerów dodatków pobiera przycisk Aktualizuj na
stronie dodatku. Panel co kilka godzin sprawdza, czy są, i pokazuje wersje,
nowe wersje i link do listy zmian.

Co się zmieniło w kolejnych wersjach, jest w [CHANGELOG.md](CHANGELOG.md).

Pakiet `suplaos` z tego repozytorium jest częścią obrazów. Nie służy do
instalowania na istniejącym Debianie: panel oczekuje konta `supla` i portów
80 i 8443, a pakiet ciągnie Dockera z Debiana.

## Budowanie obrazu

Potrzebny jest Docker.

    tools/build.sh amd64
    tools/build.sh rpi

Obraz trafia do `out/`. Na komputerze x86-64 obraz dla Raspberry Pi buduje się
z emulacją qemu, co trochę trwa. Do szybkiego testu w QEMU:

    tools/run-qemu.sh

SSH jest wtedy na `localhost:2222`.

Na Windowsie z WSL obraz można uruchomić w Hyper-V:

    tools/hyperv.sh

Skrypt konwertuje obraz do VHDX w `F:\Hyper-V\SuplaOS` (inny katalog:
`SUPLAOS_HYPERV_DIR`), tworzy maszynę `SuplaOS-dev` (inna nazwa:
`SUPLAOS_HYPERV_VM`) na pierwszym przełączniku zewnętrznym i ją uruchamia.
Windows poprosi o uprawnienia administratora. Maszyna dostaje adres
z routera, więc `https://suplaos.local:8443` działa jak z prawdziwym
urządzeniem. Każde uruchomienie zastępuje dysk maszyny świeżym
obrazem.

## Licencja

GPL-3.0, zobacz [LICENSE](../../LICENSE).
