# Certyfikat urządzenia

[In English](../certificate.md)

Panel, Supla Cloud, Zigbee2MQTT i szyfrowany port MQTT działają tylko po
HTTPS. Każde urządzenie z SuplaOS przy pierwszym starcie tworzy własny urząd
certyfikacji (CA) i podpisuje nim certyfikaty dla siebie. Przeglądarki tego CA
nie znają, więc dopóki go nie zainstalujesz, ostrzegają przed połączeniem.
Ostrzeżenie można zaakceptować, ale wygodniej raz zainstalować CA na każdym
komputerze i telefonie, z którego korzystasz. Potem panel, Supla Cloud
i Zigbee2MQTT otwierają się bez ostrzeżeń, także po zmianie adresu IP
urządzenia.

CA może podpisywać certyfikaty tylko dla nazw `.local` i adresów z sieci
lokalnych, więc jego instalacja nie pozwala nikomu podszyć się pod inne
strony.

## Pobranie

W panelu wejdź w System, Certyfikat i kliknij Pobierz certyfikat CA. Plik
nazywa się `suplaos-<nazwa>-ca.crt`. Na tej samej karcie jest jego odcisk
SHA-256, porównaj go z tym, który pokaże system przy instalacji.

Certyfikaty obejmują `https://<nazwa>.local` i adres IP urządzenia, ale nie
samą nazwę bez `.local`.

## Windows

1. Kliknij dwukrotnie plik, potem Zainstaluj certyfikat.
2. Bieżący użytkownik, Dalej.
3. Umieść wszystkie certyfikaty w następującym magazynie, Przeglądaj,
   Zaufane główne urzędy certyfikacji.
4. Dalej, Zakończ i potwierdź ostrzeżenie.

Chrome i Edge korzystają z niego od razu. Firefox zwykle też bierze go
z Windowsa, a jeśli nadal ostrzega, zaimportuj plik w Firefoksie (niżej).

## macOS

1. Kliknij dwukrotnie plik, Pęk kluczy doda go do pęku „login”.
2. Otwórz tam certyfikat (SuplaOS ...), rozwiń Zaufanie i ustaw Podczas
   używania tego certyfikatu na Zawsze ufaj.
3. Zamknij okno i potwierdź hasłem.

## Linux

Dla systemu i narzędzi z wiersza poleceń (curl, klienci mosquitto) na
Debianie i Ubuntu:

    sudo cp suplaos-<nazwa>-ca.crt /usr/local/share/ca-certificates/
    sudo update-ca-certificates

Przeglądarki na Linuksie mają własną listę, więc zaimportuj plik także w nich.

## Firefox

Ustawienia, Prywatność i bezpieczeństwo, Certyfikaty, Wyświetl certyfikaty,
Organy certyfikacji, Importuj. Wybierz plik i zaznacz Zaufaj temu CA przy
identyfikacji witryn internetowych.

## Android

Ustawienia, Bezpieczeństwo (czasem w Więcej ustawień zabezpieczeń),
Szyfrowanie i dane logowania, Zainstaluj certyfikat, Certyfikat CA i wybierz
pobrany plik. Nazwy menu różnią się u producentów telefonów, pomaga wyszukanie
w ustawieniach „certyfikat CA”. Przeglądarki z niego korzystają, większość
aplikacji nie.

## iPhone i iPad

1. Otwórz panel w Safari i pobierz certyfikat, iOS zaproponuje pobranie
   profilu.
2. Ustawienia, Pobrano profil, Zainstaluj.
3. Ustawienia, Ogólne, To urządzenie, Ustawienia zaufania certyfikatów:
   włącz pełne zaufanie dla certyfikatu SuplaOS.

## MQTT

Klienci łączący się z szyfrowanym portem 8883 sprawdzają broker tym samym CA,
np. `mosquitto_sub --cafile suplaos-<nazwa>-ca.crt ...`. Plik można też
pobrać na stronie dodatku Mosquitto.

## Nowe urządzenie, nowe CA

Po zmianie nazwy albo adresu urządzenia CA zostaje to samo. Świeżo wgrany
obraz tworzy nowe: zainstaluj je ponownie, a stare usuń.
