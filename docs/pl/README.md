# SuplaOS

[English](../../README.md)

System na Raspberry Pi 4/5, komputery x86-64 i maszyny wirtualne
([Proxmox, VirtualBox, VMware, Hyper-V](virtual-machines.md)), przygotowany pod
[supla-docker](https://github.com/SUPLA/supla-docker), czyli własny serwer
Supli w domu. Wgrywasz obraz na kartę albo dysk, włączasz urządzenie
i resztę robisz z przeglądarki. Oprócz Supli można dołożyć dodatki:
brokera MQTT, Zigbee2MQTT, Zigbee2Supla.

Projekt jest na wczesnym etapie. Obraz dla Raspberry Pi nie był jeszcze
sprawdzony na sprzęcie, jeśli możesz pomóc, zobacz [testowanie](testing.md).
Obrazy są w [Releases](https://github.com/Goral64/SuplaOS/releases).

## Pierwsze uruchomienie

Urządzenie pobiera adres z routera (DHCP). Otwórz w przeglądarce
https://suplaos.local:8443 (http://suplaos.local przekieruje tam, adres IP
widać też na ekranie podłączonym do urządzenia) i zaloguj się hasłem
`supla`. Panel najpierw poprosi o jego zmianę, nowe hasło działa też przy
SSH (`ssh supla@suplaos.local`, które też poprosi o nowe hasło, jeśli
zalogujesz się tam najpierw). Potem wybierasz język, strefę czasową i nazwę
urządzenia, i od razu możesz zainstalować Suplę.

Przeglądarka na początku ostrzeże przed certyfikatem, bo urządzenie tworzy
własny. Zaakceptuj go albo lepiej raz zainstaluj certyfikat CA urządzenia
(strona System w panelu, [jak](certificate.md)). Zwykłe HTTP nie jest
używane, także w sieci lokalnej.

## Dodatki

Dodatki instaluje się, konfiguruje i aktualizuje w panelu, w zakładce Dodatki.
Po instalacji Supli Supla Cloud jest pod https://<adres urządzenia>, a konto
zakłada się na stronie dodatku Supla. Interfejs Zigbee2MQTT jest pod
https://<adres urządzenia>:8080, udostępnia go panel po zalogowaniu.

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

Kopie bazy Supli są na stronie dodatku Supla. Zrób kopię robi ją od razu,
a panel robi ją też przed każdą aktualizacją Supli. Od czasu do czasu warto
pobrać kopię i trzymać ją poza urządzeniem. Przywróć wczytuje kopię z
powrotem: najpierw powstaje kopia bieżących danych, a jeśli wczytanie się
nie uda, wracają one na miejsce. Żeby przenieść dane na nowe urządzenie albo
nową kartę SD, zainstaluj tam Suplę, wgraj pobrany plik przyciskiem Wgraj
kopię i przywróć go.

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
