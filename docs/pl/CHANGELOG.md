# Zmiany

[In English](../../CHANGELOG.md)

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
