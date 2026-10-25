# Pełna kopia

[English](../backup.md)

Pełna kopia to jeden plik ze wszystkim, co potrzebne, żeby odtworzyć
urządzenie albo przenieść je na inne: dodatki z ustawieniami, ich dane
(bazy Supla Cloud i SUPLA Scripts, sparowane urządzenia Zigbee, użytkownicy
Mosquitto...), CA urządzenia i własna domena z certyfikatem.

Nie ma w niej ustawień samego urządzenia: nazwy, strefy czasowej, języka,
sieci, Wi-Fi i hasła. Ustawia się je na nowym urządzeniu w kreatorze
pierwszego uruchomienia.

## Robienie kopii

Strona System, Pełna kopia. Podaj hasło i zrób kopię, potem pobierz ją
z listy poniżej. Bez hasła kopii nie da się przywrócić, więc trzymaj je
w bezpiecznym miejscu. Można też zrobić kopię bez hasła, ale wtedy każdy,
kto dostanie plik, odczyta z niego hasła dodatków i klucz CA.

Trwa to kilka sekund. Mosquitto, Zigbee2MQTT i Zigbee2Supla zatrzymują się
w tym czasie na chwilę, żeby ich pliki skopiowały się w całości.

Plik to `.tar.gz`, zaszyfrowany w formacie [age](https://age-encryption.org),
jeśli ma hasło (`.tar.gz.age`). Na komputerze otworzysz go poleceniem
`age -d kopia.tar.gz.age > kopia.tar.gz`.

## Kopie automatyczne

Strona System, Kopie automatyczne: codziennie albo co tydzień o podanej
godzinie. Jeśli urządzenie jest wtedy wyłączone, kopia zrobi się po jego
włączeniu. Zostaje kilka najnowszych, starsze kopie automatyczne są usuwane
(zrobione ręcznie zostają).

Gdzie:

- Dysk urządzenia. Chroni przed pomyłką, ale nie przed zepsutą kartą SD
  albo dyskiem.
- Udział sieciowy na komputerze albo NAS-ie (SMB, jak w Windowsie). Udział
  podajesz jako `//192.168.1.10/kopie`, z użytkownikiem i hasłem albo bez
  użytkownika dla udziału gościa. Przy starym NAS-ie wybierz wersję SMB
  ręcznie.
- Udział NFS, `192.168.1.10:/volume1/kopie`. NAS musi pozwalać urządzeniu
  zapisywać tam jako root (bez root squash) albo katalog musi należeć do
  użytkownika, na którego root jest mapowany.

Sprawdź udział testuje go małym plikiem, zanim zapiszesz ustawienia. Hasło
kopii jest przechowywane na urządzeniu, kopie szyfrują się w nocy bez
Ciebie.

Gdy kopia się nie uda (NAS wyłączony, złe hasło), karta pokazuje błąd.
Kolejna próba będzie o następnej porze z harmonogramu, albo kliknij Zrób
kopię teraz.

## Przeniesienie na inne urządzenie

1. Na starym urządzeniu zrób pełną kopię i ją pobierz.
2. Wgraj SuplaOS na nowe i je uruchom. W kreatorze wybierz język, strefę
   czasową i nazwę, a w ostatnim kroku zamiast instalacji Supli wybierz
   Przywróć z pełnej kopii. Wyślij plik i podaj hasło.
3. Nowe urządzenie potrzebuje internetu, bo dodatki pobierają się od nowa.
   Trwa to kilka minut.
4. Wyłącz stare urządzenie. Dwa urządzenia z tym samym koordynatorem Zigbee
   albo tymi samymi kontami Supli będą sobie przeszkadzać.

Co zostaje jak było i może wymagać sprawdzenia:

- Urządzenia Supli łączą się z adresem serwera, który mają zapisany. Jeśli
  był to adres IP starego urządzenia, daj nowemu ten sam adres (rezerwacja
  DHCP w routerze) albo zmień go w urządzeniach. Przy własnej domenie
  skieruj nazwę na nowe urządzenie (lokalny wpis DNS albo rekord u dostawcy
  DNS).
- Koordynator Zigbee na USB może mieć na nowym urządzeniu inną nazwę. Log
  przywracania o tym napisze, ustaw go wtedy ponownie na stronie
  Zigbee2MQTT.
- CA urządzenia przechodzi razem z kopią, więc certyfikat CA zainstalowany
  już na telefonach i komputerach dalej działa. Certyfikaty panelu
  i dodatków tworzą się od nowa pod nową nazwę i adres.

Nowe urządzenie może być innego rodzaju (Raspberry Pi po mini PC albo
odwrotnie), kopia od tego nie zależy. Nie da się jej przywrócić na starszym
SuplaOS niż ten, który ją zrobił: najpierw zaktualizuj.

## Przywracanie na tym samym urządzeniu

Strona System, Pełna kopia, Przywróć przy kopii na liście albo Wyślij kopię,
żeby wgrać plik z komputera. Wszystko, co mają dodatki, zostanie zastąpione.
Najpierw robi się pełna kopia obecnego stanu i pojawia się na liście, więc
przywrócenie można cofnąć.

## Przez SSH

    sudo SUPLAOS_BACKUP_PASSWORD='...' suplaos export
    sudo SUPLAOS_BACKUP_PASSWORD='...' suplaos import /srv/suplaos/backups/suplaos-....tar.gz.age --yes

Bez `SUPLAOS_BACKUP_PASSWORD` kopia nie jest szyfrowana. `export -o` zapisuje
ją do innego pliku.
