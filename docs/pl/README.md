# SuplaOS

[English](../../README.md)

System na Raspberry Pi 4/5, komputery x86-64 i maszyny wirtualne
([Proxmox, VirtualBox, VMware, Hyper-V](virtual-machines.md)), przygotowany pod
[supla-docker](https://github.com/SUPLA/supla-docker), czyli własny serwer
Supli w domu. Wgrywasz obraz na kartę albo dysk, włączasz urządzenie
i resztę robisz z przeglądarki. Oprócz Supli można dołożyć dodatki:
brokera MQTT, Zigbee2MQTT, zigbee2supla.

Projekt jest na wczesnym etapie. Obraz dla Raspberry Pi nie był jeszcze
sprawdzony na sprzęcie, jeśli możesz pomóc, zobacz [testowanie](testing.md).
Obrazy są w [Releases](https://github.com/Goral64/SuplaOS/releases).

## Pierwsze uruchomienie

Urządzenie pobiera adres z routera (DHCP). Otwórz w przeglądarce
http://suplaos.local (adres IP widać też na ekranie podłączonym do
urządzenia) i zaloguj się hasłem `supla`. Panel najpierw poprosi o jego
zmianę, nowe hasło działa też przy SSH (`ssh supla@suplaos.local`, które
też poprosi o nowe hasło, jeśli zalogujesz się tam najpierw). Potem
wybierasz język, strefę czasową i nazwę urządzenia, i od razu możesz
zainstalować Suplę.

## Dodatki

Dodatki instaluje się, konfiguruje i aktualizuje w panelu, w zakładce Dodatki.
Po instalacji Supli Supla Cloud jest pod https://<adres urządzenia> (port 80
zajmuje panel), a konto zakłada się na stronie dodatku Supla.

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

## Aktualizacje

Panel pokazuje dostępne aktualizacje w zakładce Aktualizacje i instaluje je
jednym przyciskiem: pakiety Debiana, jądro Raspberry Pi i sam SuplaOS, który
ma własne repozytorium apt. Przez SSH to `sudo suplaos upgrade`. Poprawki
bezpieczeństwa i aktualizacje stabilnego Debiana instalują się też same
codziennie (można to wyłączyć na tej samej stronie), a panel przypomina
o restarcie, gdy czeka nowe jądro.

Gdy aktualizacja przynosi nowe wersje plików dodatków (np. nowszy
supla-docker), panel je pokazuje i stosuje na żądanie, po zrobieniu kopii,
bo kontenery tych dodatków powstają od nowa. Nowsze obrazy kontenerów
dodatków pobiera przycisk Aktualizuj na stronie dodatku.

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
`SUPLAOS_HYPERV_DIR`), tworzy maszynę `SuplaOS-dev` na pierwszym przełączniku
zewnętrznym i ją uruchamia. Windows poprosi o uprawnienia administratora.
Maszyna dostaje adres z routera, więc `http://suplaos.local` działa jak
z prawdziwym urządzeniem. Każde uruchomienie zastępuje dysk maszyny świeżym
obrazem.

## Licencja

GPL-3.0, zobacz [LICENSE](../../LICENSE).
