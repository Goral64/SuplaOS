# Testowanie SuplaOS

[English](../testing.md)

SuplaOS jest na wczesnym etapie i szukam osób, które go wypróbują, szczególnie
na Raspberry Pi 4 albo 5, których sam teraz nie mogę sprawdzić. Na razie nie
używaj go do swojej głównej instalacji Supli: wiele się jeszcze zmieni.
Aktualizacje instaluje się z panelu (Aktualizacje), ale przy wczesnych
wersjach czasem może być potrzebne wgranie obrazu od nowa.

## Skąd wziąć obraz

Obrazy są w [Releases](https://github.com/Goral64/SuplaOS/releases):

- `suplaos-<wersja>-rpi-arm64.img.xz` na Raspberry Pi 4 i 5,
- `suplaos-<wersja>-amd64.img.xz` na komputery x86-64 i maszyny wirtualne
  (UEFI, zobacz [maszyny wirtualne](virtual-machines.md)).

Wgraj go na kartę SD, dysk USB albo SSD przez Raspberry Pi Imager („Wybierz
OS”, potem „Użyj własnego”) albo balenaEtcher. Oba biorą plik `.xz` bez
rozpakowywania. W Raspberry Pi Imager pomiń „ustawienia OS”, SuplaOS z nich
nie korzysta.

Urządzenie potrzebuje kabla sieciowego, Wi-Fi nie jest jeszcze ustawiane.

## Pierwsze uruchomienie

1. Włącz urządzenie i poczekaj 2-3 minuty. Przy pierwszym starcie system
   powiększa się na cały dysk i generuje klucze.
2. Znajdź jego adres: na ekranie podłączonym do urządzenia, na liście
   urządzeń w routerze albo spróbuj http://suplaos.local.
3. Otwórz panel w przeglądarce i zaloguj się hasłem `supla`. Panel poprosi
   o jego zmianę. Nowe hasło działa też przy SSH (`ssh supla@suplaos.local`).
4. Przejdź kreator: język, strefa czasowa, nazwa urządzenia i, jeśli chcesz,
   instalacja Supli.

## Co sprawdzić

- Czy urządzenie startuje i dostaje adres? Czy adres widać na podłączonym
  ekranie?
- Czy panel się otwiera, czy pulpit pokazuje sensowne wartości (pamięć, dysk,
  temperatura na Raspberry Pi)?
- Supla: zainstaluj ją, załóż konto na stronie dodatku Supla, zaloguj się do
  Supla Cloud pod https://<adres urządzenia> i podłącz jakieś urządzenie Supla.
- ZigBee (jeśli masz koordynator): zainstaluj Mosquitto, Zigbee2MQTT
  i zigbee2supla. Koordynator USB powinien być na liście urządzeń
  w ustawieniach Zigbee2MQTT.
- Uruchom urządzenie ponownie z panelu (System), wszystko powinno wstać samo.
- Gdy wyjdzie nowa wersja: zaktualizuj system z panelu (Aktualizacje)
  i sprawdź, czy wszystko dalej działa.

## Informacja zwrotna

Napisz, co sprawdziłeś i co nie zadziałało, razem z:

- urządzeniem (np. Raspberry Pi 4 4 GB, mini PC, maszyna w Proxmoksie)
  i nośnikiem (karta SD, SSD na USB, NVMe),
- wersją obrazu,
- jeśli coś nie działało, wynikiem tych poleceń (przez SSH):

      cat /etc/suplaos-release
      systemctl --failed
      ip -br a
      suplaos status
      sudo journalctl -b -p warning --no-pager | tail -n 100

Problemy można zgłaszać w [Issues](https://github.com/Goral64/SuplaOS/issues).
